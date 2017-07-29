#!/bin/sh

set -euxo pipefail

(cd rs-deps/libroachrs &&
	rm -rf target/release/librust* &&
	cargo build --release &&
	cargo test --release)

make test PKG=./pkg/storage/engine TESTS=Rust TESTFLAGS=-v
