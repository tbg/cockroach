# Copyright 2017 The Cockroach Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
# implied. See the License for the specific language governing
# permissions and limitations under the License.

# NB: Despite CMake's portability, this build configuration makes no attempt to
# support non-GCC-like compilers.

# The CXX_STANDARD property was introduced in version 3.1.
cmake_minimum_required(VERSION 3.1 FATAL_ERROR)

project(roachlib)

find_package(LLVM REQUIRED CONFIG)
# message(STATUS "Found LLVM ${LLVM_PACKAGE_VERSION}")
# message(STATUS "Using LLVMConfig.cmake in: ${LLVM_DIR}")

# llvm_map_components_to_libnames(llvm_libs aarch64 aarch64asmparser aarch64asmprinter aarch64codegen aarch64desc aarch64disassembler aarch64info aarch64utils all all-targets amdgpu amdgpuasmparser amdgpuasmprinter amdgpucodegen amdgpudesc amdgpudisassembler amdgpuinfo amdgpuutils analysis arm armasmparser armasmprinter armcodegen armdesc armdisassembler arminfo asmparser asmprinter binaryformat bitreader bitwriter bpf bpfasmprinter bpfcodegen bpfdesc bpfdisassembler bpfinfo codegen core coroutines coverage debuginfocodeview debuginfodwarf debuginfomsf debuginfopdb demangle dlltooldriver engine executionengine globalisel gtest gtest_main hexagon hexagonasmparser hexagoncodegen hexagondesc hexagondisassembler hexagoninfo instcombine instrumentation interpreter ipo irreader lanai lanaiasmparser lanaiasmprinter lanaicodegen lanaidesc lanaidisassembler lanaiinfo libdriver lineeditor linker lto mc mcdisassembler mcjit mcparser mips mipsasmparser mipsasmprinter mipscodegen mipsdesc mipsdisassembler mipsinfo mirparser msp430 msp430asmprinter msp430codegen msp430desc msp430info native nativecodegen nvptx nvptxasmprinter nvptxcodegen nvptxdesc nvptxinfo objcarcopts object objectyaml option orcjit passes powerpc powerpcasmparser powerpcasmprinter powerpccodegen powerpcdesc powerpcdisassembler powerpcinfo profiledata runtimedyld scalaropts selectiondag sparc sparcasmparser sparcasmprinter sparccodegen sparcdesc sparcdisassembler sparcinfo support symbolize systemz systemzasmparser systemzasmprinter systemzcodegen systemzdesc systemzdisassembler systemzinfo tablegen target testingsupport transformutils vectorize x86 x86asmparser x86asmprinter x86codegen x86desc x86disassembler x86info x86utils xcore xcoreasmprinter xcorecodegen xcoredesc xcoredisassembler xcoreinfo)

include_directories(${LLVM_INCLUDE_DIRS})
# add_definitions(${LLVM_DEFINITIONS})

add_library(roach
  llvmpoc.cc
  db.cc
  encoding.cc
  eventlistener.cc
  protos/roachpb/data.pb.cc
  protos/roachpb/internal.pb.cc
  protos/roachpb/metadata.pb.cc
  protos/storage/engine/enginepb/mvcc.pb.cc
  protos/storage/engine/enginepb/mvcc3.pb.cc
  protos/storage/engine/enginepb/rocksdb.pb.cc
  protos/util/hlc/legacy_timestamp.pb.cc
  protos/util/hlc/timestamp.pb.cc
  protos/util/unresolved_addr.pb.cc
)
target_include_directories(roach
  PUBLIC  ./include
  PRIVATE ../protobuf/src ../rocksdb/include protos
)

add_library(roachccl
  ccl/db.cc
)
target_include_directories(roachccl
  PRIVATE ../rocksdb/include
)

# target_link_libraries(roach ${llvm_libs})
target_link_libraries(roachccl roach)

set_target_properties(roach roachccl PROPERTIES
  CXX_STANDARD 11
  CXX_STANDARD_REQUIRED YES
  CXX_EXTENSIONS NO
  COMPILE_OPTIONS "-Werror;-Wall;-Wno-sign-compare"
)
