#!/bin/bash

set -euxo pipefail

pkill -9 roach || true

COCKROACH_SCAN_INTERVAL=10s COCKROACH_CONSISTENCY_CHECK_INTERVAL=1s  ./cockroach start --insecure &

COCKROACH_SCAN_INTERVAL=10s COCKROACH_CONSISTENCY_CHECK_INTERVAL=1s ./cockroach start --insecure --http-port 8081 --port 26258 --store cockroach-data2 --join :26257 &

COCKROACH_SCAN_INTERVAL=10s COCKROACH_CONSISTENCY_CHECK_INTERVAL=1s ./cockroach start --insecure --http-port 8082 --port 26259 --store cockroach-data3 --join :26257 --logtostderr &


sleep 5
./cockroach sql --insecure -e "set cluster setting server.consistency_check.interval = '1s'" || true;

sleep 25

kill -9 $(pgrep roach | gshuf | head -n 1)
sleep 5
kill -9 $(pgrep roach | gshuf | head -n 1)
sleep 5
kill -9 $(pgrep roach | gshuf | head -n 1)
