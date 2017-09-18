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
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/cockroachdb/cockroach/pkg/acceptance/localcluster"
	"github.com/cockroachdb/cockroach/pkg/internal/client"
	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/storage/engine/enginepb"
	"github.com/cockroachdb/cockroach/pkg/util/hlc"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
	"github.com/cockroachdb/cockroach/pkg/util/uuid"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	cockroachBin := func() string {
		bin := "./cockroach"
		if _, err := os.Stat(bin); os.IsNotExist(err) {
			bin = "cockroach"
		} else if err != nil {
			panic(err)
		}
		return bin
	}()

	const numNodes = 1

	perNodeCfg := localcluster.MakePerNodeFixedPortsCfg(numNodes)

	cfg := localcluster.ClusterConfig{
		DataDir:     "cockroach-data-gcpressurizer",
		Binary:      cockroachBin,
		NumNodes:    numNodes,
		NumWorkers:  numNodes,
		AllNodeArgs: flag.Args(),
		DB:          "zerosum",
		PerNodeCfg:  perNodeCfg,
	}

	c := localcluster.New(cfg)
	defer c.Close()

	log.SetExitFunc(func(code int) {
		c.Close()
		os.Exit(code)
	})

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		s := <-signalCh
		log.Infof(context.Background(), "signal received: %v", s)
		cancel()
		c.Close()
		os.Exit(1)
	}()

	c.Start(context.Background())

	kv := c.Nodes[0].Client()

	const (
		key          = "a"
		spansPerTxn  = 100
		txnsPerBatch = 1000
	)

	letters := "abcdefghijklmnopqrstuvwxyz"

	tablePrefix := keys.MakeTablePrefix(9999)
	randKey := func() roachpb.Key {
		l := rand.Intn(len(letters))
		key := make(roachpb.Key, l+len(tablePrefix))
		for i, charIdx := range rand.Perm(len(letters))[:l] {
			key[i] = letters[charIdx]
		}
		return key
	}

	// A month ago.
	pastTS := timeutil.Now().Add(-24 * time.Hour * 30).UnixNano()

	t := timeutil.NewTimer()
	t.Reset(time.Second)

	var cTotalIntents int
	var cTotalTxns int

	for {
		var b client.Batch
		for i := 0; i < txnsPerBatch; i++ {
			txn := roachpb.MakeTransaction(
				"test",
				roachpb.Key(key),
				roachpb.NormalUserPriority,
				enginepb.SERIALIZABLE,
				hlc.Timestamp{WallTime: pastTS},
				500*time.Millisecond.Nanoseconds(),
			)
			txn.Intents = make([]roachpb.Span, spansPerTxn)
			for s := range txn.Intents {
				txn.Intents[s] = roachpb.Span{
					Key: randKey(),
				}
				txn.Intents[s].EndKey = append(roachpb.Key(nil), txn.Intents[s].Key...)
				txn.Intents[s].EndKey = append(txn.Intents[s].EndKey, randKey()...)
			}
			b.Put(keys.TransactionKey(roachpb.Key(key), uuid.MakeV4()), &txn)
		}

		if err := kv.Run(ctx, &b); err != nil {
			log.Warning(ctx, err)
			continue
		}
		cTotalTxns += txnsPerBatch
		cTotalIntents += txnsPerBatch * spansPerTxn

		select {
		case <-t.C:
			t.Read = true
			t.Reset(time.Second)

			log.Infof(ctx, "txns: %d (total intents %d)", cTotalTxns, cTotalIntents)
		default:
		}
	}
}
