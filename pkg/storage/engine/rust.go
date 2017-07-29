package engine

import (
	"fmt"
	"io/ioutil"
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

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		panic(err)
	}
	status := C.dbengine_open(&rdb, dir)
	C.free(unsafe.Pointer(dir))
	if status.len != 0 {
		panic(status.len)
	}

	k, v := C.CString(key), C.CString(val)
	C.dbengine_put(rdb, k, v)
	fmt.Println(C.GoString(C.dbengine_get(rdb, k)))
	C.dbengine_close(rdb)
}
