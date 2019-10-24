CockroachDB's transaction protocol has to operate in a difficult environment. CockroachDB is

- **global**: clusters span the world, and network latencies between them are high.
- **SQL**: interactive reads and writes.
- **no stale reads**: everything is transactional at a high consistency level.

In a global deployment, some fraction of business transactions will be global and it is critical that inter-node latencies are avoided wherever possible.

Through examples, we will develop a grasp on the currently expected latencies, and come up with a number of impactful optimizations.

All of the examples take place across two regions NYC and SF, both of with a number of nodes (footnote: a real deployment will have 3+ regions, but it doesn't matter here). We assume that all activity originates from a client in NYC and assume that the client-gateway latency is zero and that no time is spent on the actual processing (i.e. infinitely fast CPUs and IO). For illustrative purposes, we assume that (round-trip) latencies between nodes in a region are ~1ms, and between regions ~100ms.

In the real world, most tables have indexes and thus writes touch more than one range, meaning that we can't use fast-paths that elide the full txn protocol, as we will assume throughout.

We use schematic tables `{nyc,sf}_{global,partitioned}`. The global flavor requires global replication, i.e. a round-trip between NYC and SF, to replicate a write, while a partitioned table requires only a local ~1ms round-trip. The first component of the table name indicates the location of the leaseholder.

For example, the below read-only transaction takes 2ms:

```
BEGIN;
READ(nyc.global);      -- 1ms
READ(nyc.partitioned); -- 1ms
COMMIT;
```

This is because we assume that the client is in NYC, and because reading from the local leaseholder requires no replication. However, if we read from SF we'll start seeing significant delays:

```
BEGIN;
READ(sf.global);      -- 100ms
READ(sf.partitioned); -- 100ms
COMMIT;
```

This transaction performs two back-to-back reads and each takes 100ms; a similar transaction with 12 reads would take 1.2s. footnote(This is if the client waits for each read's response before issuing the next read (as they usually do)).

We are starting to get a sense that it can be disastrous to read data from remote leaseholders. We usually address by making sure it doesn't happen:

If the table can be partitioned or moved into a single region that accesses it wholesale, do that.
Otherwise, i.e. for data that may be read from multiple locations alike, for each region, we add an index that is held in this region only. That is, we denormalize and trade latency for storage and IO.

Unfortunately, in general we're also compromising availability for all writes to the table in this way. This is because inserts into the table need to update *all* indexes, but the indexes are not available during a region failure. However, if the table is partitioned, we can set up the indexes so that for a write originating from, say, NY, the only index that needs updating is also in NY.

That leaves `{nyc,sf}.global` as the most problematic table to ponder and is a good opportunity to introduce an idea that will return in the proposal later. The global tables have local replicas in both regions; it almost seems as though we should try to read from the local follower if the leaseholder is not local. We can't be too naive here because our transaction a) can't have stale reads and b) can't see inconsistent data.

b) is not negotiable, but fortunately we have closed timestamps: if we block reads at the local followers until the read timestamp is closed out, we will be reading from a consistent snapshot. Assuming a closed timestamp target duration of `T`, this would require blocking for `ctdelay := 0.5*100ms + T`, the first term being the time it takes the closed timestamp to travel from the remote leaseholder to the local region.

a) requires a bit of soul-searching. We certainly don't want a transaction to commit successfully with a stale read, but it could be acceptable to defer the staleness check to the commit phase. If we're strict, we have to modify a) so that we're waiting out the MaxOffset as well (or round-trip to the leaseholder once to lower the MaxOffset), which hurts because that (currently) adds 500ms.
Let's assume we can be stale if that precludes the commit. In that case, once we've waited out b), we can run all reads against local followers. Additionally, we cannot return from COMMIT until we've checked for reads in the uncertainty interval, which we can do locally if the transaction's max timestamp has already closed out (i.e. if the txn has been open for at least `ctdelay + MaxOffset`) or by contacting the leaseholders. In a world in which the closed timestamp and MaxOffset are "morally" zero, read-only transactions simply wait for half a WAN roundtrip - once - and can then read locally to their heart's delight. However, neither will be true in the foreseeable future, and throughout this document we will assume that the proactive approach is the better alternative (long-running transactions can perform the checks locally and recover the optimal behavior in this way). The resulting latency will be

```
BEGIN
n x READ(sf.global); -- ctdelay + n * 1ms
(uncertainty check)  -- 100ms
COMMIT;
```

down from `n * 100` to `ctdelay + n * 0 + 1 * 100`; we're effectively giving out "optimistic" results to the client and verify them all in parallel during `COMMIT`. With a closed timestamp duration of 50ms, this boils down to `1.2s` vs `200+n ms`, i.e. a clear win.

This in itself is already a proposal worth considering, but before we give it a name, let's try to improve it even more: can we avoid having to wait for a closed timestamp? After all, we're already serving potentially stale data until we try to COMMIT, so couldn't we make the data "staler" so that we never have to wait for an initial closed timestamp?

In fact, this is possible and wouldn't need invasive changes from where we are today. Transactions already have a distinction between the "read" and "provisional commit" (=write) timestamps. Instead of initializing a transaction with `read=write=now()`, we can start out with `write=now(), read=now()-ctdelay=now()-50ms-T`. This means that our read is more likely to fail the final refresh, which will now cover a larger period of time. In turn, we've arrived at what is clearly the minimum latency we could have hoped for, just one WAN hop (plus local contributions).

One downside of this extended proposal is that we have to bifurcate between two txn protocols before we know which one is preferrable. If a transaction is all local reads, we don't want to mess with its read timestamp - we want to be able to delay the optimistic reads until we see the first request to read remotely, and move our read timestamp back to `now()-ctdelay` for it. This may seem weird, but it's fine - we just have to make sure that before we start using this new read timestamp, all previous reads are revalidated. This just means that we have to refresh "backwards" in time, over the interval `[now()-ctdelay,now()]`. This has to happen synchronously (in parallel), but because all preceding reads were local, it's very cheap given that we get to avoid a WAN latency in return.

We will call the resulting proposal **optimistic follower reads**. If we were discussing read-only transactions only, we'd be ready to formalize it. However, read-write transactions are critical, too. They add more complexity to the final proposal, so we discuss them first.

Let's start simple:

```
BEGIN
WRITE(nyc.partitioned); -- 1ms
COMMIT;           -- 2ms
```

This is already a lot harder to understand. Naively, we'd expect the `WRITE` to take 2ms: one 0.5ms from gateway to the leaseholder, 1ms for the leaseholder to replicate the write to a follower (committing the command), 0.5ms back (COMMIT is "just a replicated write", so this explains why that does take 2ms). We didn't take into account Parallel Commit: the WRITE really only **triggers** the replicated write at the leaseholder, but doesn't block on it (this is called a "pipelined write"); verifying that the write has gone through is deferred to the `COMMIT` statement. So we get the following sequence of events:

http://www.plantuml.com/plantuml/uml/ZOv1IyD048Nlyok6d1HfsLMyU920q22Kebv2ZqiowC3iRiXkg4NyxqwQfcgzU5bstdpllIbJTdqUl40JGyE9iAXSfftR5-WILlMtlewD4roJI_GMfQN-G6osvyGgYiJTQRq247gbq49cJtZXqoNeX4SHade8xGXROx3ZTv84K1geQkI4nHDt91oenRhdJCMeB-urkRmooribntUpFR0lr0atJYbL9cerOTDOrRF97kCVelUSGRcVeUSLzZyLysUssgNvNOjt-3nGltyCcKBMsEjJxCVYyy-9Dth6l2ifj8ENBm00

t=0.0ms: gateway sends write to leaseholder
t=0.5ms: leaseholder initiates replication of write
t=1.0ms: gateway receives confirmation that command was accepted (footnote: needed for unique violations etc)
         gateway sends EndTransaction and CheckIntent (in parallel)
t=1.5ms: write replication completes
t=1.5ms: leaseholder initiates replication of EndTransaction
t=1.5ms: leaseholder responds to QueryIntent
t=2.0ms: gateway receives QueryIntentResponse
t=2.5ms: leaseholder commits EndTransaction
t=3.0ms: gateway receives EndTransactionResponse

Parallel Commits has effectively deferred the replication of the write into the "background", allowing the transaction to move on to the next statement. Directing the write at `nyc.global`, we instead obtain

```
BEGIN
WRITE(nyc.global); -- 1ms
COMMIT;            -- 101ms
```

This is because the QueryIntent check for the pipelined write has to wait until the WAN replication has completed. This doesn't look great until you remember that it's optimal. There's no way we can do better than that - we get to carry out any number of globally replicated writes at essentially the cost of just one. This is really good and shows just how important Parallel Commit is.

Finally, in the worst case, we are writing into `sf.global`:

```
BEGIN
WRITE(nyc.global); -- 100ms
COMMIT;            -- 200ms
```

This time, the simple round-trip to the leaseholder alone takes 100ms, and then we immediately turn around and contact the remote lease-holder again, except that it will also block us for 100ms because it has to wait for a global round-trip itself. Even worse, additing additional `nyc.global` writes would each set us back another 100ms. We'll find ways to improve this later, but for now we just accept it. The other new latency here is that COMMIT now takes two WAN round trips; replication alone takes 100ms, but it's only triggered by a message that needs to cross an ocean first. In this simple case, the transaction could have chosen to anchor itself on a global range lead by a local replica (reducing the commit latency to just one WAN hop), but we'll assume here and below that the Parallel Commit contains additional globally replicated writes that prevent this optimization (which is finicky anyway, since the anchoring key must be chosen with the first write already).

Nothing new happens if we throw a few local reads together with `WRITE(nyc.partitioned)`:

```
BEGIN;
READ(nyc.partitioned);   -- 1ms
READ(nyc.partitioned);   -- 1ms
WRITE(nyc.partitioned);  -- 1ms
COMMIT;                  -- 2ms
```

Note that the latencies here are still optimal, so we don't want to add optimism (as discussed earlier) to the reads. However, moving on to global reads, we do start wanting that:

```
BEGIN;
READ(nyc.global);       -- 100ms
READ(nyc.global);       -- 100ms
WRITE(nyc.partitioned); -- 1ms
COMMIT;                 -- 2ms
```

This is very close to an earlier read-only example: we're hopping across the ocean back-to-back for a total latency that's linear in the number of reads, and we would really rather serve the reads via a closed-out timestamp from a local follower in NYC, and make the commit operation conditional on being able to refresh all of the reads in parallel:

```
BEGIN;
READ(nyc.global);       -- 1ms (optimistic)
READ(nyc.global);       -- 1ms (optimistic)
WRITE(nyc.partitioned); -- 1ms
(refresh)               -- 100ms
COMMIT;                 -- 2ms
```

We are now optimal again in the sense that there's only one WAN round trip, and this round trip is necessary.

Let's move on to the worst offender from before, involving `nyc.global`, now for both reads and writes, applying optimistic follower reads right away:

```
BEGIN
READ(nyc.global)  -- 1ms (optimistic)
READ(nyc.global)  -- 1ms (optimistic)
WRITE(nyc.global) -- 100ms
(refresh)         -- 100ms
COMMIT;           -- 200ms
```

Here, optimistic follower reads makes sure that each individual global reads is paid for in local latency only, at the expense of a single extra WAN replication round trip before the COMMIT. The savvy reader may wonder whether the refresh can somehow be folded into the Parallel Commit phase. This is possible but far from trivial; we'll discuss it later, but let's assume we did do it:

```
BEGIN
READ(nyc.global)  -- 1ms (optimistic)
READ(nyc.global)  -- 1ms (optimistic)
WRITE(nyc.global) -- 100ms
COMMIT (refresh); -- 200ms
```

This is optimal except for the write latency. Pipelining defers the replication of the write into the background, blocking on these writes only for the duration of a read. We've gotten the latency for reads from `nyc.global` down by reading from local followers, and in fact we can do the same for the pipelined write: we evaluate the write on a suitable follower, and fire-and-forget the actual write to the remote leaseholder in the background; the refresh and QueryIntent will make sure that the transaction fails if either the outcome of the write is different from what we saw on our local follower or the write (at the leaseholder) failed for any reason.

TODO: lots!

- optimistic follower reads
- refreshing parallel commits (i.e. some kind of lock)
	- develop requirement for minimum locking (uncertainty+GC)
- optimistic follower pipelined writes
- txn record placement 
