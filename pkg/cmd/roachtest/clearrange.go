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
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

package main

import (
	"context"
	"time"
)

func registerClearRange(r *registry) {
	r.Add(testSpec{
		Name:   `clearrange`,
		Nodes:  nodes(10),
		Stable: false,
		Run: func(ctx context.Context, t *test, c *cluster) {
			t.Status(`downloading store dumps`)
			// Created via:
			// roachtest --cockroach cockroach-v2.0.1 store-gen --stores=10 bank \
			//           --payload-bytes=10240 --ranges=0 --rows=65104166
			fixtureURL := `gs://cockroach-fixtures/workload/bank/version=1.0.0,payload-bytes=10240,ranges=0,rows=65104166,seed=1`

			location := storeDirURL(fixtureURL, c.nodes, "2.0")
			if err := downloadStoreDumps(ctx, c, location, c.nodes); err != nil {
				t.Fatal(err)
			}

			c.Put(ctx, cockroach, "./cockroach")
			c.Start(ctx)

			t.Status(`restoring tiny table`)
			defer t.WorkerStatus()

			c.Run(ctx, c.Node(1), `./cockroach sql --insecure -e "CREATE DATABASE tinybank"`)
			c.Run(ctx, c.Node(1), `./cockroach sql --insecure -e "
				RESTORE csv.bank FROM
        'gs://cockroach-fixtures/workload/bank/version=1.0.0,payload-bytes=100,ranges=10,rows=800,seed=1/bank'
				WITH into_db = 'tinybank'"`)

			t.Status()

			m := newMonitor(ctx, c)
			m.Go(func(ctx context.Context) error {
				conn := c.Conn(ctx, 1)
				defer conn.Close()

				t.WorkerStatus("dropping table")
				defer t.WorkerStatus()

				if _, err := conn.ExecContext(ctx, `ALTER TABLE bank.bank EXPERIMENTAL CONFIGURE ZONE 'gc: {ttlseconds: 30}'`); err != nil {
					return err
				}

				if _, err := conn.ExecContext(ctx, `DROP TABLE bank.bank`); err != nil {
					return err
				}

				// Spend a few minutes reading data to make sure the DROP above didn't brick the cluster.
				// TODO(tschottdorf): could use kv workload with a min rate instead.
				const minutes = 10
				t.WorkerStatus("repeatedly running COUNT(*) on small table")
				for i := 0; i < minutes; i++ {
					after := time.After(time.Minute)
					var count int
					if _, err := conn.ExecContext(ctx, `SET statement_timeout = '10s'`); err != nil {
						return err
					}
					if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM tinybank.bank`).Scan(&count); err != nil {
						return err
					}
					c.l.printf("read %d rows\n", count)
					t.WorkerProgress(float64(i+1) / float64(minutes))
					select {
					case <-after:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			})
			m.Wait()
		},
	})
}
