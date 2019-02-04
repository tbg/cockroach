#!/bin/bash

set -euxo pipefail

need_import=1

if [ -d cockroach-data/0 ]; then
	need_import=0
fi

(killall -9 cockroach ; killall -9 workload) || true

make build bin/workload

for i in $(seq 0 3); do
	./cockroach start --insecure --port $((26257+i)) --http-port $((8080+i)) --store cockroach-data/$i --join :26257 2> /dev/null &
done

warehouses=100

if [ $need_import -ne 0 ]; then
	sleep 1
	./cockroach init --insecure
	sleep 1
	./cockroach sql --insecure -e "
SET CLUSTER SETTING cluster.organization = 'Cockroach Labs - Production Testing'; SET CLUSTER SETTING enterprise.license = '...';
SET CLUSTER SETTING kv.range_merge.queue_enabled = false;
SET CLUSTER SETTING kv.range_split.by_load_enabled = false;
	";
	sleep 1
	./bin/workload fixtures load tpcc --warehouses $warehouses
fi

sleep 5
./bin/workload run tpcc --expensive-checks --scatter --warehouses $warehouses --ramp 30s --wait=false --tolerate-errors --duration=24h postgres://root@localhost:262{57,58,59,60}?sslmode=disable --histograms tpcc.json &

sleep 5

while true; do
#ALTER TABLE tpcc.district SPLIT AT SELECT i FROM generate_series(1, $((10*warehouses-1))) AS g(i);
#ALTER TABLE tpcc.district SCATTER;
	./cockroach sql --insecure -e "
SET CLUSTER SETTING kv.range_merge.queue_enabled = false;
ALTER TABLE tpcc.warehouse SPLIT AT SELECT i FROM generate_series(1, $((warehouses-1))) AS g(i);
select pg_sleep(30);
SET CLUSTER SETTING kv.range_merge.queue_enabled = true;
ALTER TABLE tpcc.warehouse SCATTER;
"
	./cockroach sql --insecure < check*.sql
	sleep 30
done;
