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
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
)

// An Action is a modular step that can be run against a Cluster and carries
// out a predetermined unit of work (i.e. does not intend to run forever).
//
// Actions come in two flavors: "cooperative" and "chaotic". A cooperative
// Action can also be scheduled as a chaotic action, but the converse is false.
//
// A cooperative Action can run concurrently with other cooperative actions
// without interference, assuming suitably constrained concurrency, and will
// only be invoked against a healthy (as determined by the test harness) cluster.
//
// Chaotic actions can only be scheduled together with other chaotic actions,
// and all bets are off. A failure returned from a chaotic action will be
// ignored. In particular, the set of chaotic actions is a superset of the
// set of cooperative actions - or in other words, a cooperative action is
// also a chaotic action. However, many cooperative actions provide little
// use when interpreted as chaotic actions (see the examples below).
//
// TODO(tbg): chaotic actions should be able to highlight problems to the test
// harness that can not stem from interference with other chaotic actions, and
// those *should* fail the test.
//
// Examples: An Action that
// - runs a one-hour low-throughput KV workload against a dedicated
//   database is cooperative. Like all actions, it can be understood as
//   a chaotic action, but since `workload` (by default, which we assume
//   is the case in this example) bails out on the first error received
//   from SQL, this is not a terribly useful chaotic workload. A flavor
//   that passes --tolerate-errors=false is more useful.
// - imports a table into a unique new name is cooperative. Likely makes a
//   useful chaotic Action as well, since the job should resume past node
//   restarts.
// - that reads or backs up a randomly chosen table at a historical timestamp is
//   cooperative. With retries, also useful as a chaotic workload.
// - intentionally overloads the cluster with requests, kills nodes, or introduces network
//   issues is not cooperative (i.e. is chaotic).
//
// Chaotic actions must reverse their chaotic effects on termination; for
// example by restarting down nodes or by stopping load. The same is true for a
// cooperative Action, for example it wouldn't be OK for a RESTORE Action to
// abandon a runaway job remain in an unfinalized status, as this could taint
// the next steps.
//
// TODO(tbg): there's likely an evolution of this down the road. For example, a
// graceful node shutdown (or node decommissioning) should count as cooperative,
// but it's unclear how this would be taken into account by all other
// cooperative Actions which we expect to have custom health checks. We may also
// want to schedule a mix of cooperative and chaotic actions (assuming that they
// might all fail, but that's ok) but that's best left as a follow-up because it
// requires a lot more thinking about the semantics; in the meantime we can
// manually package chaotic actions by stringing together hand-selected actions
// "under the hood".
//
// Actions also provide progress tracking. The Progress method returns a) an
// opaque progress status (i.e. something to display to the user, could be a
// percentage or a status string or a time remaining) and a duration within
// which the Action commits itself to having made progress (as measured via a
// change in the `fmt.Sprint(<opaque status>)`. An Action that is observed as
// failing to make progress may be canceled (via context cancellation), to which
// all implementations must be receptive.
//
// TODO(tbg): the "failing to make progress" problem is something we've struggled
// with in other parts of CRDB (the KV queues come to mind), so perhaps we can
// take this opportunity to carve out a good reusable abstraction, not sure what's
// here really qualifies as such. We also have something in roach test with `t.Status`,
// avoid making another half-baked attempt at this here.
type Action interface {
	Owner() registry.Owner
	Progress() (interface{}, time.Duration)
	Cooperative() bool
	Run(context.Context, Fataler, Cluster)
}
