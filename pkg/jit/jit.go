// Copyright 2017 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package jit

// This is awkward. LLVM has a nice CMake setup that allows specifying all
// of the linking in libroach's CMakeLists, but the resulting library does
// not actually "scoop up" all of llvm's libraries. Instead, we have to
// teach cgo the output of various `llvm-config` invocations.
//
// TODO(tschottdorf): generate this, perhaps similar to zcgo_*.

// #cgo CPPFLAGS: -I../../c-deps/libroach/include
// #cgo CPPFLAGS: -I../../../c-deps/llvm/include
// #cgo LDFLAGS: -lroach
//
// // From `llvm-config --ldflags`
// #cgo LDFLAGS: -Wl,-search_paths_first -Wl,-headerpad_max_install_names
//
// // From `llvm-config --system-libs`
// #cgo LDFLAGS: -lcurses -lz -lm
//
// // Just link ... everything (definitely not necessary).
// // From `for i in $(llvm-config --libs); do echo "// #cgo LDFLAGS: $i"; done`
// #cgo LDFLAGS: -lLLVMLTO
// #cgo LDFLAGS: -lLLVMPasses
// #cgo LDFLAGS: -lLLVMObjCARCOpts
// #cgo LDFLAGS: -lLLVMSymbolize
// #cgo LDFLAGS: -lLLVMDebugInfoPDB
// #cgo LDFLAGS: -lLLVMDebugInfoDWARF
// #cgo LDFLAGS: -lLLVMMIRParser
// #cgo LDFLAGS: -lLLVMCoverage
// #cgo LDFLAGS: -lLLVMTableGen
// #cgo LDFLAGS: -lLLVMDlltoolDriver
// #cgo LDFLAGS: -lLLVMOrcJIT
// #cgo LDFLAGS: -lLLVMTestingSupport
// #cgo LDFLAGS: -lLLVMXCoreDisassembler
// #cgo LDFLAGS: -lLLVMXCoreCodeGen
// #cgo LDFLAGS: -lLLVMXCoreDesc
// #cgo LDFLAGS: -lLLVMXCoreInfo
// #cgo LDFLAGS: -lLLVMXCoreAsmPrinter
// #cgo LDFLAGS: -lLLVMSystemZDisassembler
// #cgo LDFLAGS: -lLLVMSystemZCodeGen
// #cgo LDFLAGS: -lLLVMSystemZAsmParser
// #cgo LDFLAGS: -lLLVMSystemZDesc
// #cgo LDFLAGS: -lLLVMSystemZInfo
// #cgo LDFLAGS: -lLLVMSystemZAsmPrinter
// #cgo LDFLAGS: -lLLVMSparcDisassembler
// #cgo LDFLAGS: -lLLVMSparcCodeGen
// #cgo LDFLAGS: -lLLVMSparcAsmParser
// #cgo LDFLAGS: -lLLVMSparcDesc
// #cgo LDFLAGS: -lLLVMSparcInfo
// #cgo LDFLAGS: -lLLVMSparcAsmPrinter
// #cgo LDFLAGS: -lLLVMPowerPCDisassembler
// #cgo LDFLAGS: -lLLVMPowerPCCodeGen
// #cgo LDFLAGS: -lLLVMPowerPCAsmParser
// #cgo LDFLAGS: -lLLVMPowerPCDesc
// #cgo LDFLAGS: -lLLVMPowerPCInfo
// #cgo LDFLAGS: -lLLVMPowerPCAsmPrinter
// #cgo LDFLAGS: -lLLVMNVPTXCodeGen
// #cgo LDFLAGS: -lLLVMNVPTXDesc
// #cgo LDFLAGS: -lLLVMNVPTXInfo
// #cgo LDFLAGS: -lLLVMNVPTXAsmPrinter
// #cgo LDFLAGS: -lLLVMMSP430CodeGen
// #cgo LDFLAGS: -lLLVMMSP430Desc
// #cgo LDFLAGS: -lLLVMMSP430Info
// #cgo LDFLAGS: -lLLVMMSP430AsmPrinter
// #cgo LDFLAGS: -lLLVMMipsDisassembler
// #cgo LDFLAGS: -lLLVMMipsCodeGen
// #cgo LDFLAGS: -lLLVMMipsAsmParser
// #cgo LDFLAGS: -lLLVMMipsDesc
// #cgo LDFLAGS: -lLLVMMipsInfo
// #cgo LDFLAGS: -lLLVMMipsAsmPrinter
// #cgo LDFLAGS: -lLLVMLanaiDisassembler
// #cgo LDFLAGS: -lLLVMLanaiCodeGen
// #cgo LDFLAGS: -lLLVMLanaiAsmParser
// #cgo LDFLAGS: -lLLVMLanaiDesc
// #cgo LDFLAGS: -lLLVMLanaiAsmPrinter
// #cgo LDFLAGS: -lLLVMLanaiInfo
// #cgo LDFLAGS: -lLLVMHexagonDisassembler
// #cgo LDFLAGS: -lLLVMHexagonCodeGen
// #cgo LDFLAGS: -lLLVMHexagonAsmParser
// #cgo LDFLAGS: -lLLVMHexagonDesc
// #cgo LDFLAGS: -lLLVMHexagonInfo
// #cgo LDFLAGS: -lLLVMBPFDisassembler
// #cgo LDFLAGS: -lLLVMBPFCodeGen
// #cgo LDFLAGS: -lLLVMBPFDesc
// #cgo LDFLAGS: -lLLVMBPFInfo
// #cgo LDFLAGS: -lLLVMBPFAsmPrinter
// #cgo LDFLAGS: -lLLVMARMDisassembler
// #cgo LDFLAGS: -lLLVMARMCodeGen
// #cgo LDFLAGS: -lLLVMARMAsmParser
// #cgo LDFLAGS: -lLLVMARMDesc
// #cgo LDFLAGS: -lLLVMARMInfo
// #cgo LDFLAGS: -lLLVMARMAsmPrinter
// #cgo LDFLAGS: -lLLVMAMDGPUDisassembler
// #cgo LDFLAGS: -lLLVMAMDGPUCodeGen
// #cgo LDFLAGS: -lLLVMAMDGPUAsmParser
// #cgo LDFLAGS: -lLLVMAMDGPUDesc
// #cgo LDFLAGS: -lLLVMAMDGPUInfo
// #cgo LDFLAGS: -lLLVMAMDGPUAsmPrinter
// #cgo LDFLAGS: -lLLVMAMDGPUUtils
// #cgo LDFLAGS: -lLLVMAArch64Disassembler
// #cgo LDFLAGS: -lLLVMAArch64CodeGen
// #cgo LDFLAGS: -lLLVMAArch64AsmParser
// #cgo LDFLAGS: -lLLVMAArch64Desc
// #cgo LDFLAGS: -lLLVMAArch64Info
// #cgo LDFLAGS: -lLLVMAArch64AsmPrinter
// #cgo LDFLAGS: -lLLVMAArch64Utils
// #cgo LDFLAGS: -lLLVMObjectYAML
// #cgo LDFLAGS: -lLLVMLibDriver
// #cgo LDFLAGS: -lLLVMOption
// #cgo LDFLAGS: -lgtest_main
// #cgo LDFLAGS: -lgtest
// #cgo LDFLAGS: -lLLVMX86Disassembler
// #cgo LDFLAGS: -lLLVMX86AsmParser
// #cgo LDFLAGS: -lLLVMX86CodeGen
// #cgo LDFLAGS: -lLLVMGlobalISel
// #cgo LDFLAGS: -lLLVMSelectionDAG
// #cgo LDFLAGS: -lLLVMAsmPrinter
// #cgo LDFLAGS: -lLLVMDebugInfoCodeView
// #cgo LDFLAGS: -lLLVMDebugInfoMSF
// #cgo LDFLAGS: -lLLVMX86Desc
// #cgo LDFLAGS: -lLLVMMCDisassembler
// #cgo LDFLAGS: -lLLVMX86Info
// #cgo LDFLAGS: -lLLVMX86AsmPrinter
// #cgo LDFLAGS: -lLLVMX86Utils
// #cgo LDFLAGS: -lLLVMMCJIT
// #cgo LDFLAGS: -lLLVMLineEditor
// #cgo LDFLAGS: -lLLVMInterpreter
// #cgo LDFLAGS: -lLLVMExecutionEngine
// #cgo LDFLAGS: -lLLVMRuntimeDyld
// #cgo LDFLAGS: -lLLVMCodeGen
// #cgo LDFLAGS: -lLLVMTarget
// #cgo LDFLAGS: -lLLVMCoroutines
// #cgo LDFLAGS: -lLLVMipo
// #cgo LDFLAGS: -lLLVMInstrumentation
// #cgo LDFLAGS: -lLLVMVectorize
// #cgo LDFLAGS: -lLLVMScalarOpts
// #cgo LDFLAGS: -lLLVMLinker
// #cgo LDFLAGS: -lLLVMIRReader
// #cgo LDFLAGS: -lLLVMAsmParser
// #cgo LDFLAGS: -lLLVMInstCombine
// #cgo LDFLAGS: -lLLVMTransformUtils
// #cgo LDFLAGS: -lLLVMBitWriter
// #cgo LDFLAGS: -lLLVMAnalysis
// #cgo LDFLAGS: -lLLVMProfileData
// #cgo LDFLAGS: -lLLVMObject
// #cgo LDFLAGS: -lLLVMMCParser
// #cgo LDFLAGS: -lLLVMMC
// #cgo LDFLAGS: -lLLVMBitReader
// #cgo LDFLAGS: -lLLVMCore
// #cgo LDFLAGS: -lLLVMBinaryFormat
// #cgo LDFLAGS: -lLLVMSupport
// #cgo LDFLAGS: -lLLVMDemangle
//
// #include <stdlib.h>
// #include <llvmpoc.h>
import "C"

func dummy() int {
	result := C.JITExample()
	return int(result)
}
