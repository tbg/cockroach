// Copyright 2018 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package storage_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
)

func TestStoreMinProposal(t *testing.T) {
	defer leaktest.AfterTest(t)()
	mtc := &multiTestContext{}
	defer mtc.Stop()
	mtc.Start(t, 1)
	verifyLiveness(t, mtc)
	ctx := context.Background()
	store := mtc.stores[0]

	mp1, cleanup1 := store.GetMinProposal(ctx)
	mp2, cleanup2 := store.GetMinProposal(ctx)
	if mp1 != mp2 {
		t.Errorf("expected mp1 and mp2 equal; got %s vs %s", mp1, mp2)
	}

	// Initial call to get closed timestamp establishes baseline.
	closed1, epoch1 := store.GetClosedTimestampAndEpoch(ctx)
	if closed1 != mp1 {
		t.Errorf("expected closed and mp1 equal; got %s vs %s", closed1, mp2)
	}
	if epoch1 != 1 {
		t.Errorf("exected epoch == 1; got %d", epoch1)
	}

	// Now get more min proposals, which should occur at a higher timestamp.
	mp3, cleanup3 := store.GetMinProposal(ctx)
	if !mp1.Less(mp3) {
		t.Errorf("expected mp1 < mp3; got %s >= %s", mp1, mp3)
	}
	cleanup3()

	// However, the closed timestamp won't change so long as we have prior
	// min proposal cleanups outstanding.
	closed2, _ := store.GetClosedTimestampAndEpoch(ctx)
	if closed1 != closed2 {
		t.Errorf("expected closed1 and closed2 equal; got %s vs %s", closed1, closed2)
	}

	// With cleanups still outstanding, the min proposal will not advance.
	mp4, cleanup4 := store.GetMinProposal(ctx)
	if mp4 != mp3 {
		t.Errorf("expected mp3 and mp4 equal; got %s vs %s", mp3, mp4)
	}
	cleanup4()

	// Release just one of the two outstanding cleanups.
	cleanup1()

	// The closed and epoch still won't change.
	closed3, _ := store.GetClosedTimestampAndEpoch(ctx)
	if closed1 != closed3 {
		t.Errorf("expected closed1 and closed3 equal; got %s vs %s", closed1, closed3)
	}
	mp5, cleanup5 := store.GetMinProposal(ctx)
	if mp3 != mp5 {
		t.Errorf("expected mp3 and mp5 equal; got %s vs %s", mp3, mp5)
	}
	cleanup5()

	// Now cleanup the final min proposal to verify the new closed
	// equals the more recent min proposals, and the min proposal can advance.
	cleanup2()
	closed4, _ := store.GetClosedTimestampAndEpoch(ctx)
	if closed4 != mp3 {
		t.Errorf("expected closed4 and mp3 equal; got %s vs %s", closed4, mp3)
	}
	mp6, cleanup6 := store.GetMinProposal(ctx)
	if !mp3.Less(mp6) {
		t.Errorf("expected mp3 < mp6; got %s >= %s", mp3, mp6)
	}
	cleanup6()
}
