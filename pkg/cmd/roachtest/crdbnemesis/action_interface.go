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
// Actions come in two flavors: "cooperative" and "chaotic".
//
// A cooperative Action can run concurrently with other cooperative actions
// without interference, assuming suitably constrained concurrency. For example,
// an Action that runs a one-hour low-throughput KV workload against a dedicated
// database is cooperative. An action that imports a table into a unique new
// name is cooperative. An action that reads or backs up a randomly chosen table
// at a historical timestamp is also cooperative. An action that intentionally
// overloads the cluster with requests, kills nodes, or introduces network
// issues is not cooperative (i.e. is chaotic). Chaotic actions must reverse
// their chaotic effects on termination; for example by restarting down nodes or
// by stopping load.
//
// TODO(tbg): there's likely an evolution of this down the road. For example, a
// graceful node shutdown (or node decommissioning) should count as cooperative,
// but it's unclear that this would be taken into account by all other
// workloads. We may also want to schedule a mix of cooperative and chaotic
// actions (assuming that they might all fail, but that's ok) but that's best
// left as a follow-up because it requires a lot more thinking about the
// semantics; in the meantime we can manually package chaotic actions
// by stringing together hand-selected actions "under the hood".
//
// Chaotic actions can also be run concurrently with both cooperative and
// chaotic actions. However, interference is expected and both the cooperative
// and chaotic actions involved are likely to encounter behavior they are not
// expecting. Cooperative actions should be written
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
// here really qualifies as such.
type Action interface {
	Owner() registry.Owner
	Progress() (interface{}, time.Duration)
	Cooperative() bool
	Run(context.Context, Fataler, Cluster)
}
