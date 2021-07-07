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

// An ActionFactory is a generator for Action. It is handed a random number
// generator which it can use to (deterministically) determine at least one (but
// possibly multiple) Actions to return, which are to be executed concurrently.
// If multiple actions are returned, they must all be cooperative and
// independent, as not all of them may ultimately be invoked.
//
// For example, a simple implementation might return a single KV workload with a
// randomized read:write ratio. Another implementation might randomly pull from
// a variety of sub-factories and return up to N (cooperative) actions. The
// intention is that complex actions are built up from simple building blocks
// (via SimpleAction) provided by different contributors, resulting in a wide
// variety of workloads the clusters under crdbnemesis will encounter.
//
// TODO(tbg): does it make sense to return multiple Actions? The original idea
// was that it was better to let the factory (rather than something in the
// runner, i.e. the caller to the factory) pick what steps to combine. But it
// complicates one of the first goals, namely having exactly, say, three
// cooperative actions in-flight at any given point in time. This is much more
// straightforward with a model in which the runner pulls a new action from a
// round-robin ActionFactory every time a slot frees up (we can do that with
// []Action as well, but need to either provide a "max actions" hint or simply
// ignore excess actions, both are fine). Also, once problems with a certain
// combination do occur one wants to reproduce that, and it seems nice to "just"
// provide an ActionFactory that returns the right combination as an []Action.
// So I'm inclined to think that this is good.
type ActionFactory interface {
	Supporter
	GetActions(r *rand.Rand) []Action
}
