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

import "math/rand"

// SimpleActionFactory is a reusable implementation of ActionFactory that returns
// a single action from GetActions.
type SimpleActionFactory struct {
	// Supports evaluates the ActionConstraint passed to GetActions. New is invoked only
	// if Supports returns true.
	//
	// A good default choice is RequireAtLeastV21Dot1 (where 21.1 is replaced by
	// the version that the current branch will become). This will ensure that the
	// newly introduced Actions aren't scheduled against older clusters. If this
	// is desired, Supports can be tailored accordingly. For (a contrived)
	// example, a `SELECT 1` should work against any version of CockroachDB, so
	// here it would make sense to always return `true` from this method.
	Supports func(constraint ActionConstraint) bool
	// New returns a SimpleAction to return from GetActions.
	New func(r *rand.Rand) SimpleAction
}

var _ ActionFactory = (*SimpleActionFactory)(nil)

// GetActions implements ActionFactory.
func (f *SimpleActionFactory) GetActions(r *rand.Rand, constraint ActionConstraint) []Action {
	if !f.Supports(constraint) {
		return nil
	}
	a := f.New(r)
	return []Action{a.I()}
}

type weightedFactory struct {
	f ActionFactory
	w float64
}

// RoundRobinActionFactory is an ActionFactory that delegates weighted-randomly
// to a set of ActionFactories registered with it.
//
// RoundRobinActionFactory is not thread safe.
type RoundRobinActionFactory struct {
	sum float64
	sl  []weightedFactory
}

var _ ActionFactory = (*RoundRobinActionFactory)(nil)

// Add registers an ActionFactory for use with GetActions. The weight determines
// how likely GetActions will chose this factory (for ActionConstraints it can
// satisfy). By convention, a value of 1.0 is used as a base value.
//
// The intended usage is that all calls to Add precede the first call to GetActions.
func (f *RoundRobinActionFactory) Add(factory ActionFactory, weight float64) {
	f.sl = append(f.sl, weightedFactory{f: factory, w: weight})
}

// GetActions implements ActionFactory. From the factories registered with it
// that can satisfy the constraints, a weighted-random factory is chosen and its output
// is returned. An empty slice indicates that no registered factory was able to satisfy
// the ActionConstraint.
func (f *RoundRobinActionFactory) GetActions(r *rand.Rand, constraint ActionConstraint) []Action {
	// TODO(tbg): I'll be very surprised if this doesn't have a bug or two or ten.

	seed := r.Int63()
	var admissible []weightedFactory
	var sum float64
	mAdmissibleIdxToActions := map[int][]Action{}
	for i := range f.sl {
		r := rand.New(rand.NewSource(seed))
		sl := f.sl[i].f.GetActions(r, constraint)
		if len(sl) == 0 {
			// Current factory can't satisfy ActionConstraint.
			continue
		}
		mAdmissibleIdxToActions[len(admissible)] = sl
		admissible = append(admissible, f.sl[i])
		sum += f.sl[i].w
	}

	// Now standard weighted-random pick from `admissible` slice.
	thresh := r.Float64() * sum
	var cur float64
	for i := range admissible {
		cur += admissible[i].w
		if cur >= thresh {
			return mAdmissibleIdxToActions[i]
		}
	}
	return mAdmissibleIdxToActions[len(admissible)-1] // wouldn't be hit if float64 had infinite precision
}
