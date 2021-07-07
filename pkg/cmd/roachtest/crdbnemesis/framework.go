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

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/cluster"
	"github.com/cockroachdb/cockroach/pkg/util/randutil"
)

// Cluster is a handle to a running CockroachDB cluster that ActionFactory can
// run against.
//
// TODO(tbg): this should take a better/narrower interface; most of the things
// on Cluster are not safely usable by individual `Action`s. Should start
// opinionated and build up from there rather than inviting "broken" Actions.
type Cluster = cluster.Cluster

type Fataler interface {
	Fatal(string, ...interface{})
	SkipNow()
}

type StepperSupportType byte

const (
	// UpgradeFinalized indicates that the cluster is fully upgraded and all migrations have completed.
	UpgradeFinalized StepperSupportType = iota
	// UpgradeUnfinalized indicates that all nodes in the cluster are running the updated binary, but
	// that the migrations may not all have run yet. It's possible that a transition to UpgradeRolling
	// will take place (i.e. binaries may be rolled back to the older version).
	UpgradeUnfinalized
	// UpgradeRolling indicates that there may be nodes running the old binary in the cluster.
	UpgradeRolling
)

type RandStepIter interface {
	Next(r *rand.Rand) (ActionFactory, Action, bool)
}

func Run(ctx context.Context, t Fataler, c Cluster, it RandStepIter) {
	// NB: the seed deterministically determines the
	// history (when the code is kept fixed).
	//
	// TODO(tbg): the purism here is nice but in practice one wants to
	// run the same history on different SHAs so better to print it out
	// too.
	r, seed := randutil.NewPseudoRand()
	_ = seed

	for {
		s, opts, ok := it.Next(r)
		if !ok {
			break
		}
		m := c.NewMonitor(ctx)
		if !opts.Chaos() {
			m.Go(func(ctx context.Context) error {
				// TODO: Monitor cluster's health.
				return nil
			})
		}
		m.Go(func(ctx context.Context) error {
			// TODO: the Fataler here needs long-term thought. For long-running generic
			// resilience tests, we'll want to preserve state when a step fails to facilitate debugging, and then let
			// someone decide whether the test is truly broken now (and the cluster needs
			// to be torn down); if it can continue we should be able to continue it.
			// This means that the `t` here needs to be recoverable at this point.
			// We could completely switch to error propagation here but the `t.Fatal`-style
			// workflow keeps tests focused on their logic which is helpful. It also produces
			// better messages that originate from where the problem was detected, rather than
			// some higher level that ultimately reports them.
			// In the short term, this seems fine. We have other problems to solve before
			// these tests can actually be long-running, such as the orchestration via long-lived
			// ssh sessions which will possibly prove brittle.
			s.Run(ctx, t, opts, c)
			return nil
		})
		m.Wait()
		// TODO: monitor cluster health (in particular, return to a healthy
		// state if the last step had `opts.Chaos()` set).
	}
}

func RunMixedVersionTest(t Fataler, c *Cluster, maxSteps int, maxTime time.Duration) {
	// - From the current version, go to the N predecessor versions and store in a slice.
	// - Start cluster in the first version.
	// - Go through iterator supporting current version.
	// - Get iterator for the next version & supporting the current version too.
	// - Run through that iterator while interleaving an unfinalized upgrade that gets rolled back and rolled forward again,
	//   e.g.: <steps>, roll n1 and n2, <steps>, rollback n1 and n2, <steps>, roll all nodes, <steps>, finalize upgrade
	// - go to third step above.
}
