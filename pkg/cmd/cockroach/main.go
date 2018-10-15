package main

// #cgo CPPFLAGS: -I../../../c-deps/libroach/include
// #cgo CPPFLAGS: -I.
// #cgo CPPFLAGS: -I../../../c-deps
// #include <foo.h>
import "C"
import "os/exec"

func main() {
	//_ = C.DBOpenHookCCL
	_ = C.FooAESNI

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
