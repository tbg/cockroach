// Copyright 2017 The Cockroach Authors.
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
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

package queue

import (
	"context"
	gosql "database/sql"
	"math/rand"
	"sync/atomic"

	"github.com/cockroachdb/cockroach/pkg/util/randutil"
	"github.com/cockroachdb/cockroach/pkg/util/syncutil"
	"github.com/spf13/pflag"

	"github.com/cockroachdb/cockroach/pkg/testutils/workload"
)

const (
	schema = `(ts BIGINT, k BYTES, v BYTES NOT NULL, PRIMARY KEY (k, v))`
)

func init() {
	workload.Register(meta)
}

var meta = workload.Meta{
	Name:        `queue`,
	Description: `Simulate a simplistic queue that creates lots of KV tombstones.`,
	Version:     `1.0.0`,
	New: func() workload.Generator {
		g := &queue{}
		g.flags = pflag.NewFlagSet(`kv`, pflag.ContinueOnError)
		g.mu.Rand = rand.New(rand.NewSource(0))
		return g
	},
}

type queue struct {
	flags *pflag.FlagSet
	mu    struct {
		syncutil.Mutex
		*rand.Rand
	}
	timestamp int32 // atomically
}

// Meta implements the Generator interface.
func (*queue) Meta() workload.Meta { return meta }

// Flags implements the Generator interface.
func (w *queue) Flags() *pflag.FlagSet {
	return w.flags
}

// Configure implements the Generator interface.
func (w *queue) Configure(_ []string) error {
	return nil
}

// Tables implements the Generator interface.
func (*queue) Tables() []workload.Table {
	table := workload.Table{
		Name:   `queue`,
		Schema: schema,
	}
	return []workload.Table{table}
}

// Ops implements the Generator interface.
func (w *queue) Ops() []workload.Operation {
	opInsertRemove := func(db *gosql.DB) (func(context.Context) error, error) {
		pInsert, err := db.Prepare("INSERT INTO queue VALUES ($1, $2, $3)")
		if err != nil {
			return nil, err
		}

		pRemove, err := db.Prepare("DELETE FROM queue LIMIT 1")
		if err != nil {
			return nil, err
		}

		return func(ctx context.Context) error {
			w.mu.Lock()
			k := randutil.RandBytes(w.mu.Rand, 64)
			v := randutil.RandBytes(w.mu.Rand, 64)
			w.mu.Unlock()

			seq := atomic.AddInt32(&w.timestamp, 1)
			if _, err := pInsert.Exec(seq, k, v); err != nil {
				return err
			}

			_, err := pRemove.Exec() //seq) //, k)
			return err
		}, nil
	}

	return []workload.Operation{{
		Name: "insertremove",
		Fn:   opInsertRemove,
	}}
}
