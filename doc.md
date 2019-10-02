Here's a concrete proposal that I find quite compelling, please poke some holes in it. This came out of this week's 1:1 with Andrei and I think it's basically write-snapshot isolation (TODO find the paper again).

The problem with contending read-modify-update statements is that we'll get `ReadWithinUncertaintyIntervalError` and `WriteTooOldError`. We get the former if we're reading at a timestamp that's lower than the most recent version of the key. We get the latter when trying to write at such a timestamp. In both cases, a refresh will fail.

If we can make refreshes always succeed, we don't even have to do them, so they become free (and our txns latency-optimal).

This seems like an outlandish thing to achieve at first, but it seems that it's exactly what we get if for all keys touched we acquire **hardened write locks**, where

- **hardened**: they're not optimistic locks, i.e. we know when we hold the lock and don't have to check before commit. Note that laying down an intent but not waiting for replication is *not* hardened, as the intent may never appear and other writers may update the row, invalidating refreshes - but see later on how this could be made work.
- **write**: only the txn holding the lock for a key (range) can lay down intents there. Only one txn can hold the write lock for any given span. Note that we're not blocking concurrent readers (unless they're trying to get the same lock), which has interesting consequences discussed later.

I think hardened write locks are straightforward to implement (see later section), let's assume for now that we have them and that they can be acquired as a side effect of a read or pipelined write (i.e. we never wait for replication) and that they don't need to be made durable. In effect, these locks are cheap.

Now consider the following transaction:

```sql
BEGIN;
UPDATE tbl SET v=v+1 WHERE k = 1;
UPDATE tbl SET v=v+1 WHERE k = 2;
UPDATE tbl SET v=v+1 WHERE k = 3;
COMMIT;
```

which runs in an extremely contended environment in which all keys this txn is trying to access are hammered by short-lived reading and/or writing txns. I consider this a fairly general relevant case of contention: the contending transactions have nothing in common, so we can't reorder statements to front-load all of the contention into the first statement (if we can handle this contention, we don't have to explain to users how to rearrange their app); and there is lots of read-modify-update (which ORMs like to do, and so we don't have to teach our users to not use ORMs which is likely a fool's errand anyway). And last but not least, there's more than one statement, so the users *will* see the retry errors should they occur, just as they do in real life - if they didn't occur we would certainly appreciate that a lot.

We make the assumption that the response to acquiring a hardened write lock contains the most recent version (i.e. timestamp and value) of the covered key(s), which will be essential below.

At the KV level, the above transaction roughly becomes

```go
txn.Begin()
for k := 0; k < 3; i++ {
    // NB: if we had a programmable read-modify-write op we'd save a round-trip
    // to the leader here, but that's not the tree I'm barking up today.
    txn.Put(k, txn.Read(k)+1)
}
txn.Commit()
```

or, making explicit that we're acquiring hardened write locks in `txn.Read`,

```go
txn.Begin()
for k := 0; k < 3; i++ {
    txn.Put(k, txn.ReadAndAcquire(k)+1)
}
txn.Commit()
```

Let's say the transaction's timestamp is originally 100. When it reads `k=1`, after waiting for intents to get resolved, there's likely a committed version at a higher timestamp, say 110. The read, which would either be at risk of a `ReadWithinUncertaintyIntervalError` or would read at a timestamp useless for a later update to the key does acquire the hardened write lock. From the result, the txn gateway learns that a refresh will be necessary, and it now knows the timestamp of the most recent version *and* holds the lock guaranteeing that this version won't change. Thus it can refresh in-place, moving the read timestamp forward to `111` (or any timestamp higher than `110`, probably just `clock.Now()`). Then, it writes successfully to the same key.

In the next iteration `k=1`, the same procedure repeats. By the same argument, we can refresh `k=1` in place. But we can also refresh `k=0` in-place because we know that the refresh will succeed; we hold the lock. Iteratively, we thus know that we can freely increase our commit timestamp as we please, recovering how snapshot isolation used to work before we removed it, except we're still serializable.

This has the interesting consequence that contending transactions not acquiring hardened write locks (for example read-only txns) can freely push a txn that they know will have hardened write locks for all of its accesses.
The hardened lock (if it's a point lock) could be removed optimistically if an intent is known to have been realized (i.e. as a side effect of `QueryIntent` or even of raft application). This is because once the intent is known to be replicated, it can't be removed until its txn has terminated.

We also don't have to take hardened write locks for all accesses. However, those key spans for which they're not held need to be explicitly refreshed before committing, and this refresh may fail, requiring a txn restart.

It's also worth pointing out that if there is no contention, there's no overhead to acquiring the locks other than the memory maintained at the leaseholder.

For completeness, consider how the above txn fares without the locking. Each read will either cause an uncertainty restart or the later write will fail with `WriteTooOld`, and the refresh will always fail, meaning that we're looking at one restart per statement (each write will go through on the second attempt because we lay down an intent in the first).

We can also shoehorn the "try your best to lay down a placeholder intent on each read, but don't wait for it to appear" strategy into something workable to substitute hardened write locks: we don't know that refreshes of these keys will succeed until we've proven that the intent actually showed up. However, we need to prove that anyway before we commit (`QueryIntent`); so it will work out to just assume that refreshes always succeed. However, note the large drawback here: we can't lay down ranged intents, so we lose the nice property that a txn that protects all of its accesses (incl ranged reads) can be pushed and will still commit. Laying down intents just as placeholders is also more expensive than doing something in-memory at the leaseholder. As a third drawback, I find this harder to understand than just implementing lightweight locks that have clear semantics. Still, this isn't a non-starter proposal.

So how are these locks implemented?

### Hardened write locks

Hardened write locks (which can be point or ranged) are held in memory at the leaseholder and are tied to the liveness epoch of the leaseholder node. Transactions keep track of the epochs for which they have acquired locks. For example, if `txn.AcquireWriteLock(someKey)` is served under a lease held by n5 at liveness epoch 7, the txn coordinator remembers `(n5, epoch=7)`. Before attempting to commit, it must check that n5 is still live at epoch 7. The locks are released as a side-effect of committing/aborting ResolveIntent; when n5 discovers observes such a request it may remove all locks of the txn across its leaseholders. Locks belonging to what is known to be a past epoch can also be dropped.

The lock structure needs to be passed along during lease transfers. If a liveness epoch fails, transactions holding locks tied to it must restart (more precisely, if all leaseholders are known to have been live at the recorded epoch for the final commit timestamp, the commit can succeed). A new leaseholder stepping up after incrementing the previous leaseholder's epoch starts out with no locks.

Attempting to acquire a lock which is already held blocks.

As mentioned above, for the specific use case of contention control it appears reasonable to allow readers to bypass the exclusive locks, unless the readers also wanted to acquire an exclusive lock. This means that read-only transactions would remain unaffected by exclusive locking.
