package main

import (
	"context" // _ "github.com/cockroachdb/cockroach/pkg/ccl" // ccl init hooks

	_ "github.com/cockroachdb/cockroach/pkg/storage/engine"
	gopsnet "github.com/shirou/gopsutil/net"
)

// #cgo CPPFLAGS: -I../../../c-deps/libroach/include
// #cgo LDFLAGS: -ljemalloc
// #cgo LDFLAGS: -lroach
// #cgo LDFLAGS: -lroachccl
// #cgo LDFLAGS: -lprotobuf
// #cgo LDFLAGS: -lrocksdb
// #cgo LDFLAGS: -lsnappy
// #cgo LDFLAGS: -lcryptopp
// #cgo linux LDFLAGS: -lrt -lpthread
// #cgo windows LDFLAGS: -lshlwapi -lrpcrt4
//
// #include <stdlib.h>
// #include <libroach.h>
// #include <libroachccl.h>
import "C"

func main() {
	C.DBSetOpenHook(C.DBOpenHookCCL)
	_, _ = gopsnet.IOCountersWithContext(context.TODO(), true)
}
