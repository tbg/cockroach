// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package actions

import (
	"context"
	"math/rand"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/crdbnemesis"
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
	"go.uber.org/atomic"
)

func init() {
	all = append(all, &workloadKVActionFactory)
}

var workloadKVActionFactory = crdbnemesis.SimpleActionFactory{
	Supports: crdbnemesis.RequireAtLeastV21Dot1,
	New: func(r *rand.Rand) crdbnemesis.SimpleAction {
		return crdbnemesis.SimpleAction{
			Name:        "workload_kv",
			Owner:       registry.OwnerKV,
			Cooperative: true,
			RunFn: func(ctx context.Context, t crdbnemesis.Fataler, c crdbnemesis.Cluster, _ *atomic.String, _ *atomic.Duration) {
				// TODO(tbg): this won't work very well since if this runs concurrently with a chaos event,
				// this monitor will tear down very early. Maybe it's better to pass in a top-level monitor
				// that consequently becomes aware of ~various things?
				// There's also this old bug in the monitor that it basically needs to start watching events
				// when it's instantiated, not when .Wait is called. Or maybe the solution is to make the
				// monitor here not care about the cluster nodes at all, at least by default?
				m := c.NewMonitor(ctx)
				m.Go(c.DeployWorkload(ctx, t, "kv", "--duration", "1h").Wait)
			},
		}
	},
}
