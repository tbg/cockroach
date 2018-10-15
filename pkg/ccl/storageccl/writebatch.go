// Copyright 2016 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/licenses/CCL.txt

package storageccl

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/storage/batcheval"
	"github.com/cockroachdb/cockroach/pkg/storage/batcheval/result"
	"github.com/cockroachdb/cockroach/pkg/storage/engine"
	"github.com/cockroachdb/cockroach/pkg/storage/engine/enginepb"
	"github.com/cockroachdb/cockroach/pkg/storage/spanset"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/pkg/errors"
)

func init() {
	batcheval.RegisterCommand(roachpb.WriteBatch, batcheval.DefaultDeclareKeys, evalWriteBatch)
}

// evalWriteBatch applies the operations encoded in a BatchRepr. Any existing
// data in the affected keyrange is first cleared (not tombstoned), which makes
// this command idempotent.
func evalWriteBatch(
	ctx context.Context, batch engine.ReadWriter, cArgs batcheval.CommandArgs, _ roachpb.Response,
) (result.Result, error) {
	return result.Result{}, nil
}

func clearExistingData(
	ctx context.Context, batch engine.ReadWriter, start, end engine.MVCCKey, nowNanos int64,
) (enginepb.MVCCStats, error) {
	{
		isEmpty := true
		if err := batch.Iterate(start, end, func(_ engine.MVCCKeyValue) (bool, error) {
			isEmpty = false
			return true, nil // stop right away
		}); err != nil {
			return enginepb.MVCCStats{}, errors.Wrap(err, "while checking for empty key space")
		}

		if isEmpty {
			return enginepb.MVCCStats{}, nil
		}
	}

	iter := batch.NewIterator(engine.IterOptions{UpperBound: end.Key})
	defer iter.Close()

	iter.Seek(start)
	if ok, err := iter.Valid(); err != nil {
		return enginepb.MVCCStats{}, err
	} else if ok && !iter.UnsafeKey().Less(end) {
		return enginepb.MVCCStats{}, nil
	}

	existingStats, err := iter.ComputeStats(start, end, nowNanos)
	if err != nil {
		return enginepb.MVCCStats{}, err
	}

	log.Eventf(ctx, "target key range not empty, will clear existing data: %+v", existingStats)
	// If this is a Iterator, we have to unwrap it because
	// ClearIterRange needs a plain rocksdb iterator (and can't unwrap
	// it itself because of import cycles).
	if ssi, ok := iter.(*spanset.Iterator); ok {
		iter = ssi.Iterator()
	}
	// TODO(dan): Ideally, this would use `batch.ClearRange` but it doesn't
	// yet work with read-write batches (or IngestExternalData).
	if err := batch.ClearIterRange(iter, start, end); err != nil {
		return enginepb.MVCCStats{}, err
	}
	return existingStats, nil
}
