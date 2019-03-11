#!/bin/bash

set -euxo pipefail

make GOFLAGS='-race' build
make bin/workload

killall -9 cockroach || true
rm -rf cockroach-data || true
mkdir -p cockroach-data/{1,2,3,4}

#export GORACE="log_path=datarace.log halt_on_error=1"

./cockroach start --insecure --host=127.0.0.1 --port=26257 --http-port=26258 --store=cockroach-data/1 --cache=256MiB --background
sleep 10
./cockroach start --insecure --host=127.0.0.1 --port=26259 --http-port=26260 --store=cockroach-data/2 --cache=256MiB --join=127.0.0.1:26257 --background
sleep 10
./cockroach start --insecure --host=127.0.0.1 --port=26261 --http-port=26262 --store=cockroach-data/3 --cache=256MiB --join=127.0.0.1:26257 --background
sleep 10
#./cockroach start --insecure --host=127.0.0.1 --port=26263 --http-port=26264 --store=cockroach-data/4 --cache=256MiB --join=127.0.0.1:26257 --background

exit 0
sleep 10

./bin/workload run kv --init --splits 100 --read-percent 50 --batch 64 --concurrency 4 postgresql://root@127.0.0.1:26257?sslmode=disable postgresql://root@127.0.0.1:26259?sslmode=disable postgresql://root@127.0.0.1:26261?sslmode=disable postgresql://root@127.0.0.1:26263?sslmode=disable
