#!/bin/sh

set -euxo pipefail

rm target/release/librust*
rm -rf dummy-storage-location
cargo build --release
cargo test --release
make -C ../.. test PKG=./rs-deps/libroachrs TESTFLAGS=-v
rm -rf foo || true
