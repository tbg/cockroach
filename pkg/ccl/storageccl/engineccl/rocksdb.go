// Copyright 2017 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/licenses/CCL.txt

package engineccl

import (
	"github.com/cockroachdb/cockroach/pkg/storage/engine"
)

// TODO(tamird): why does rocksdb not link jemalloc,snappy statically?

// #cgo CPPFLAGS: -I../../../../c-deps/libroach/include
// #cgo LDFLAGS: -ljemalloc
// #cgo LDFLAGS: -lroachccl
// #cgo LDFLAGS: -lroach
// #cgo LDFLAGS: -lprotobuf
// #cgo LDFLAGS: -lrocksdb
// #cgo LDFLAGS: -lsnappy
// #cgo LDFLAGS: -lcryptopp
// #cgo linux LDFLAGS: -lrt -lpthread
// #cgo windows LDFLAGS: -lshlwapi -lrpcrt4
//
// #include <stdlib.h>
// #include <libroachccl.h>
import "C"

func init() {
	engine.SetRocksDBOpenHook(C.DBOpenHookCCL)
}
