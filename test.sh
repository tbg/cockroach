#!/usr/bin/env bash

set -euo pipefail

key=${1-$(date)}

function finish {
  killall -9 cockroach
}
trap finish EXIT

sudo ntpdate -u time.apple.com

./cockroach start --insecure --background
./cockroach sql --insecure <<EOF
SET CLUSTER SETTING trace.debug.enable = true;
CREATE DATABASE IF NOT EXISTS time;
CREATE TABLE IF NOT EXISTS time.kv (id STRING PRIMARY KEY, v STRING);

UPSERT INTO time.kv VALUES('${key}', '${key}');
EOF

./cockroach quit --insecure

sleep 1

sudo gdate -s '1 minute ago'

#COCKROACH_SIMULATED_OFFSET=-20s
./cockroach start --insecure --background --logtostderr=INFO --background

out=$(echo "SELECT * FROM time.kv WHERE id = '${key}';" | ./cockroach sql --insecure --format=csv)

echo "*****************************"
echo "${out}"
echo "*****************************"
echo "${out}" | grep -F '1 row'
