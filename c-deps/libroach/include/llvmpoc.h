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
// implied.  See the License for the specific language governing
// permissions and limitations under the License.

#ifndef LLVMPOC_H
#define LLVMPOC_H

#ifdef __cplusplus
extern "C" {
#endif
int JITExample();
int mymul(int i, int j);
int mymod(int i, int j);
#ifdef __cplusplus
}
#endif
#endif // LLVMPOC_H
