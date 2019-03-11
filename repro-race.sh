#!/bin/bash

set -euxo pipefail

make GOFLAGS='-race' build
make bin/workload

killall -9 cockroach || true
rm -rf cockroach-data || true
mkdir -p cockroach-data/{1,2,3,4}

#export GORACE="log_path=datarace.log halt_on_error=1"

./cockroach start --insecure --host=localhost --port=26257 --http-port=26258 --store=cockroach-data/1 --cache=256MiB --background
./cockroach start --insecure --host=localhost --port=26259 --http-port=26260 --store=cockroach-data/2 --cache=256MiB --join=localhost:26257 --background
./cockroach start --insecure --host=localhost --port=26261 --http-port=26262 --store=cockroach-data/3 --cache=256MiB --join=localhost:26257 --background
./cockroach start --insecure --host=localhost --port=26263 --http-port=26264 --store=cockroach-data/4 --cache=256MiB --join=localhost:26257 --background

./bin/workload run kv --init --splits 100 --read-percent 50 --batch 64 --concurrency 4 postgresql://root@localhost:26257?sslmode=disable postgresql://root@localhost:26259?sslmode=disable postgresql://root@localhost:26261?sslmode=disable postgresql://root@localhost:26263?sslmode=disable
