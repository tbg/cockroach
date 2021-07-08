// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package crdbnemesis

import (
	"context"
	"math/rand"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/tests"
)

// Cluster is a handle to a running CockroachDB cluster that ActionFactory can
// run against.
type Cluster = interface {
	// StartWorkload .
	//
	// TODO(tbg): need to get away from long-running ssh sessions here. Things should
	// be deployed via systemctl and status checked similar to `roachprod monitor`.
	// Note that we're not asking for a node here, the test harness decides where
	// the workload node is. Need to figure out how multi-region fits in here, though.
	StartWorkload(ctx context.Context, t Fataler, args ...interface{})
}

// Fataler is a slim test.Test.
//
// TODO(tbg): use test.Test or a suitably slimmed down version of it.
type Fataler interface {
	Fatal(string, ...interface{})
	SkipNow()
}

func RunMixedVersionTest(
	seed int64, t Fataler, c *Cluster, f ActionFactory, minDurationPerStage time.Duration,
) {
	// - From the current version, go to the N predecessor versions and store in a slice.
	// - Start cluster in the first version.
	// - constraint: v<first version>, in finalized state
	// - execute actions from ActionFactory until minDurationPerStage has passed, then wait until done
	// - constraint: v<second version>, rolling state
	// - execute actions from ActionFactory interleaved with rolling nodes into and out of the second version, do this
	//   for minDurationPerStage again
	// - constraint: v<second version>, unfinalized state
	// - initiate the cluster version upgrade and in parallel execute actions from ActionFactory, once upgrade is complete
	//   let pending actions finish
	// - constraint: v<second version>, finalized state
	// - execute actions for minDurationPerStage
	// - repeat for third, fourth, etc version.
	//
	// Notes:
	// - We may be really lenient about failing actions in the predecessor versions, as the point of going through them
	//   is to set up interesting state once we reach the "current" version, not to pick up all the possible flakes that
	//   we will barely be able to fix. This (and the desire to ultimately make this kind of generated test very long-running)
	//   means that we need to pass "nested" fatalers to the actions that we can "terminate" before they fail the whole
	//   suite.
	// - Should monitor cluster health throughout (certain blips allowed during node-rolls, with next event waiting until
	//   the dust settles)
	r := rand.New(rand.NewSource(seed))
	_ = r
	_ = c
	constraint := ActionConstraint{}
	_ = constraint
	_ = tests.PredecessorVersion
	_ = f.GetActions
}
