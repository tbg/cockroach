# Compact Node-Based Transaction IDs

Today, we use UUIDs as transaction IDs. This document suggests that we should switch to a transaction ID scheme that identifies the gateway node and original transaction timestamp. This combination is guaranteed to be unique due to known clock constraints (for a version without clock constraints, see below) and has powerful consequences:

- Conflict resolution can contact the gateway node directly, without going through consensus via the transaction record. In particular, this makes it feasible to not write the transaction record until late in the life of a transaction (which should have measurable performance implications), and does away with a lot of the complexity around having to track the `Writing` flag which has devored countless engineering hours.
- MVCC versioned values contain a timestamp in the key which in the common case is equal or very close to the transaction's original timestamp, so in turn the overhead for storing the transaction ID in *committed* values becomes cheap (just add the NodeID part). This enables
  - CDC (so that committed values can be grouped into transactions)
  - [Parallel Commits](parallel-commit.md), thus shaving a consensus latency off most transactions (!)
  - A path towards idempotency, which simplifies the storage layer and should significantly reduce ambiguous results

## Details

We can switch to transaction IDs which are composed of

1. the NodeID
1. the transaction's original timestamp (which is unique per node even across restarts, thanks to our HLC guarantees and sleeping out the MaxOffset)

This allows for smaller versioned keys in the common case in which the transaction commits at its original timestamp, in which case the timestamp is omitted from the TxnID stored in the version and all we store is the NodeID.
When the transaction *was* pushed, we additionally store the delta to the base timestamp (i.e. if the value is at 1000 but the transaction was originally 200, we store a delta of 800).


### Avoiding Clocks

Relying on the max offset to keep transaction IDs unique can be undesired. Instead, each node can keep a local counter which is incremented every time the node starts, and is part of the transaction ID.

----

NB: the below is rambly and more of a brain dump than an actual tech note.

## Communicating with the txn coordinator directly

Having the `NodeID` in the transaction ID unlocks an alternative mechanism to handling transactional conflicts by contacting the coordinator directly.

For example, when finding a conflicting intent, the txn can send a (streaming) RPC to the remote coordinator, compare priorities, and either wait for the coordinator to signal completion or prompt it to abort the transaction.

This will often be faster (depending on latencies between the involved nodes), thanks to the absence of consensus and polling in this path, and could replace the txn wait queue, though deadlock detection needs to be taken care of.

When the coordinator isn't reachable within a heartbeat timeout (or however long we're willing to wait), abort the transaction the hard way.

NB: this would address [#20448].

### Phasing out HeartbeatTxn

Now that a coordinator is directly reachable, we can consider not sending HeartbeatTxn. The only reason for keeping it is that during exotic network partitions, some nodes may find their transactions aborted by other nodes unable to contact them.

## Idempotency

Today, the KV layer is not idempotent. For example, take a transaction that finishes with the following batch on a range:

```
CPut(k1)       // change k1 from v0 to v1
EndTransaction // commit
```

and the executing leaseholder dies while this RPC is in flight, so that it's unclear whether it applied or not.

If it did apply successfully, the intent at k1 would have been resolved automatically and the transaction record removed, so when retrying there is no way to tell whether it was our txn that actually wrote the value; we have to return an ambiguous result.

Now that we embed a transaction ID in each write (committed or not), we can do better if we also embed the sequence number used for the write (i.e. retain it from the intent) as we're now able to see that this is our prior write, and can execute the `CPut` as a no-op.
 
When we execute `EndTransaction` the current code would still fail as it requires the transaction record to be present (created by `BeginTransaction`), so let's look at that next.

### BeginTransaction

Why do we have `BeginTransaction` in the first place? Some light archaeology,

```
git log origin/master --oneline --grep BeginTransaction -- {pkg,.}/{storage,kv}
```

suggests that its [original raison d'être][#2062] was to prevent outdated heartbeats or pushes from recreating the transaction record (neither of these are a reason to keep it today).
An additional moving piece involving them is replay protection, where we populate the write timestamp cache for `BeginTransaction` to make sure that a 1PC batch can't apply more than once. This too would be addressed by having the txnID in committed values (for you'd recognize the replay that way and apply as a no-op). Eagerly creating the transaction record was also a concern in improving conflict resolution since in the absence of a record, conflicting writers could not communicate. However, this is more than adequately addressed by communicating directly with the coordinator.

All in all, this leads me to believe that there's no good reason to keep `BeginTransaction` around.

### The `Writing` Flag

This is one of the warts in the transaction machinery that has caused a long string of bugs. It is a flag that is *supposed to be* set on the transaction proto if and only if the transaction record has been created (which in turn is if and only if the transaction has laid down an intent). This is used in two ways: 1. to decide whether to insert a `BeginTransaction` into a batch in `client.Txn` and 2. to decide whether TxnCoordSender should be heartbeating the transaction record.

The problem is that this is impossible to track adequately given the current setup; the `Writing` status is updated above in `TxnCoordSender` which sits above `DistSender`, but `DistSender` splits up batches across ranges and so may end up writing a few intents but return with an error (not mentioning those intents).

Note that 1. immediately becomes obsolete if we remove `BeginTransaction`.
For 2., the heartbeat loop is a lot less important when conflicting transactions communicate directly. In fact, heartbeats are unnecessary unless we want to keep them as a fallback in the case of exotic network partitions, but then they are more of a best-effort affair and don't need to be tracked as closely.

Combining these, I think we can remove the `Writing` flag along with `BeginTransaction`, though we may find a reason to to want to move the abort cache protection to `EndTransaction` instead (may not be necessary with the new-found idempotence).

### Detecting Replays

This is straightforward but worth spelling out. We currently embed a sequence number into each intent for which we guarantee that it increases monotonically per key for each write. So trying to overwrite an intent with an equal or lower sequence number results in an error; this catches cases in which a previous write from the same epoch gets stuck in the RPC subsystem and clobbers a newer one (which would result in lost updates).

See [#20448] for a related discussion.

[#20448]: https://github.com/cockroachdb/cockroach/issues/20448#issuecomment-373611915
[#2062]: https://github.com/cockroachdb/cockroach/issues/2062#issuecomment-144437252
