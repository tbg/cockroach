package kvserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/base"
	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/kv/kvserver"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/server"
	"github.com/cockroachdb/cockroach/pkg/testutils"
	"github.com/cockroachdb/cockroach/pkg/testutils/testcluster"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/stretchr/testify/require"
)

func TestResetQuorum(t *testing.T) {
	defer leaktest.AfterTest(t)()
	testutils.RunTrueAndFalse(t, "withUnabortableIntent", testResetQuorumImpl)
}

func testResetQuorumImpl(t *testing.T, withUnabortableIntent bool) {
	ctx := context.Background()

	args := base.TestClusterArgs{
		ReplicationMode: base.ReplicationManual,
	}
	tc := testcluster.StartTestCluster(t, 2, args)
	defer tc.Stopper().Stop(ctx)

	// Set up a scratch range isolated to n2.
	k := tc.ScratchRange(t)
	desc, err := tc.AddReplicas(k, tc.Target(1))
	require.NoError(t, err)
	require.NoError(t, tc.TransferRangeLease(desc, tc.Target(1)))
	desc, err = tc.RemoveReplicas(k, tc.Target(0))
	require.NoError(t, err)
	require.Len(t, desc.Replicas().All(), 1)

	srv := tc.Server(0)
	require.NoError(t, srv.DB().Put(ctx, k, "i will be lost"))
	if withUnabortableIntent {
		// Simulate an open replication change: a txn anchored to k's range descriptor
		// with a second intent on k's meta2 key.
		txn := srv.DB().NewTxn(ctx, "foo")
		require.NoError(t, txn.Put(ctx, keys.RangeDescriptorKey(roachpb.RKey(k)), &desc))
		require.NoError(t, txn.Put(ctx, keys.RangeMetaKey(desc.EndKey), "bar"))
		// Scratch range goes down with this replication change open.
	}
	tc.StopServer(1)

	// Sanity check that requests to the ScratchRange fail.
	func() {
		cCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		err := srv.DB().Put(cCtx, k, "baz")
		// NB: we don't assert on the exact error since our RPC layer
		// tries to return a better error than DeadlineExceeded (at
		// the time of writing, we get a connection failure to n2),
		// and we don't wait for the context to even time out.
		// We're probably checking in the RPC layer whether we've
		// retried with an up-to-date desc and fail fast if so.
		require.Error(t, err)
	}()

	// Get the store on the designated survivor n1.
	var store *kvserver.Store
	require.NoError(t, srv.GetStores().(*kvserver.Stores).VisitStores(func(inner *kvserver.Store) error {
		store = inner
		return nil
	}))
	if store == nil {
		t.Fatal("no store found on n1")
	}

	// Call ResetQuorum to reset quorum on the unhealthy range.
	t.Logf("resetting quorum")
	_, err = srv.Node().(*server.Node).ResetQuorum(
		ctx,
		&roachpb.ResetQuorumRequest{
			RangeID: int32(desc.RangeID),
		},
	)
	require.NoError(t, err)

	t.Logf("resetting quorum complete")

	require.NoError(t, srv.DB().Put(ctx, k, "baz"))
}

func TestAbortIntent(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	args := base.TestClusterArgs{
		ReplicationMode: base.ReplicationManual,
	}
	tc := testcluster.StartTestCluster(t, 2, args)
	defer tc.Stopper().Stop(ctx)

	// Set up a scratch range isolated to n2.
	//
	// 'k' will go down with n2, 'availableK' will remain up.
	k := tc.ScratchRange(t)
	availableK := k.Next()
	_, _, err := tc.SplitRange(k.Next())
	require.NoError(t, err)

	desc, err := tc.AddReplicas(k, tc.Target(1))
	require.NoError(t, err)
	require.NoError(t, tc.TransferRangeLease(desc, tc.Target(1)))
	desc, err = tc.RemoveReplicas(k, tc.Target(0))
	require.NoError(t, err)

	srv := tc.Server(0)
	require.NoError(t, srv.DB().Put(ctx, k, "bar"))
	require.NoError(t, srv.DB().Put(ctx, availableK, "goo"))

	txn := srv.DB().NewTxn(ctx, "boom")
	// Anchor a txn on 'k', and have it write to 'availableK' as well.
	require.NoError(t, txn.Put(ctx, k, "foo"))
	require.NoError(t, txn.Put(ctx, availableK, "foo"))
	tc.StopServer(1)

	// Read 'availableK' via READ_UNCOMMITTED.
	var b kv.Batch
	b.Header.ReadConsistency = roachpb.READ_UNCOMMITTED
	b.Get(availableK)
	require.NoError(t, srv.DB().Run(ctx, &b))
	resp := b.RawResponse().Responses[0].GetGet()
	require.NotNil(t, resp.Value)       // there is a committed value
	require.NotNil(t, resp.IntentValue) // there's an intent

	{
		bb, err := resp.Value.GetBytes()
		require.NoError(t, err)
		require.Equal(t, "goo", string(bb))
	}

	{
		bb, err := resp.IntentValue.GetBytes()
		require.NoError(t, err)
		require.Equal(t, "foo", string(bb))
	}
}
