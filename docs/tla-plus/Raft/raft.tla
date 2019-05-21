-------------------------------- MODULE raft --------------------------------

EXTENDS TLC, Sequences, FiniteSets, Integers

PT == INSTANCE PT

CONSTANTS
    \* How many terms to simulate (i.e. when to stop exploring)
    MaxTerm,
    \* Model set of peers, for example {p1,p2,p3,p4}
    Peers,
    \* Seq of conf changes, for example << <<{p1,p2,p3}, {}>>, <<{p1,p2}, {}>>
    Cfgs

CONSTANTS
    \* Peer states (model values)
    Leader,
    Candidate,
    Follower,
    \* More useful model values
    NULL,
    MsgVote,
    MsgVoteResp,
    MsgApp,
    MsgAppResp

Terms == 0..MaxTerm
Votes == Peers \union {NULL}
Roles == {Leader, Candidate, Follower}

TypeMsgAppInvariant == [
            type:  {MsgApp},
            from:  Peers,
            term:  Terms,
            logTerm: Terms,
            logIndex: 1..(1+Len(Cfgs)),
            cfg: [ 1..2 -> SUBSET Peers ],
            committed: 1..(1+Len(Cfgs))
        ]

TypeMsgAppRespInvariant == [
            type:  {MsgAppResp, MsgVoteResp},
            from:  Peers,
            to:    Peers,
            term:  Terms,
            index: 1..(1+Len(Cfgs))
        ]

TypeMsgVoteInvariant == [
            type:  {MsgVote},
            from:  Peers,
            term:  Terms,
            logTerm: Terms,
            logIndex: 1..(1+Len(Cfgs))
        ]

TypeMsgVoteRespInvariant == [
            type:  {MsgAppResp, MsgVoteResp},
            from:  Peers,
            to:    Peers,
            term:  Terms
        ]

TypeInflightInvariant == SUBSET UNION {
        TypeMsgAppInvariant,
        TypeMsgVoteInvariant,
        TypeMsgAppRespInvariant,
        TypeMsgVoteRespInvariant
    }

MaxLogIndex == 1 + Len(Cfgs) \* initial log index plus number of possible proposals
LogEntries == [ term: Terms, cfg: PT!Range(Cfgs) \union {NULL} ]
Logs == UNION { PT!TupleOf(LogEntries, n) : n \in 1..MaxLogIndex }
TypeGlobalStateInvariant == 
    [
        log:        [ Peers -> Logs ],
        term:       [ Peers -> Terms ],
        vote:       [ Peers -> Votes ],
        cfg:        [ Peers -> [ 1..2 -> SUBSET Peers ] ],
        committed:  [ Peers -> Nat ],
        role:       [ Peers -> Roles ],
        inflight:   TypeInflightInvariant
    ]

\* This will be the first fully committed entry in the initial log, to avoid
\* bootstrapping code that we don't care about.
FirstEntry == [ term |-> 1, cfg |-> NULL ]

\* A config is a tuple. If the second component is empty it's
\* not a joint quorum. In the initial config, we want a majority
\* of Peers.
InitialConfig == <<Peers, {}>>


(***************************************************************************
Log access.
 ***************************************************************************)

LastIndex(log) == Len(log)
LastTerm(log) == log[LastIndex(log)].term
EntryAtOrNull(log, idx) == IF Len(log) >= idx THEN log[idx] ELSE NULL

(***************************************************************************
Quorum computation.
 ***************************************************************************)

HasMajority(peers, acks) ==
    \/ peers = {}
    \/ Cardinality(acks \cap peers) >= Cardinality(peers) \div 2 + 1

ConfSatisfied(cfg, acks) ==
    /\ HasMajority(cfg[1], acks)
    /\ HasMajority(cfg[2], acks)        

(***************************************************************************
Message helpers.

All messages ever sent can in principle be received by nodes at any time
(including multiple times). All messages have an origin (msg.from), but
there are broadcast and unicast message types.

- broadcast: MsgApp, MsgVote
  These are sent without a recipient and simulate a copy of the message
  sent to each individual peer.
- unicast: MsgAppResp, MsgVoteResp
  These are the responses to MsgApp and MsgVote, respectively, and are
  addressed specifically sent back to the origin node. This is necessary
  because, for example, we can only vote for one candidate but there may
  be multiple that would otherwise interpret a broadcast response as a
  vote in their favor.
 ***************************************************************************)

\* All inflight messages of a given type.
Inflight(state, type) == {m \in state.inflight: m.type = type}

\* For a given node, term, and set of unicast messages, return the set of nodes
\* that have messaged the node at the given term. This is used to find out which
\* peers voted for the node, or acknowledged a certain log entry.
AcksFor(node, term, resps) == { m.from: m \in
            { msg \in resps: msg.term = term /\ msg.to = node }}

Send(state, msg) == [ inflight |-> state.inflight \union {msg} ] @@ state

(***************************************************************************
Transitions between the Follower, Leader, and Candidate roles.
 ***************************************************************************)
 
\* Update the role of a peer, perhaps with an updated term. The term can never
\* decrease (something to check via an invariant). If the term increases, the
\* vote will be cleared. Not called directly from the main event loop.
Reset(state, term, role) ==
    [ term |-> term,
      role |-> role,
      \* Reset vote only if term changed.
      vote |-> IF term > state.term THEN NULL ELSE state.vote
    ] @@ state

\* If the new term is larger, reset to become a follower.
MaybeBecomeFollower(state, term) ==
    IF state.term < term
    THEN Reset(state, term, Follower)
    ELSE state

\* Become a candidate for the next term.
BecomeCandidate(state) ==
    Send(
        Reset(state, state.term+1, Candidate),
        [
            type |-> MsgVote,
            from |-> state.self,
            term |-> state.term+1,
            logTerm |-> LastTerm(state.log),
            logIndex |-> LastIndex(state.log)
        ]
    )

(***************************************************************************
Reacting to broadcast messages, i.e. winning an election or receiving enough
acks to commit a proposed log entry.
 ***************************************************************************)

\* Become leader for the current term if there are enough votes (that could in
\* principle be received) to do so.
MaybeBecomeLeader(state) ==
    LET
        votes == AcksFor(state.self, state.term, Inflight(state, MsgVoteResp))
    IN
        IF ConfSatisfied(state.cfg, votes) THEN
            Reset(state, state.term, Leader)
        ELSE state

\* Mark the entry at log position idx as committed if there are enough acks (that
\* could in principle be received) to do so.
MaybeMarkCommitted(state, idx) ==
    LET
        acks == AcksFor(
                state.self,
                state.term,
                \* Note that there could be messages for higher indexes
                \* that satisfy the config and those might implicitly
                \* commit idx as well. But in our model of networking
                \* a higher commit index implies that acks are actually
                \* on the network for all lower indexes.
                {m \in Inflight(state, MsgAppResp): m.index = idx})
    IN
        IF /\ idx > state.committed
           /\ ConfSatisfied(state.cfg, acks)
        THEN [ committed |-> idx ] @@ state
        ELSE state

(***************************************************************************
Reacting to unicast messages, i.e. being asked to vote or to append an entry.
 ***************************************************************************)

\* Handle an incoming request to vote. The assumption is that the state already
\* reflects any term bump that the message may have caused. If the vote request
\* is for the current term, and the peer hasn't voted, persist the vote and send
\* a response.
HandleVoteReq(state, msg) ==
    LET
        rej == \/ msg.term /= state.term
               \/ state.vote \notin {NULL, msg.from}
        resp == [ type |-> MsgVoteResp,
                from |-> state.self,
                to |-> msg.from,
                term |-> state.term ]
    IN IF ~rej THEN
        Send([ vote |-> msg.from ] @@ state, resp)
    ELSE state

\* Handle an updated commit index communicated to a peer by a leader.
\* This simply ratchets the commit index up.
MaybeCommit(state, idx) ==
    IF state.committed < idx THEN
        [ committed |-> idx ] @@ state
    ELSE state

\* React to an append request (assuming that any term bumps are already
\* reflected in the state). This calls MaybeCommit and, if the append
\* matches up with the current log, adds the entry (discarding any over-
\* lapping uncommitted tail as necessary). An ack is sent on success.
HandleAppend(oldstate, msg) ==
    LET
        state == MaybeCommit(oldstate, msg.committed)
        \* Entry to be overwritten (may not exist).
        exEnt == EntryAtOrNull(state.log, msg.logIndex)
        ent == [term |-> msg.term, cfg |-> msg.cfg]
        \* Response to be sent on success.
        resp == [
            type |-> MsgAppResp,
            from |-> state.self,
            to |-> msg.from,
            term |-> state.term,
            index |-> msg.logIndex+1
        ]
    IN
        IF exEnt = NULL \/ exEnt.term /= msg.logTerm THEN
            \* "base" entry missing or mismatched, need to
            \* handle the append that gives us that one first.
            state
        ELSE
            \* Can just append entry, overwriting and discarding
            \* conflicting any conflicting log tail.
            Send(
                [
                    log |-> Append(SubSeq(state.log, 1, msg.logIndex), ent),
                    cfg |-> msg.cfg
                ] @@ state,
                msg
            )


\* Multiplex unicast requests into their specialized handlers above.
HandleReq(state, msg) ==
    IF msg.type = MsgVote THEN
        HandleVoteReq(state, msg)
    ELSE
    IF msg.type = MsgApp THEN
        HandleAppend(state, msg)
    ELSE state

(***************************************************************************
Proposal of new commands (only valid for leaders).
 ***************************************************************************)

\* ProposeConfChange proposes a new entry. The entry is added to the local log
\* and an append broadcast into the ether.
ProposeConfChange(state, nextCfg) ==
    LET msg == [
            type |-> MsgApp,
            from |-> state.self,
            logIndex |-> LastIndex(state.log),
            logTerm |-> LastTerm(state.log),
            term |-> state.term,
            committed |-> state.committed,
            cfg |-> nextCfg ]
    IN Send(HandleAppend(state, msg), msg)

(*--algorithm raft

variable
    inflight = {};
    cfgs = Cfgs;
    \* Data that must be persisted to disk (i.e. won't be lost
    \* across simulated restarts).
    p_log = [ p \in Peers |-> <<FirstEntry>> ];
    p_term = [ p \in Peers |-> FirstEntry.term ];
    p_vote = [ p \in Peers |-> NULL ];
    p_cfg = [ p \in Peers |-> InitialConfig ];
    \* We can simulate crash-restarts by resetting these
    \* (but don't right now).
    p_committed = [ p \in Peers |-> FirstEntry.term ];
    p_role = [ p \in Peers |-> Follower ];

define

GlobalState == [
    log        |-> p_log,
    term       |-> p_term,
    vote       |-> p_vote,
    cfg        |-> p_cfg,
    committed  |-> p_committed,
    role       |-> p_role,
    inflight   |-> inflight
]

TypeInvariant == GlobalState \in TypeGlobalStateInvariant

State(p) == [
    self       |-> p,
    log        |-> p_log[p],
    term       |-> p_term[p],
    vote       |-> p_vote[p],
    cfg        |-> p_cfg[p],
    committed  |-> p_committed[p],
    role       |-> p_role[p],

    inflight   |-> inflight
]


LogsMatch(l, m) == \A i \in 1..PT!Min(Len(l), Len(m)) : l[i] = m[i]

\* NB: this invariant is supposed to be violated. The logs only need to match up
\* to the commit index, they can diverge after.
\* TODO(tbg): assert the above, and that (index, term) -> entry is unique.
AllLogsMatch == \A p, q \in Peers: LogsMatch(State(p).log, State(q).log)

TermNeverDecreases == [][\A p \in Peers: p_term[p] <= p_term'[p]]_p_term
ProposedEntryEventuallyCommits == TRUE \* TBD
CommittedEntriesWereProposed == TRUE \* TBD

end define;

macro ApplyState(state)
begin
    assert DOMAIN state = DOMAIN State(self);
    p_log[self]       := state.log;
    p_term[self]      := state.term;
    p_vote[self]      := state.vote;
    p_cfg[self]       := state.cfg;
    p_committed[self] := state.committed;
    p_role[self]      := state.role;
    inflight          := state.inflight;
end macro;

macro RunOnce()
begin
    with state = State(self) do
            either
                \* Don't allow entering any term > MaxTerm.
                await state.term < MaxTerm;
                ApplyState(BecomeCandidate(state))
            or
                await state.role = Candidate;
                ApplyState(MaybeBecomeLeader(state));
            or
                await /\ state.role = Leader
                      /\ Len(cfgs) > 0;
                with nextCfg = Head(cfgs) do
                    cfgs := Tail(cfgs);
                    ApplyState(ProposeConfChange(state, nextCfg));
               end with;
            or
               await /\ state.role = Leader
                     /\ LastIndex(state.log) > state.committed;
               with idx \in state.committed+1..LastIndex(state.log) do
                    ApplyState(MaybeMarkCommitted(state, idx));
                end with;
            or
                with msg \in state.inflight do
                    ApplyState(HandleReq(MaybeBecomeFollower(state, msg.term), msg))
                end with;
            end either;
    end with
end macro;

fair process peer \in Peers
begin
Loop:
    while State(self).term <= MaxTerm do
        either
            RunOnce();
        or
            \* If the election for MaxTerm ends in a stalemate, all nodes
            \* may end up in MaxTerm without any possible state transitions
            \* (since we don't allow campaigning past MaxTerm) which leads
            \* to stuttering. Allow peers to bail at this point to avoid
            \* this (while at the same time making sure we never get stuck
            \* in any earlier term).
            await State(self).term = MaxTerm;
            goto Exit;
        end either;
    end while;
    assert FALSE;
    assert Len(cfgs) = 0;
    assert \A p \in Peers: /\ State(p).committed = Len(State(p).log)
                           /\ Len(State(p).log) = Len(Cfgs)+4;
Exit:
    skip;
end process;

end algorithm; *)
\* BEGIN TRANSLATION
VARIABLES inflight, cfgs, p_log, p_term, p_vote, p_cfg, p_committed, p_role, 
          pc

(* define statement *)
GlobalState == [
    log        |-> p_log,
    term       |-> p_term,
    vote       |-> p_vote,
    cfg        |-> p_cfg,
    committed  |-> p_committed,
    role       |-> p_role,
    inflight   |-> inflight
]

TypeInvariant == GlobalState \in TypeGlobalStateInvariant

State(p) == [
    self       |-> p,
    log        |-> p_log[p],
    term       |-> p_term[p],
    vote       |-> p_vote[p],
    cfg        |-> p_cfg[p],
    committed  |-> p_committed[p],
    role       |-> p_role[p],

    inflight   |-> inflight
]


LogsMatch(l, m) == \A i \in 1..PT!Min(Len(l), Len(m)) : l[i] = m[i]




AllLogsMatch == \A p, q \in Peers: LogsMatch(State(p).log, State(q).log)

TermNeverDecreases == [][\A p \in Peers: p_term[p] <= p_term'[p]]_p_term
ProposedEntryEventuallyCommits == TRUE
CommittedEntriesWereProposed == TRUE


vars == << inflight, cfgs, p_log, p_term, p_vote, p_cfg, p_committed, p_role, 
           pc >>

ProcSet == (Peers)

Init == (* Global variables *)
        /\ inflight = {}
        /\ cfgs = Cfgs
        /\ p_log = [ p \in Peers |-> <<FirstEntry>> ]
        /\ p_term = [ p \in Peers |-> FirstEntry.term ]
        /\ p_vote = [ p \in Peers |-> NULL ]
        /\ p_cfg = [ p \in Peers |-> InitialConfig ]
        /\ p_committed = [ p \in Peers |-> FirstEntry.term ]
        /\ p_role = [ p \in Peers |-> Follower ]
        /\ pc = [self \in ProcSet |-> "Loop"]

Loop(self) == /\ pc[self] = "Loop"
              /\ IF State(self).term <= MaxTerm
                    THEN /\ \/ /\ LET state == State(self) IN
                                    \/ /\ state.term < MaxTerm
                                       /\ Assert(DOMAIN (BecomeCandidate(state)) = DOMAIN State(self), 
                                                 "Failure of assertion at line 357, column 5 of macro called at line 403, column 13.")
                                       /\ p_log' = [p_log EXCEPT ![self] = (BecomeCandidate(state)).log]
                                       /\ p_term' = [p_term EXCEPT ![self] = (BecomeCandidate(state)).term]
                                       /\ p_vote' = [p_vote EXCEPT ![self] = (BecomeCandidate(state)).vote]
                                       /\ p_cfg' = [p_cfg EXCEPT ![self] = (BecomeCandidate(state)).cfg]
                                       /\ p_committed' = [p_committed EXCEPT ![self] = (BecomeCandidate(state)).committed]
                                       /\ p_role' = [p_role EXCEPT ![self] = (BecomeCandidate(state)).role]
                                       /\ inflight' = (BecomeCandidate(state)).inflight
                                       /\ cfgs' = cfgs
                                    \/ /\ state.role = Candidate
                                       /\ Assert(DOMAIN (MaybeBecomeLeader(state)) = DOMAIN State(self), 
                                                 "Failure of assertion at line 357, column 5 of macro called at line 403, column 13.")
                                       /\ p_log' = [p_log EXCEPT ![self] = (MaybeBecomeLeader(state)).log]
                                       /\ p_term' = [p_term EXCEPT ![self] = (MaybeBecomeLeader(state)).term]
                                       /\ p_vote' = [p_vote EXCEPT ![self] = (MaybeBecomeLeader(state)).vote]
                                       /\ p_cfg' = [p_cfg EXCEPT ![self] = (MaybeBecomeLeader(state)).cfg]
                                       /\ p_committed' = [p_committed EXCEPT ![self] = (MaybeBecomeLeader(state)).committed]
                                       /\ p_role' = [p_role EXCEPT ![self] = (MaybeBecomeLeader(state)).role]
                                       /\ inflight' = (MaybeBecomeLeader(state)).inflight
                                       /\ cfgs' = cfgs
                                    \/ /\ /\ state.role = Leader
                                          /\ Len(cfgs) > 0
                                       /\ LET nextCfg == Head(cfgs) IN
                                            /\ cfgs' = Tail(cfgs)
                                            /\ Assert(DOMAIN (ProposeConfChange(state, nextCfg)) = DOMAIN State(self), 
                                                      "Failure of assertion at line 357, column 5 of macro called at line 403, column 13.")
                                            /\ p_log' = [p_log EXCEPT ![self] = (ProposeConfChange(state, nextCfg)).log]
                                            /\ p_term' = [p_term EXCEPT ![self] = (ProposeConfChange(state, nextCfg)).term]
                                            /\ p_vote' = [p_vote EXCEPT ![self] = (ProposeConfChange(state, nextCfg)).vote]
                                            /\ p_cfg' = [p_cfg EXCEPT ![self] = (ProposeConfChange(state, nextCfg)).cfg]
                                            /\ p_committed' = [p_committed EXCEPT ![self] = (ProposeConfChange(state, nextCfg)).committed]
                                            /\ p_role' = [p_role EXCEPT ![self] = (ProposeConfChange(state, nextCfg)).role]
                                            /\ inflight' = (ProposeConfChange(state, nextCfg)).inflight
                                    \/ /\ /\ state.role = Leader
                                          /\ LastIndex(state.log) > state.committed
                                       /\ \E idx \in state.committed+1..LastIndex(state.log):
                                            /\ Assert(DOMAIN (MaybeMarkCommitted(state, idx)) = DOMAIN State(self), 
                                                      "Failure of assertion at line 357, column 5 of macro called at line 403, column 13.")
                                            /\ p_log' = [p_log EXCEPT ![self] = (MaybeMarkCommitted(state, idx)).log]
                                            /\ p_term' = [p_term EXCEPT ![self] = (MaybeMarkCommitted(state, idx)).term]
                                            /\ p_vote' = [p_vote EXCEPT ![self] = (MaybeMarkCommitted(state, idx)).vote]
                                            /\ p_cfg' = [p_cfg EXCEPT ![self] = (MaybeMarkCommitted(state, idx)).cfg]
                                            /\ p_committed' = [p_committed EXCEPT ![self] = (MaybeMarkCommitted(state, idx)).committed]
                                            /\ p_role' = [p_role EXCEPT ![self] = (MaybeMarkCommitted(state, idx)).role]
                                            /\ inflight' = (MaybeMarkCommitted(state, idx)).inflight
                                       /\ cfgs' = cfgs
                                    \/ /\ \E msg \in state.inflight:
                                            /\ Assert(DOMAIN (HandleReq(MaybeBecomeFollower(state, msg.term), msg)) = DOMAIN State(self), 
                                                      "Failure of assertion at line 357, column 5 of macro called at line 403, column 13.")
                                            /\ p_log' = [p_log EXCEPT ![self] = (HandleReq(MaybeBecomeFollower(state, msg.term), msg)).log]
                                            /\ p_term' = [p_term EXCEPT ![self] = (HandleReq(MaybeBecomeFollower(state, msg.term), msg)).term]
                                            /\ p_vote' = [p_vote EXCEPT ![self] = (HandleReq(MaybeBecomeFollower(state, msg.term), msg)).vote]
                                            /\ p_cfg' = [p_cfg EXCEPT ![self] = (HandleReq(MaybeBecomeFollower(state, msg.term), msg)).cfg]
                                            /\ p_committed' = [p_committed EXCEPT ![self] = (HandleReq(MaybeBecomeFollower(state, msg.term), msg)).committed]
                                            /\ p_role' = [p_role EXCEPT ![self] = (HandleReq(MaybeBecomeFollower(state, msg.term), msg)).role]
                                            /\ inflight' = (HandleReq(MaybeBecomeFollower(state, msg.term), msg)).inflight
                                       /\ cfgs' = cfgs
                               /\ pc' = [pc EXCEPT ![self] = "Loop"]
                            \/ /\ State(self).term = MaxTerm
                               /\ pc' = [pc EXCEPT ![self] = "Exit"]
                               /\ UNCHANGED <<inflight, cfgs, p_log, p_term, p_vote, p_cfg, p_committed, p_role>>
                    ELSE /\ Assert(FALSE, 
                                   "Failure of assertion at line 415, column 5.")
                         /\ Assert(Len(cfgs) = 0, 
                                   "Failure of assertion at line 416, column 5.")
                         /\ Assert(\A p \in Peers: /\ State(p).committed = Len(State(p).log)
                                                   /\ Len(State(p).log) = Len(Cfgs)+4, 
                                   "Failure of assertion at line 417, column 5.")
                         /\ pc' = [pc EXCEPT ![self] = "Exit"]
                         /\ UNCHANGED << inflight, cfgs, p_log, p_term, p_vote, 
                                         p_cfg, p_committed, p_role >>

Exit(self) == /\ pc[self] = "Exit"
              /\ TRUE
              /\ pc' = [pc EXCEPT ![self] = "Done"]
              /\ UNCHANGED << inflight, cfgs, p_log, p_term, p_vote, p_cfg, 
                              p_committed, p_role >>

peer(self) == Loop(self) \/ Exit(self)

Next == (\E self \in Peers: peer(self))
           \/ (* Disjunct to prevent deadlock on termination *)
              ((\A self \in ProcSet: pc[self] = "Done") /\ UNCHANGED vars)

Spec == /\ Init /\ [][Next]_vars
        /\ \A self \in Peers : WF_vars(peer(self))

Termination == <>(\A self \in ProcSet: pc[self] = "Done")

\* END TRANSLATION

=============================================================================
\* Modification History
\* Last modified Mon May 20 21:04:52 CEST 2019 by tschottdorf
\* Created Mon Apr 29 13:11:24 CEST 2019 by tschottdorf
