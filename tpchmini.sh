set -euxo pipefail

killall -9 cockroach || true
killall -9 workload || true
sleep 1
rm -rf cockroach-data || true
mkdir -p cockroach-data

./cockroach start --insecure --host=localhost --port=26257 --http-port=26258 --store=cockroach-data/1 --cache=256MiB --background
./cockroach start --insecure --host=localhost --port=26259 --http-port=26260 --store=cockroach-data/2 --cache=256MiB --join=localhost:26257 --background
./cockroach start --insecure --host=localhost --port=26261 --http-port=26262 --store=cockroach-data/3 --cache=256MiB --join=localhost:26257 --background
./cockroach start --insecure --host=localhost --port=26263 --http-port=26264 --store=cockroach-data/4 --cache=256MiB --join=localhost:26257 --background

sleep 5

./cockroach sql --insecure -e 'set cluster setting kv.range_merge.queue_enabled = false;'

./cockroach sql --insecure <<EOF
CREATE DATABASE csv;
IMPORT TABLE csv.lineitem CREATE USING 'gs://cockroach-fixtures/tpch-csv/schema/lineitem.sql' CSV DATA (
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.1',
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.2',
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.3',
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.4',
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.5',
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.6',
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.7',
 'gs://cockroach-fixtures/tpch-csv/sf-1/lineitem.tbl.8'
) WITH delimiter='|' ;
EOF

sleep 5

for port in 26257 26259 26261 26263; do
 ./cockroach sql --insecure -e "select name, value from crdb_internal.node_metrics where name like '%raftsn%' order by name desc" --port "${port}"
done
