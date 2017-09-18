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
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
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
)

var flagNumTxns = flag.Int("txns", 100, "number of transactions to write")
var flagNumIntentsPerTxn = flag.Int("intents", 0, "number of (fictional) intents per transactions")

func main() {
	flag.Parse()

	targetStatus := [3]roachpb.TransactionStatus{
		roachpb.COMMITTED,
		roachpb.COMMITTED,
		roachpb.COMMITTED,
	}
	var _ = targetStatus

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

	log.SetExitFunc(func(code int) {
		cleanupOnce()
		os.Exit(code)
	})

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		s := <-signalCh
		log.Infof(context.Background(), "signal received: %v", s)
		cancel()
		cleanupOnce()
		os.Exit(1)
	}()

	c.Start(context.Background())

	if _, err := c.Nodes[0].DB().Exec("SET CLUSTER SETTING trace.debug.enable = true"); err != nil {
		log.Fatal(ctx, err)
	}

	{
		tmp := filepath.Join(cfg.DataDir, "zone")
		if err := ioutil.WriteFile(tmp, []byte("num_replicas: 1\ngc:\n  ttlseconds: 0\n"), 0755); err != nil {
			log.Fatal(ctx, err)
		}
		if _, _, err := c.ExecCLI(ctx, 0, []string{"zone", "set", ".default", "--file", tmp}); err != nil {
			log.Fatal(ctx, err)
		}
		_ = os.Remove(tmp)
	}

	kv := c.Nodes[0].Client()

	const (
		txnsPerBatch = 1000
	)

	letters := "abcdefghijklmnopqrstuvwxyz"

	// Base everything off of the liveness key for the nonexistent node zero. This makes sure
	// nothing we write gets in the way of the "real" node liveness span.
	key := keys.NodeLivenessKey(0)

	randKey := func() roachpb.Key {
		l := rand.Intn(len(letters))
		k := make(roachpb.Key, l+len(key))
		copy(k, key)
		for i, charIdx := range rand.Perm(len(letters))[:l] {
			k[len(key)+i] = letters[charIdx]
		}
		return k
	}

	pastTS := timeutil.Now().Add(-2 * time.Hour).UnixNano()

	t := timeutil.NewTimer()
	t.Reset(time.Second)

	var cTotalIntents int
	var cTotalTxns int
	logStatus := func() {
		log.Infof(ctx, "txns: %d (total intents %d)", cTotalTxns, cTotalIntents)
	}

	var wg sync.WaitGroup
	for cTotalTxns < *flagNumTxns {
		var b client.Batch
		numTxnsInBatch := *flagNumTxns
		if numTxnsInBatch > txnsPerBatch {
			numTxnsInBatch = txnsPerBatch
		}
		for i := 0; i < numTxnsInBatch; i++ {
			intents := make([]roachpb.Span, *flagNumIntentsPerTxn)
			for s := range intents {
				intents[s] = roachpb.Span{
					Key: randKey(),
				}
				intents[s].EndKey = append(roachpb.Key(nil), intents[s].Key...)
				intents[s].EndKey = append(intents[s].EndKey, randKey()...)
			}
			txnCtx, txnCancel := context.WithCancel(ctx)
			txn := roachpb.MakeTransaction(
				"test",
				key,
				roachpb.NormalUserPriority,
				enginepb.SERIALIZABLE,
				hlc.Timestamp{WallTime: pastTS},
				500*time.Millisecond.Nanoseconds(),
			)
			kvTxn := client.NewTxnWithProto(kv, txn)
			var bTxn client.Batch
			for _, intent := range intents {
				bTxn.Put(intent.Key, intent.EndKey /* abuse as value */)
			}
			wg.Add(1)
			go func() {
				defer txnCancel()
				defer wg.Done()
				if err := kvTxn.Run(txnCtx, &bTxn); err != nil {
					log.Fatal(ctx, err)
				}
			}()

			//txn.Intents = intents
			_ = pastTS
			//txn.Status = targetStatus[i%len(targetStatus)]
			//b.PutInline(keys.TransactionKey(key, txn.ID), &txn)
		}

		if err := kv.Run(ctx, &b); err != nil {
			log.Fatal(ctx, err)
		}
		cTotalTxns += numTxnsInBatch
		cTotalIntents += numTxnsInBatch * (*flagNumIntentsPerTxn)

		select {
		case <-t.C:
			t.Read = true
			t.Reset(time.Second)
			logStatus()
		default:
		}
	}
	log.Info(ctx, "waiting for intents to be written...")
	wg.Wait()
	logStatus()
	close(signalCh)
}
