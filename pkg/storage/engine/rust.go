package engine

import (
	"fmt"
	"io/ioutil"
	"unsafe"
)

/*
#cgo LDFLAGS: -L../../../rs-deps/libroachrs/target/release -lroachrs
#cgo CPPFLAGS: -I../../../rs-deps/libroachrs/include
#include <stdlib.h>
#include <libroach.h>
#include <libroachrs.h>
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
	cDir := C.CString(dir)
	status := C.dbengine_open(&rdb, cDir)
	C.free(unsafe.Pointer(cDir))
	if status.len != 0 {
		panic(status.len)
	}

	k, v := C.CString(key), C.CString(val)
	C.dbengine_put(rdb, k, v)
	fmt.Println(C.GoString(C.dbengine_get(rdb, k)))
	C.dbengine_close(rdb)
}
