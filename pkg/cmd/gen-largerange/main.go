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
// permissions and limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/cockroachdb/cockroach/pkg/acceptance/localcluster"
	"github.com/cockroachdb/cockroach/pkg/util/binfetcher"
	"github.com/cockroachdb/cockroach/pkg/util/randutil"
)

var flagVersion = flag.String("version", "", "CockroachDB version (empty for ./cockroach)")

// The default settings generate a 2GiB = (1+1)*1000 MB table (with pesky large
// primary keys and values).
var flagKeyBytes = flag.Int("keybytes", 1<<20 /* 1MB */, "key size")
var flagValBytes = flag.Int("valbytes", 1<<20 /* 1MB */, "row size (across column families)")
var flagNumInserts = flag.Int("num", 1000, "number of insertions")

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())

	cockroachBin := func() string {
		if *flagVersion == "" {
			return "./cockroach"
		}
		bin, err := binfetcher.Download(ctx, binfetcher.Options{
			Binary:  "cockroach",
			Version: *flagVersion,
		})
		if err != nil {
			panic(err)
		}
		return bin
	}()

	const numNodes = 1

	perNodeCfg := localcluster.MakePerNodeFixedPortsCfg(numNodes)

	cfg := localcluster.ClusterConfig{
		DataDir:     "cockroach-data-gen-largerange",
		Binary:      cockroachBin,
		NumNodes:    numNodes,
		NumWorkers:  numNodes,
		AllNodeArgs: flag.Args(),
		DB:          "test",
		PerNodeCfg:  perNodeCfg,
	}

	c := localcluster.LocalCluster{
		Cluster: localcluster.New(cfg),
	}

	cleanupOnce := func() func() {
		var once sync.Once
		return func() {
			once.Do(func() {
				c.Close()
			})
		}
	}()
	defer cleanupOnce()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-signalCh
		cancel()
		cleanupOnce()
		os.Exit(1)
	}()

	c.Start(context.Background())

	time.Sleep(2 * time.Second)
	const setupStmt = `
SET CLUSTER SETTING trace.debug.enable = true;
SET CLUSTER SETTING server.remote_debugging.mode = 'any';

CREATE DATABASE IF NOT EXISTS test;
DROP TABLE IF EXISTS test.largerange;

CREATE TABLE IF NOT EXISTS test.largerange (
	k STRING PRIMARY KEY,
	v STRING
);

ALTER TABLE test.largerange EXPERIMENTAL CONFIGURE ZONE e'
range_max_bytes: 1000000000000
gc:
  ttlseconds: 2147483647
num_replicas: 1
';
`

	db := c.Nodes[0].DB()
	if _, err := db.ExecContext(ctx, setupStmt); err != nil {
		panic(err)
	}

	rnd, _ := randutil.NewPseudoRand()

	stmt, err := db.PrepareContext(ctx, `INSERT INTO test.largerange VALUES($1, $2)`)
	if err != nil {
		panic(err)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for i := 0; i < *flagNumInserts; i++ {
		k := randutil.RandBytes(rnd, *flagKeyBytes)
		v := randutil.RandBytes(rnd, *flagValBytes)
		if _, err := stmt.ExecContext(ctx, k, v); err != nil {
			panic(err)
		}
		select {
		case <-ticker.C:
			fmt.Printf("%d/%d\n", i+1, *flagNumInserts)
		default:
		}
	}
}
