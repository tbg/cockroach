package main

import (
	"os/exec"

	_ "github.com/cockroachdb/cockroach/pkg/storage/engine"
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
	_ = C.DBOpenHookCCL

	cmd := exec.Command("echo")
	cmd.Run()

	// C.DBSetOpenHook(C.DBOpenHookCCL)
	//	_, _ = net.IOCountersWithContext(context.TODO(), true)
}
