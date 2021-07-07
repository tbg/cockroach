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
	"go.uber.org/atomic"
)

// SimpleAction is a concrete re-usable implementation of Action (via the I() method)
type SimpleAction struct {
	Name           string
	Owner          registry.Owner
	Progress       atomic.String
	ProgressWithin atomic.Duration
	Cooperative    bool
	RunFn          func(context.Context, Fataler, Cluster, *atomic.String, *atomic.Duration)
}

func (s *SimpleAction) I() Action {
	return &simpleActionImpl{s}
}

// simpleActionImpl is a helper type that lets us name the fields of
// SimpleAction like the methods of the Action interface without clashes.
type simpleActionImpl struct {
	*SimpleAction
}

var _ Action = (*simpleActionImpl)(nil)

func (s *simpleActionImpl) Owner() registry.Owner {
	return s.SimpleAction.Owner
}

func (s *simpleActionImpl) Progress() (interface{}, time.Duration) {
	return s.SimpleAction.Progress.Load(), s.SimpleAction.ProgressWithin.Load()
}

func (s *simpleActionImpl) Cooperative() bool {
	return s.SimpleAction.Cooperative
}

func (s *simpleActionImpl) Run(ctx context.Context, t Fataler, c Cluster) {
	s.RunFn(ctx, t, c, &s.SimpleAction.Progress, &s.SimpleAction.ProgressWithin)
}
