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

	"github.com/cockroachdb/cockroach/pkg/roachpb"
)

// SimpleActionFactory is a reusable implementation of ActionFactory that returns
// a single action from GetActions.
type SimpleActionFactory struct {
	Supporter Supporter
	New       func(r *rand.Rand) SimpleAction
}

var _ ActionFactory = (*SimpleActionFactory)(nil)

// SupportsBinaryVersion implements ActionFactory.
func (f *SimpleActionFactory) SupportsBinaryVersion(v roachpb.Version) StepperSupportType {
	return f.Supporter.SupportsBinaryVersion(v)
}

type ActionConstraint struct {
	BinaryVersion       roachpb.Version
	ClusterVersionState StepperSupportType
}

// GetActions implements ActionFactory.
func (f *SimpleActionFactory) GetActions(r *rand.Rand) []Action {
	a := f.New(r)
	return []Action{a.I()}
}

type weightedFactory struct {
	f ActionFactory
	w float64
}

type RoundRobinActionFactory struct {
	sum float64
	sl  []weightedFactory
}

var _ ActionFactory = (*RoundRobinActionFactory)(nil)

func (f *RoundRobinActionFactory) Add(factory ActionFactory, weight float64) {
	f.sum += weight
	f.sl = append(f.sl, weightedFactory{f: factory, w: weight})
}

func (f *RoundRobinActionFactory) GetActions(r *rand.Rand) []Action {
	thresh := r.Float64() * f.sum
	var sum float64
	for i := range f.sl {
		sum += f.sl[i].w
		if sum >= thresh || i == len(f.sl)-1 {
			return []Action{f.sl[i].f.GetActions(r)[0]}
		}
	}
}
