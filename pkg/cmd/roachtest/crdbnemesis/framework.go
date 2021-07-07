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
	"math/rand"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/cluster"
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/tests"
)

// Cluster is a handle to a running CockroachDB cluster that ActionFactory can
// run against.
//
// TODO(tbg): this should take a better/narrower interface; most of the things
// on Cluster are not safely usable by individual `Action`s. Should start
// opinionated and build up from there rather than inviting "broken" Actions.
type Cluster = cluster.Cluster

// Fataler is a slim test.Test.
//
// TODO(tbg): use test.Test or a suitably slimmed down version of it.
type Fataler interface {
	Fatal(string, ...interface{})
	SkipNow()
}

func RunMixedVersionTest(
	seed int64, t Fataler, c *Cluster, f ActionFactory, minDurationPerSegment time.Duration,
) {
	// - From the current version, go to the N predecessor versions and store in a slice.
	// - Start cluster in the first version.
	// - constraint: v<first version>, in finalized state
	// - execute actions from ActionFactory until minDurationPerSegment has passed, then wait until done
	// - constraint: v<second version>, rolling state
	// - execute actions from ActionFactory interleaved with rolling nodes into and out of the second version, do this
	//   for minDurationPerSegment again
	// - constraint: v<second version>, unfinalized state
	// - initiate the cluster version upgrade and in parallel execute actions from ActionFactory, once upgrade is complete
	//   let pending actions finish
	// - constraint: v<second version>, finalized state
	// - execute actions for minDurationPerSegment
	// - repeat for third, fourth, etc version.
	// - We may be really lenient about failing actions in the predecessor versions, as the point of going through them
	//   is to set up interesting state once we reach the "current" version, not to pick up all the possible flakes that
	//   we will barely be able to fix. This (and the desire to ultimately make this kind of generated test very long-running)
	//   means that we need to pass "nested" fatalers to the actions that we can "terminate" before they fail the whole
	//   suite.
	r := rand.New(rand.NewSource(seed))
	_ = r
	_ = c
	constraint := ActionConstraint{}
	_ = constraint
	_ = tests.PredecessorVersion
	_ = f.GetActions
}
