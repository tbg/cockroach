// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package sql

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/base"
	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/testutils/serverutils"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func TestTenant(t *testing.T) {
	defer leaktest.AfterTest(t)()

	// NB: test must be invoked with COCKROACH_TENANT_ID=1 in order to work.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tenant := keys.TenantID() != 0

	t.Run(fmt.Sprintf("tenant=%t", tenant), func(t *testing.T) {

		params := base.TestServerArgs{}
		if tenant {
			// Start the "base" server as a subprocess. This is an approximation
			// to having a pure SQL container talk to a KV backend. A poor one.
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.v", "-test.run", "TestTenant")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = append(cmd.Env, "COCKROACH_TENANT_ID=0")
			go func() {
				if err := cmd.Run(); err != nil {
					if ctx.Err() == nil {
						t.Error(err)
					}
				}
			}()

			log.Infof(ctx, "TBG --- JOINING")
			params.Addr = "localhost:26257"
			params.JoinAddr = "localhost:26258"
		} else {
			log.Infof(ctx, "TBG --- HOSTING")
			params.Addr = "localhost:26258"
		}
		tc, db, _ := serverutils.StartServer(t, params)
		defer tc.Stopper().Stop(ctx)

		if !tenant {
			_, err := db.Exec(`SELECT crdb_internal.create_tenant(1)`)
			require.NoError(t, err)
			<-(chan struct{})(nil)
		} else {
			time.Sleep(time.Second)
			_, err := db.Query(`SELECT 1`)
			require.NoError(t, err)
			_, err = db.Query(`SHOW DATABASES`)
			require.NoError(t, err)
			_, err = db.Query(`SELECT * FROM system.settings`)
			require.NoError(t, err)
			c := make(chan os.Signal)
			signal.Notify(c)
			<-c
		}

	})
}
