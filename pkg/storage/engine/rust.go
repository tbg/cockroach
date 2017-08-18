package engine

import (
	"io/ioutil"
	"unsafe"
)

/*
#cgo LDFLAGS: -L../../../rs-deps/libroachrs/target/release -lroachrs -lresolv
#cgo CPPFLAGS: -I../../../rs-deps/libroachrs/include
#include <stdlib.h>
#include <libroach.h>
#include <libroachrs.h>
*/
import "C"

func Run() {
	var rdb *C.DBEngine

	key := MVCCKey{
		Key: []byte("foo"),
	}

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

	const val = "💖bar💖"
	k, v := C.CString(string(key.Key)), C.CString(val)
	defer C.free(unsafe.Pointer(k))
	defer C.free(unsafe.Pointer(v))

	C.dbengine_put(rdb, k, v)

	var dbStr C.DBString
	if status := C.dbengine_get(rdb, goToCKey(key), &dbStr); status.data != nil {
		C.from_go(status.data, status.len)
		panic("get failed")
	}
	actualVal := cStringToGoBytes(dbStr)
	if s := string(actualVal); s != val {
		panic(s)
	}
	C.dbengine_close(rdb)
}
