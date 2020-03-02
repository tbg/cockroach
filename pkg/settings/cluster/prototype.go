// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package cluster

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/internal/client"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
)

type Migration func(*Helper) error

type Helper struct {
	db *client.DB
	// lots more stuff. Including facility to run SQL migrations
}

func (h *Helper) IterRangeDescriptors(
	f func(...roachpb.RangeDescriptor) error, blockSize int,
) error {
	// Paginate through all ranges and invoke f with chunks of size ~blockSize.
	// call h.Progress between invocations.
	return nil
}

func (h *Helper) Retry(f func() error) error {
	for {
		err := f()
		if err != nil {
			continue
		}
		return err
	}
}

func (h *Helper) RequiredNodes() []roachpb.NodeID {
	// TODO: read node registry and return list of nodes to be taken into
	// account.
	_ = h.db
	return nil
}

func (h *Helper) Progress(s string, num, denum int) {
	// Set progress of the current step to num/denum. Denum can be zero if final
	// steps not known (example: iterating through ranges)
	// Free-form message can be attached (PII?)
	//
	// TODO: this API is crap but you get the idea
}

func (h *Helper) EveryNode(op string, args ...interface{}) error {
	for _, nodeID := range h.RequiredNodes() {
		// dial nodeID and send with proper args (in parallel):
		_ = nodeID
		_ = EveryNodeRequest{action: op}
		// Also return a hard error here if a node that is required is down.
		// There's no point trying to upgrade until the node is back and by
		// handing control back to the orchestrator we get better UX.
		//
		// For any transient-looking errors, retry. For any hard-looking errors,
		// return to orchestrator (which will likely want to back off; migrating
		// in a hot loop could cause stability issues).
	}
	return nil
}

type V struct {
	roachpb.Version
	Migration
}

// Stub for proper proto-backed request type.
type EveryNodeRequest struct {
	action string // very much not real code
}

type magicJob struct {
	// not sure what will go on here, probably a *Job and then some more
}

// Done with num of denum items, message s.
func (j *magicJob) SetStatus(s string, num, denum int) error {
	return nil
}

func RunMigrations(j *magicJob) error {
	// This gets called by something higher up that knows how owns the job, and
	// can set it up, etc.

	ctx := context.Background()
	var db *client.DB // from somewhere
	ClusterVersionKey := roachpb.Key("TODO")
	var ackedV roachpb.Version
	if err := db.GetProto(ctx, ClusterVersionKey, &ackedV); err != nil {
		return err
	}

	// Hard-coded example data. From 20.1, will run the following migrations, not
	// all of which may have a Migration attached. Assembled from a registry while
	// taking into account ackedV
	vs := []V{{Version: roachpb.Version{Major: 20, Minor: 1}, Migration: func(*Helper) error { return nil }}}
	_ = ackedV

	for _, v := range vs {
		h := &Helper{}
		// h.Progress should basically call j.Status(v.String()+": "+s, num, denum)

		// Persist the beginning of this migration on all nodes. Basically they will
		// persist the version, then move the cluster setting forward, then return.
		// Note that they don't store whether the migration is ongoing. I don't think
		// we need that but maybe we will? Hmm yeah it seems reasonable that we will.
		// We might have functionality that is only safe when all nodes in the cluster
		// have done their part to migrate into it, and which we don't want to delay
		// for one release. Hmm, but then we could just gate that functionality's
		// activation on a later unstable version and we'd get the right behavior.
		// Ok so maybe we don't need more than this.
		if err := h.EveryNode("ack-pending-version", v); err != nil {
			return err
		}
		if v.Migration != nil {
			if err := v.Migration(h); err != nil {
				return err
			}
		}
	}
	return nil
}

func MigrationUnreplicatedTruncatedState(h Helper) error {
	if err := h.IterRangeDescriptors(func(descs ...roachpb.RangeDescriptor) error {
		// Really AdminMigrate but it doesn't exist yet. It takes an argument
		// determining the range version to migrate into. Unsure if that's a
		// different kind of version or just `h.Version()`. I think I lean
		// towards the latter.
		_, err := h.db.Scan(context.TODO(), descs[0].StartKey, descs[len(descs)-1].EndKey, -1)
		return err
	}, 500); err != nil {
		return err
	}
	if err := h.EveryNode("forced-replicagc"); err != nil {
		return err
	}
	return nil
}

func MigrationGenerationComparable(h Helper) error {
	// Hmm now that I look at this again I think we can remove it just like that.
	// No snapshots are in flight any more that don't respect the generational
	// rules. Replicas get replicaGC'ed eagerly. (Once round of forced-replicagc
	// somewhere is enough). Need to think this through more.
	return nil
}

func MigrationSeparateRaftLog(h Helper) error {
	// Impl will loop through all repls on the node and migrate them one by one
	// (get raftMu, check if already copied, if not copy stuff, delete old stuff,
	// release raftMu).
	if err := h.EveryNode("move-raft-logs"); err != nil {
		return err
	}
	// TODO: sanity check that all are really using separate raft log now?
	return nil
}

func MigrationReplicatedLockTable(h Helper) error {
	// Pretty vanilla Raft migration, see https://github.com/cockroachdb/cockroach/issues/41720#issuecomment-590817261
	// also run forced-replicagc.
	// Should probably bake the forced replicaGC and migrate command into a
	// helper, there's never a reason not to run forced-replicaGC after a Migrate.
	return nil
}

func MigrateReplicasKnowReplicaID(h Helper) error {
	// Is a migration necessary here or are we already nuking them at startup anyway?
	// Oh seems like it, so nothing to do here:
	// https://github.com/cockroachdb/cockroach/blob/f0e751f00b7ba41f39dd83ddc8238011fa5b9c19/pkg/storage/store.go#L1347-L1350
	return nil
}
