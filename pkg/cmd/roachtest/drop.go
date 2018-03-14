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
	"fmt"

	_ "github.com/lib/pq"

	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
)

func init() {
	// TODO(tschottdorf): rearrange all tests so that their synopses are available
	// via godoc and (some variation on) `roachtest run <testname> --help`.

	// This test imports a TPCC dataset and then issues a manual deletion followed
	// by a truncation for the `stock` table (which contains warehouses*100k
	// rows). Next, it issues a `DROP` for the whole database, and sets the GC TTL
	// to one second.
	runDrop := func(ctx context.Context, t *test, c *cluster, warehouses, nodes int) {
		c.Put(ctx, cockroach, "./cockroach", c.Range(1, nodes))
		c.Put(ctx, workload, "./workload", c.Range(1, nodes))
		c.Start(ctx, c.Range(1, nodes), startArgs("-e", "COCKROACH_MEMPROF_INTERVAL=15s"))

		db := c.Conn(ctx, 1)
		defer db.Close()

		m := newMonitor(ctx, c, c.Range(1, nodes))
		m.Go(func(ctx context.Context) error {
			run := func(stmt string) {
				t.Status(stmt)
				_, err := db.ExecContext(ctx, stmt)
				if err != nil {
					t.Fatal(err)
				}
			}

			count := func(msg string) int {
				const stmt = "SELECT COUNT(*) FROM tpcc.stock"
				var n int
				tBegin := timeutil.Now()
				if err := db.QueryRowContext(ctx, stmt).Scan(&n); err != nil {
					t.Fatal(err)
				}
				c.l.printf("%s: ran %s in %s\n", msg, stmt, timeutil.Since(tBegin))
				return n
			}

			run(`SET CLUSTER SETTING trace.debug.enable = true`)

			t.Status("importing TPCC fixture")
			c.Run(ctx, 1, fmt.Sprintf(
				"./workload fixtures load tpcc --warehouses=%d --db tpcc {pgurl:1}", warehouses))

			for i := 0; i < 5; i++ {
				if exp, act := warehouses*100000, count("before delete"); exp != act {
					t.Fatalf("expected %d rows, got %d", exp, act)
				}
			}

			// Drop a constraint that would get in the way of deleting from tpcc.stock.
			const stmtDropConstraint = "ALTER TABLE tpcc.order_line DROP CONSTRAINT fk_ol_supply_w_id_ref_stock"
			run(stmtDropConstraint)

			// NB: activate the other branch if you ever want to run this
			// test against v1.1. (It will fail when changing the zone
			// config below, though).
			if true {
				const stmtDelete = "DELETE FROM tpcc.stock"
				run(stmtDelete)
			} else {
				for {
					res, err := db.ExecContext(ctx, "DELETE FROM tpcc.stock LIMIT 10000")
					if err != nil {
						t.Fatal(err)
					}
					if n, err := res.RowsAffected(); err != nil {
						t.Fatal(err)
					} else if n == 0 {
						break
					}
				}
			}

			for i := 0; i < 5; i++ {
				if exp, act := 0, count("after delete"); exp != act {
					t.Fatalf("expected %d rows, got %d", exp, act)
				}
			}

			const stmtTruncate = "TRUNCATE TABLE tpcc.stock"
			run(stmtTruncate)

			if exp, act := 0, count("after truncate"); exp != act {
				t.Fatalf("expected %d rows, got %d", exp, act)
			}

			const stmtDrop = "DROP DATABASE tpcc"
			run(stmtDrop)
			// The data has already been deleted, but changing the default zone config
			// should take effect retroactively.
			const stmtZone = `ALTER RANGE default EXPERIMENTAL CONFIGURE ZONE '
gc:
  ttlseconds: 1
'`
			run(stmtZone)

			// TODO(tschottdorf): assert that the disk usage drops to "near nothing".
			return nil
		})
		m.Wait()
	}

	warehouses := 100
	numNodes := 9

	tests.Add(testSpec{
		Name:  fmt.Sprintf("drop/tpcc/w=%d,nodes=%d", warehouses, numNodes),
		Nodes: nodes(numNodes),
		Run: func(ctx context.Context, t *test, c *cluster) {
			// NB: this is likely not going to work out in `-local` mode. Edit the
			// numbers during iteration.
			if local {
				numNodes = 4
				warehouses = 1
				fmt.Printf("running with w=%d,nodes=%d in local mode\n", warehouses, numNodes)
			}
			runDrop(ctx, t, c, warehouses, numNodes)
		},
	})
}
