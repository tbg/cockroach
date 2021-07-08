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

import "github.com/cockroachdb/cockroach/pkg/roachpb"

// An UpgradeStatusConstraint requests that the Action emitted by an ActionFactory
// supports a cluster at the version and upgrade status specified.
type UpgradeStatusConstraint struct {
	BinaryVersion       roachpb.Version
	ClusterVersionState UpgradeStatus
}

// An ActionConstraint represents the conditions that an Action
// emitted by an ActionFactory is requested to fulfill, and also
// promises to the factory that the Action will be executed under
// the same constraint. For example, if the constraint specifies
// that a fully-migrated v21.1 is running, this will be true once
// the action is actually scheduled, and the action must actually
// support it.
type ActionConstraint struct {
	Upgrade UpgradeStatusConstraint
	// TODO(tbg): duration hint where applicable (for example, `--duration` flag for `workload`).
	// TODO(tbg): Cooperative vs anything-goes.
}

// UpgradeStatus is an enum indicating what stage of a cluster upgrade an
// associated cluster is in. This enum is always presented with an associated
// binary version, which is understood as the "new" version.
type UpgradeStatus byte

const (
	// UpgradeStatusFinalized indicates that the cluster is fully upgraded and all migrations have completed.
	// This is the steady state when no upgrade is currently going on (i.e. the last upgrade is complete).
	UpgradeStatusFinalized UpgradeStatus = iota
	// UpgradeStatusUnfinalized indicates that all nodes in the cluster are
	// running the updated binary, but that the migrations may not all have run
	// yet. It's possible that a transition to UpgradeRolling will take place
	// (i.e. binaries may be rolled back to the older version), or that migrations
	// are started (if they're already running) and that UpgradeStatusFinalized
	// will follow. This is the intermediate state of a cluster upgrade.
	UpgradeStatusUnfinalized
	// UpgradeStatusRolling indicates that there may be nodes running the old binary
	// in the cluster. This is the state at the beginning of an upgrade.
	UpgradeStatusRolling
)

// RequireAtLeastV21Dot1 returns true if v21.1 is fully finalized (i.e.
// if the cluster is at least there or newer).
//
// TODO(tbg): version.Version? What about alphas? What weird shenanigans will
// we need to navigate around release branch cuts, etc? Think about the pitfalls
// before they become as annoying as with the existing mixed-version testing.
func RequireAtLeastV21Dot1(constraint ActionConstraint) bool {
	v := constraint.Upgrade.BinaryVersion
	s := constraint.Upgrade.ClusterVersionState
	atLeast := roachpb.Version{Major: 21, Minor: 1}
	v.Patch, v.Internal = 0, 0
	if v.Less(atLeast) {
		// Newest binary in cluster is, say, 19.2 but we require at least 21.1
		return false
	}
	if atLeast.Less(v) {
		// Newest binary in cluster is, say, 25.1 and we require at least 21.1;
		// the fact that a newer one is running implies that 21.1 was fully
		// migrated into at some point so we're good to go.
		//
		// TODO(tbg): this assumes that MinSupportedVersion lags the
		return true
	}
	// We want 21.1 and we're at 21.1, but if we're still upgrading from 20.2
	// we're not ready to support this cluster yet.
	return s == UpgradeStatusFinalized
}
