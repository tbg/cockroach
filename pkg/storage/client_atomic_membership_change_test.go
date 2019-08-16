// Copyright 2019 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package storage_test

import (
	"context"
	"sort"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/base"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/storage"
	"github.com/cockroachdb/cockroach/pkg/testutils"
	"github.com/cockroachdb/cockroach/pkg/testutils/testcluster"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestAtomicReplicationChange is a simple smoke test for atomic membership
// changes.
func TestAtomicReplicationChange(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	args := base.TestClusterArgs{
		ServerArgs: base.TestServerArgs{
			Knobs: base.TestingKnobs{
				Store: &storage.StoreTestingKnobs{},
			},
		},
		ReplicationMode: base.ReplicationManual,
	}
	tc := testcluster.StartTestCluster(t, 6, args)
	defer tc.Stopper().Stop(ctx)

	// Create a range and put it on n1, n2, n3. Intentionally do this one at a
	// time so we're not using atomic replication changes yet.
	k := tc.ScratchRange(t)
	expDesc, err := tc.AddReplicas(k, tc.Target(1))
	require.NoError(t, err)
	expDesc, err = tc.AddReplicas(k, tc.Target(2))
	require.NoError(t, err)

	// Run a fairly general change.
	chgs := []roachpb.ReplicationChange{
		{ChangeType: roachpb.ADD_REPLICA, Target: tc.Target(3)},
		{ChangeType: roachpb.ADD_REPLICA, Target: tc.Target(5)},
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(2)},
		{ChangeType: roachpb.ADD_REPLICA, Target: tc.Target(4)},
	}

	runChange := func(expDesc roachpb.RangeDescriptor, chgs []roachpb.ReplicationChange) roachpb.RangeDescriptor {
		t.Helper()
		desc, err := tc.Servers[0].DB().AdminChangeReplicas(
			// TODO(tbg): when 19.2 is out, remove this "feature gate" here and in
			// AdminChangeReplicas.
			context.WithValue(ctx, "testing", "testing"),
			k, expDesc, chgs,
		)
		require.NoError(t, err)
		return *desc
	}

	expDesc = runChange(expDesc, chgs)

	var stores []roachpb.StoreID
	for _, rDesc := range expDesc.Replicas().All() {
		if rDesc.GetType() != roachpb.ReplicaType_VOTER {
			t.Fatalf("found a non-VOTER: %+v", expDesc)
		}
		stores = append(stores, rDesc.StoreID)
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i] < stores[j] })
	exp := []roachpb.StoreID{1, 2, 4, 5, 6}
	require.Equal(t, exp, stores)

	// Nodes may apply the change at different times, so we need a retry loop.
	testutils.SucceedsSoon(t, func() error {
		// TODO(tbg): idx=2 should also receive the joint descriptor.
		for _, idx := range []int{0, 1, 3, 4, 5} {
			// Verify that all replicas left the joint config automatically (raft does
			// this and ChangeReplicas blocks until it has).
			repl, err := tc.Servers[idx].Stores().GetReplicaForRangeID(expDesc.RangeID)
			require.NoError(t, err)
			act := repl.RaftStatus().Config.String()
			exp := "voters=(1 2 4 5 6)"
			if exp != act {
				return errors.Errorf("wanted: %s\ngot: %s", exp, act)
			}
		}
		return nil
	})

	// Rebalance back down all the way.
	chgs = []roachpb.ReplicationChange{
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(1)},
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(3)},
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(4)},
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(5)},
	}

	runChange(expDesc, chgs)

	// Rebalance back down all the way.
	chgs = []roachpb.ReplicationChange{
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(1)},
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(3)},
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(4)},
		{ChangeType: roachpb.REMOVE_REPLICA, Target: tc.Target(5)},
	}

	runChange(expDesc, chgs)
}
