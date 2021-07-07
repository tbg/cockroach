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
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/util/randutil"
)

type Instance interface {
	Progress() (interface{}, time.Duration)
	Chaos() bool
}

// Cluster is a handle to a running CockroachDB cluster
// that ActionFactory can run against.
//
// TODO(tbg): this should take a better interface; most of the things on Cluster
// are not safely usable by individual `Instance`s.
type Cluster = cluster.Cluster

type Fataler interface {
	Fatal(string, ...interface{})
	SkipNow()
}

type StepperSupportType byte

const (
	Unsupported           StepperSupportType = iota // does not support this version
	OnlyFinalized                                   // supports cluster only if all nodes at main version & cluster version finalized
	CanMixWithPredecessor                           // supports cluster as long as one node is at main version
)

type ActionFactory interface {
	Supporter
	Name() string
	RandOptions(r *rand.Rand) Instance
	Run(context.Context, Fataler, Instance, Cluster)
	Owner() registry.Owner
}

type SimpleInstance struct {
	CurrentProgress float64
	ProgressWindow  time.Duration
	HasChaos        bool
}

func (s *SimpleInstance) Progress() (interface{}, time.Duration) {
	return s.Progress, s.ProgressWindow
}

func (s *SimpleInstance) Chaos() bool {
	return s.HasChaos
}

type Supporter interface {
	// SupportsBinaryVersion determines whether an operation supports
	// a cluster running at the given binary version.
	//
	// TODO: need to check if we want version.Version here. In general
	// should clean up the versions across roachtest; it's a mess.
	SupportsBinaryVersion(roachpb.Version) StepperSupportType
}

type Generator struct {
	m map[string]ActionFactory
	w map[string]float64
}

func (r *Generator) Register(s ActionFactory, w float64) {
	name := s.Name()
	r.m[name] = s
	r.w[name] = w
}

// AtLeastSupporter is a supporter that requires a binary version of at least 'AtLeast'.
// If SupportsMixed is true, also supports a cluster in which a supported version is
// running mixed with an older version.
type AtLeastSupporter struct {
	AtLeast       roachpb.Version
	SupportsMixed bool
}

func (s AtLeastSupporter) SupportsBinaryVersion(v roachpb.Version) StepperSupportType {
	if v.Less(s.AtLeast) {
		// Introduced at 20.1, but we're testing only v19.X or below.
		return Unsupported
	}
	// Introduced at 20.1, and we're testing at least that version.
	if s.SupportsMixed {
		// Supports a mixed 20.1/19.2 cluster.
		return CanMixWithPredecessor
	}
	// Need cluster version to be at least 20.1.
	return OnlyFinalized
}

// AtLeastV21Dot2MixedSupporter is a struct that can be embedded into ActionFactory implementations
// to implement an AtLeastSupporter{AtLeast: v21.2, SupportsMixed: true}. This should be the
// default choice for Steppers introduced prior to the v21.2 release.
type AtLeastV21Dot2MixedSupporter struct{}

func (*AtLeastV21Dot2MixedSupporter) SupportsBinaryVersion(v roachpb.Version) StepperSupportType {
	return AtLeastSupporter{
		AtLeast:       roachpb.Version{Major: 21, Minor: 2},
		SupportsMixed: true,
	}.SupportsBinaryVersion(v)
}

type RandStepIter interface {
	Next(r *rand.Rand) (ActionFactory, Instance, bool)
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
