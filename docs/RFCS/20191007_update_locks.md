- Feature Name:
- Status: draft/in-progress/completed/rejected/obsolete/postponed
- Start Date: YYYY-MM-DD
- Authors:
- RFC PR: (PR # after acceptance of initial draft)
- Cockroach Issue: (one or more # from the issue tracker)

**Remember, you can submit a PR with your RFC before the text is
complete. Refer to the [README](README.md#rfc-process) for details.**

# Summary

One paragraph explanation of the proposed change.

Suggested contents:
- What is being proposed
- Why (short reason)
- How (short plan)
- Impact

# Motivation

Why are we doing this? What use cases does it support? What is the expected outcome?

# Guide-level explanation

In the CockroachDB transaction model, transactions lay down intents before they
commit. While fundamentally, an intent is little more than a replicated write
which we want to make visible only with the commit operation, they play a
central role in CockroachDB's concurrency handling. This RFC will propose a
change to the concurrency model, and thus we should take a moment to understand
the "cost function" that determines which changes to the model we would consider
worthwhile.

- round-trips are expensive: CockroachDB deployments usually at least span
  zones, if not regions or continents. As such, we must assume that a round-trip
adds at 100ms or more of latency to the transaction.
- no contention, no cost: contention (i.e. multiple readers/writers clashing over access to overlapping key spans) occurs not at most sporadically for many workloads. While it is important to perform reasonably well when contention does occur, no penalties should be introduced on
non-contended operations.

Let's evaluate today's concurrency model against an example, namely the
following user transaction on a simple key-value table containing rows for
`k=0,1,2`:

```sql
BEGIN;
UPDATE kv SET v = v+1 WHERE k = 0;
UPDATE kv SET v = v+1 WHERE k = 1;
UPDATE kv SET v = v+1 WHERE k = 2;
COMMIT;
```

This example was chosen for multiple reasons and is a reasonable representative
of transactions as used by ORMs.

- ORMs tend to perform read-modify-write cycles, which is also how the `UPDATE`
  statement is implemented internally at the time of writing.
- Multi-statement transactions are common, particularly in ORMs, and are
  especially hard to avoid there.
- apps using an ORM often cannot implement the `SAVEPOINT cockroach_restart`
  technique which we have used to make transaction restart handling work.

When running without contending readers or writers, this transaction runs as
efficiently as it possibly can (given that the client will only supply the
statements one by one):

1. A transaction timestamp (say `t=100`) is chosen by the gateway.
1. The row `k=0` is read at `t=100` (say it reads `0`).
1. The row `k=0` is updated with `v=1` at `t=100` (pipelined write)
1. The row `k=1` is read at `t=100` (say it reads `0`).
1. The row `k=1` is updated with `v=1` at `t=100` (pipelined write)
1. The row `k=2` is read at `t=100` (say it reads `0`).
1. The row `k=2` is updated with `v=1` at `t=100` (pipelined write)
1. The transaction commits.

This transaction could be run more optimally: the last write could be batched
together with the commit, and in fact this could happen for all writes if the
client supplied the entirety of transaction in one statement (and the SQL
layer were smart enough to emit the optimal KV operations from that). None of
those optimizations are inefficiencies in the KV transactional protocol, though,
so we don't dwell on them further.

Now let's assume the same transaction runs in a contended environment. In the
worst case, we'll see something like this:

1. Transaction `T` starts, at timestamp (say) `t=100` as before.
1. Transaction `C` (for `C`ontending) starts at timestamp `t=101`, reads
   `k=0`, and writes a new version of it, say `v=1`, and commits.
1. `T` attempts a read of `k=0` and receives a transaction retry error
   (`ReadWithinUncertaintyInterval`) due to `C`'s write at `ts=101`.
1. `T` restarts at new timestamp `t=102` (this is an internal restart,
   i.e. the SQL client doesn't have to know).
1. Another transaction `C` starts at timestamp `t=103`, reads
   `k=0`, and writes a new version of it, say `v=2`, and commits.
1. `T` attempts to read `k=0` at `t=102`. This now succeeds (uncertainty
   restarts occur only once per node per txn), but it returns `v=1` and
   not the latest version, `v=2`.
1. `T` attempts to update `k=0` to write the value `v=2`, but receives a
   transaction retry error (`WriteTooOld` due to the existence of newer version
   written by `C` at `ts=103`). However, an intent is laid down.
1. `T` attempts to refresh its timestamp from `t=102` to `t=104`. This fails
   because of the value at `t=103`.
1. `T` restarts at `t=104` (this is still an internal restart; we're still
   in the first statement of the txn).
1. `T` reads `v=2` from `k=0` at `t=104`. No other transaction could have read or written
   to `k=0` in the meantime, due to the intent. The exception is if a
   higher-priority transaction had decided to abort `T`, but let's assume this is
   not the case.
1. `T` updates `k=0` to `v=3`.
1. `T` proceeds to the second `UPDATE` statement.

We'll pause her for a moment. So far, we've only managed to get through the first
of three statements in the transaction, but a lot more has happened. While in
the uncontended case, we round-tripped to the leaseholder twice to get the first
`UPDATE` statement in place, we have already done so six times to cover the same
distance.

Instead of writing down the second `UPDATE` statement in the same minute detail,
we take a more high-level view. Roughly speaking, we'll hit exactly the same
problems (mod the uncertainty restart): the first read will return a non-latest
version; the write will get `WriteTooOld` but lay down the intent; the refresh
will fail and so we will restart the txn; we'll read again, and finally lay down
the "final" intent. However, this restart will be visible to the client and so
it will have to run the transaction again from the beginning (but all intents
remain in place). Asymptotically, we see that with `N` such `UPDATE` statements,
we will have

```
num_roundtrips(N) = 
  1                                  // read (gets non-latest version)
+ 1                                  // pipelined write (gets `WriteTooOld`)
+ 1                                  // failed refresh attempt (if N>0)
+ num_roundtrips(N-1)                // (uncontendedly) redo previous stmts
+ 1                                  // read (gets latest)
+ 1                                  // pipelined write
```

or,

```
num_roundtrips(N) = 5   + num_roundtrips(N-1)
                  = 5*N + num_roundtrips(0)
                  = 5*N + 4
```

Moreover, for the number of txn retries that have to be carried out at the
client (via `SAVEPOINT cockroach_restart`) we have

```
num_ext_retry(N) = N-1
```

That is, each of the `UPDATE` statements (but the first) will force the
transaction to retry once.



our current model has shortcomings
here's the example, it's contrived but not overly so
we'll sample some real queries later


How do we teach this?

Explain the proposal as if it was already included in the project and
you were teaching it to another CockroachDB programmer. That generally means:

- Introducing new named concepts.
- Explaining the feature largely in terms of examples. Take into account that a product manager (PM) will want to connect back the work introduced by the RFC with user stories. Whenever practical, do ask PMs if they already have user stories that relate to the proposed work, and do approach PMs to attract user buy-in and mindshare if applicable.
- Explaining how CockroachDB contributors and users should think about
  the feature, and how it should impact the way they use
  CockroachDB. It should explain the impact as concretely as possible.
- If applicable, provide sample error messages, deprecation warnings, or migration guidance.
- If applicable, describe the differences between teaching this to
  existing roachers and new roachers.

For implementation-oriented RFCs (e.g. for core internals), this
section should focus on how contributors should think about
the change, and give examples of its concrete impact. For policy RFCs,
this section should provide an example-driven introduction to the
policy, and explain its impact in concrete terms.

# Reference-level explanation

This is the technical portion of the RFC. Explain the design in sufficient detail that:

(You may replace the section title if the intent stays clear.)

- Its interaction with other features is clear.
- It covers where this feature may be surfaced in other areas of the product
   - If the change influences a user-facing interface, make sure to preserve consistent user experience (UX). Prefer to avoid UX changes altogether unless the RFC also argues for a clear UX benefit to users. If UX has to change, then prefer changes that match the UX for related features, to give a clear impression to users of homogeneous CLI / GUI elements. Avoid UX surprises at all costs. If in doubt, ask for input from other engineers with past UX design experience and from your design department.
- It considers how to monitor the success and quality of the feature.
   - Your RFC must consider and propose a set of metrics to be collected, if applicable, and suggest which metrics would be useful to users and which need to be exposed in a public interface.
   - Your RFC should outline how you propose to investigate when users run into related issues in production. If you propose new data structures, suggest how they should be checked for consistency. If you propose new asynchronous subsystems, suggest how a user can observe their state via tracing. In general, think about how your coworkers and users will gain access to the internals of the change after it has happened to either gain understanding during execution or troubleshoot problems.
- It is reasonably clear how the feature would be implemented.
- Corner cases are dissected by example.

The section should return to the examples given in the previous
section, and explain more fully how the detailed proposal makes those
examples work.

## Detailed design

What / how.

Outline both "how it works" and "what needs to be changed and in which order to get there."

Describe the overview of the design, and then explain each part of the
implementation in enough detail that reviewers will be able to
identify any missing pieces. Make sure to call out interactions with
other active RFCs.

## Drawbacks

Why should we *not* do this?

If applicable, list mitigating factors that may make each drawback acceptable.

Investigate the consequences of the proposed change onto other areas of CockroachDB. If other features are impacted, especially UX, list this impact as a reason not to do the change. If possible, also investigate and suggest mitigating actions that would reduce the impact. You can for example consider additional validation testing, additional documentation or doc changes, new user research, etc.

Also investigate the consequences of the proposed change on performance. Pay especially attention to the risk that introducing a possible performance improvement in one area can slow down another area in an unexpected way. Examine all the current "consumers" of the code path you are proposing to change and consider whether the performance of any of them may be negatively impacted by the proposed change. List all these consequences as possible drawbacks.

## Rationale and Alternatives

This section is extremely important. See the
[README](README.md#rfc-process) file for details.

- Why is this design the best in the space of possible designs?
- What other designs have been considered and what is the rationale for not choosing them?
- What is the impact of not doing this?

## Unresolved questions

- What parts of the design do you expect to resolve through the RFC
  process before this gets merged?
- What parts of the design do you expect to resolve through the
  implementation of this feature before stabilization?
- What related issues do you consider out of scope for this RFC that
  could be addressed in the future independently of the solution that
  comes out of this RFC?
