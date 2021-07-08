// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package factory

import (
	"context"
	"math/rand"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/crdbnemesis"
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
	"go.uber.org/atomic"
)

var WorkloadKVActionFactory = crdbnemesis.SimpleActionFactory{
	Supports: crdbnemesis.RequireAtLeastV21Dot1,
	New: func(r *rand.Rand) crdbnemesis.SimpleAction {
		return crdbnemesis.SimpleAction{
			Name:        "workload_kv",
			Owner:       registry.OwnerKV,
			Cooperative: true,
			RunFn: func(ctx context.Context, t crdbnemesis.Fataler, c crdbnemesis.Cluster, status *atomic.String, duration *atomic.Duration) {
				c.StartWorkload(ctx, t, "--duration", "1h")
			},
		}
	},
}
