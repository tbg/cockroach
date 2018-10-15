package main

// #cgo CPPFLAGS: -I../../../c-deps/libroach/include
// #cgo LDFLAGS: -ljemalloc
// #cgo LDFLAGS: -lroach
// #cgo LDFLAGS: -lroachccl
// #cgo LDFLAGS: -lprotobuf
// #cgo LDFLAGS: -lrocksdb
// #cgo LDFLAGS: -lsnappy
// #cgo LDFLAGS: -lcryptopp
//
// #include <stdlib.h>
// #include <libroachccl.h>
import "C"
import "os/exec"

func main() {
	_ = C.DBOpenHookCCL

	// passes
	// #include <stdio.h>
	// void myprint(char* s) {
	//	printf("%s\n", s);
	// }
	//	C.myprint(C.CString("hi\n"))

	//	fmt.Printf("hi") // passes

	// boom:
	cmd := exec.Command("echo")
	cmd.Run()

	// C.DBSetOpenHook(C.DBOpenHookCCL)
	//	_, _ = net.IOCountersWithContext(context.TODO(), true)
}
