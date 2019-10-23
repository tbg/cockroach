CockroachDB's transaction protocol has to operate in a difficult environment. CockroachDB is

- **global**: clusters span the world, and network latencies between them are high.
- **SQL**: interactive reads and writes.
- **no stale reads**: everything is transactional at a high consistency level.

In a global deployment, some fraction of business transactions will be global and it is critical that inter-node latencies are avoided wherever possible.

Let's develop an intuition on the current state with a few examples.

All of them take place across two regions NYC and SF, both of with a number of nodes (footnote: a real deployment will have 3+ regions, but it doesn't matter here). We assume that all activity originates from a client in NYC and assume that the client-gateway latency is zero and that no time is spent on the actual processing (i.e. infinitely fast CPUs and IO). For illustrative purposes, we assume that (round-trip) latencies between nodes in a region are ~1ms, and between regions ~100ms.

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
Let's assume we can be stale if that precludes the commit. In that case, once we've waited out b), we can run all reads against local followers, provided that we then refresh at the (remote) leaseholder (and also verify the uncertainty interval in the process). This would bring

```
BEGIN
n x READ(sf.global);
COMMIT;
```

down from `n * 100` to `ctdelay + n * 0 + 1 * 100`; we're effectively giving out "optimistic" results to the client and verify them all in parallel during `COMMIT`. With a closed timestamp duration of 50ms, this boils down to `1.2s` vs `200ms`, i.e. a clear win.

This in itself is already a proposal worth considering, but before we give it a name, let's try to improve it even more: can we avoid having to wait for a closed timestamp? After all, we're already serving potentially stale data until we try to COMMIT, so couldn't we make the data "staler" so that we never have to wait for an initial closed timestamp?

In fact, this is possible and wouldn't need invasive changes from where we are today. Transactions already have a distinction between the "read" and "provisional commit" (=write) timestamps. Instead of initializing a transaction with `read=write=now()`, we can start out with `write=now(), read=now()-ctdelay=now()-50ms-T`. This means that our read is more likely to fail the final refresh, which will now cover a larger period of time. In turn, we've arrived at what is clearly the minimum latency we could have hoped for!

One downside of this extended proposal is that we have to bifurcate between two txn protocols before we know which one is preferrable. If a transaction is all local reads, we don't want to mess with its read timestamp - we want to be able to delay the optimistic reads until we see the first request to read remotely, and then move our read timestamp back to `now()-ctdelay`. This may seem weird, but it's fine - we just have to make sure that before we start using this new read timestamp, all previous reads are still valid. This just means that we have to refresh "backwards" in time, over the interval `[now()-ctdelay,now()]`. This has to happen synchronously (in parallel), but because all preceding reads were local, it's very cheap given that we get to avoid a WAN latency in return.

We call the resulting proposal **optimistic follower reads**. If we were discussing read-only transactions, we'd be ready to formalize it. However, read-write transactions are critical, too. They add more complexity to the final proposal, so we discuss them first.

## Optimistic follower reads.


