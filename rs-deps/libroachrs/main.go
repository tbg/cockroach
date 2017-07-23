package main

import (
	"fmt"
)

/*
#cgo LDFLAGS: -L./target/release -lruststorage
#include "./ruststorage.h"
*/
import "C"

func main() {
	const (
		key = "foo"
		val = "bar"
	)
	db := C.dbengine_open(C.CString("./foo"))

	k, v := C.CString(key), C.CString(val)
	C.dbengine_put(db, k, v)
	fmt.Println(C.GoString(C.dbengine_get(db, k)))
}
