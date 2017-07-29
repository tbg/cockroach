package engine

import (
	"fmt"
	"unsafe"
)

/*
#cgo LDFLAGS: -L../../../rs-deps/libroachrs/target/release -lruststorage
#include "./ruststorage.h"
*/
import "C"

func Run() {
	const (
		key = "foo"
		val = "bar"
	)
	var rdb *C.DBEngine
	dir := C.CString("./foo")
	status := C.dbengine_open(&rdb, dir)
	C.free(unsafe.Pointer(dir))
	if status.len != 0 {
		panic(status.len)
	}

	k, v := C.CString(key), C.CString(val)
	C.dbengine_put(rdb, k, v)
	fmt.Println(C.GoString(C.dbengine_get(rdb, k)))
}
