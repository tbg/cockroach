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
	k, v := C.CString(key), C.CString(val)
	_ = v
	fmt.Println(key, "exists:", C.has(k) != 0)
	C.put(k, v)
	fmt.Println(key, "exists:", C.has(k) != 0)
}
