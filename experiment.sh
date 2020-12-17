#!/bin/bash
set -euxo pipefail


function run {
    mode="${1}"
    shift
    ./bin/roachprod sql tobias-tracing:1 -- -e "set cluster setting trace.mode = '$mode'";
    sleep 1
    ./bin/roachprod run tobias-tracing:4 -- ./cockroach workload run kv '{pgurl:1-3}' "$@"
    ./bin/roachprod sql tobias-tracing:1 -- -e "show cluster setting trace.mode";
}

#./bin.docker_amd64/roachprod create tobias-tracing -n 4
#./bin.docker_amd64/roachprod run tobias-tracing:1-3 "sudo apt -qq update && sudo apt -qq -yy install graphviz"
./bin/roachprod stop tobias-tracing:1-3
./bin/roachprod put tobias-tracing cockroach-linux-2.6.32-gnu-amd64 ./cockroach
./bin/roachprod start tobias-tracing:1-3

# Load a little bit of data.
./bin/roachprod run tobias-tracing:4 -- ./cockroach workload init kv '{pgurl:1-3}' --splits 100
#run legacy     --duration=180s --read-percent 0

# Compare
run background --duration=1800s --read-percent 100
#run legacy     --duration=180s --read-percent 100
