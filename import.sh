#!/bin/bash
set -euo pipefail

cluster=tobias-test

roachprod list | grep $cluster || roachprod create $cluster -n 4

roachprod wipe $cluster
roachprod ssh $cluster -- rm -rf logs
roachprod put $cluster cockroach-linux-2.6.32-gnu-amd64 ./cockroach
roachprod start $cluster

for i in 1 2 3 4; do
	echo $(roachprod adminui $cluster:$i)"/#/metrics/queues/cluster"
done

roachprod sql tobias-test:1 <<EOF
SET CLUSTER SETTING trace.debug.enable = true;
SET CLUSTER SETTING kv.range_merge.queue_enabled = false;
CREATE DATABASE csv;
EOF


roachprod sql tobias-test:1 <<EOF
RESTORE csv.bank FROM
'gs://cockroach-fixtures/workload/bank/version=1.0.0,payload-bytes=10240,ranges=0,rows=500000,seed=1/bank'
WITH into_db = 'csv';
EOF

#roachprod sql tobias-test:1 <<EOF
#IMPORT TABLE csv.lineitem CREATE USING 'gs://cockroach-fixtures/tpch-csv/schema/lineitem.sql' CSV DATA (
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.1',
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.2',
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.3',
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.4',
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.5',
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.6',
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.7',
# 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.8'
#) WITH delimiter='|' ;
#EOF

