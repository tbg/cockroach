// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package main

import (
	"context"
	"time"
)

type backgroundFn func(ctx context.Context, u *versionUpgradeTest) error

// A backgroundStepper is a tool to run long-lived commands while a cluster is
// going through a sequence of version upgrade operations.
// It exposes a `launch` step which launches the method carrying out long-running
// work (in the background) and a `stop` step collecting any errors.
type backgroundStepper struct {
	// This is the operation that will be launched in the background. When the
	// context gets cancelled, it should shut down and return without an error.
	// The way to typically get this is:
	//
	//  err := doSomething(ctx)
	//  ctx.Err() != nil {
	//    return nil
	//  }
	//  return err
	run backgroundFn

	// Internal.
	m      *monitor
	cancel func()
}

func makeBackgroundStepper(run backgroundFn) backgroundStepper {
	return backgroundStepper{run: run}
}

// launch spawns the function the background step was initialized with.
func (s *backgroundStepper) launch(ctx context.Context, _ *test, u *versionUpgradeTest) {
	s.m = newMonitor(ctx, u.c)
	ctx, s.m.cancel = context.WithCancel(ctx)
	s.m.Go(func(ctx context.Context) error {
		return s.run(ctx, u)
	})
}

func (s *backgroundStepper) stop(_ context.Context, t *test, _ *versionUpgradeTest) {
	s.cancel()
	if err := s.m.WaitE(); err != nil {
		t.Fatal(err)
	}
}

func backgroundSchemaChanger(loadNode int) backgroundStepper {
	return makeBackgroundStepper(func(ctx context.Context, u *versionUpgradeTest) error {
		err := u.c.RunE(ctx, u.c.Node(loadNode), "./workload run ...")
		if ctx.Err() != nil {
			// If the context is cancelled, that's probably why the workload
			// returned, so swallow error. (This is how the harness tells us
			// to shut down the workload).
			return nil
		}
		return err
	})
}

// TODO(spas): work this into the test you really want.
func runMixedVersionRandSchema(ctx context.Context, t *test, c *cluster) {
	// NB: this runs the workload on n4 which is also a CRDB node, but this
	// should work.
	const loadNode = 4
	bsc := backgroundSchemaChanger(loadNode)
	u := newVersionUpgradeTest(c, nil, /* no the features */
		// TODO(spas): fill in the things you really want.
		uploadAndStartFromCheckpointFixture("19.2.1"),
		waitForUpgradeStep(),

		func(ctx context.Context, _ *test, u *versionUpgradeTest) {
			c.Run(ctx, c.Node(loadNode), "./workload init ...")
		},
		bsc.launch,

		binaryUpgradeStep("HEAD"),
		waitForUpgradeStep(),
		func(ctx context.Context, _ *test, u *versionUpgradeTest) {
			// Keep workload running for a little longer, why not. Probably
			// also want to insert these delays between some other steps above.
			time.Sleep(time.Minute)
		},
		bsc.stop,
	)

	u.run(ctx, t)
}
