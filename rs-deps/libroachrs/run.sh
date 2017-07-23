#!/bin/sh
rm target/release/librust*; rm -rf dummy-storage-location; cargo build --release && cargo test --release && go run main.go
