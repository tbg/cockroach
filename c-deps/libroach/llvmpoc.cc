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

#include "llvm/Support/TargetSelect.h" // For InitializeNativeTarget() etc.
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/Module.h"
#include "llvm/IR/LLVMContext.h"
#include "llvm/IR/Verifier.h"

#include "KaleidoscopeJIT.h"
#include <llvmpoc.h>

using namespace llvm;

// This example is mostly cribbed from
// https://stackoverflow.com/questions/42982070/llvm-simple-example-of-a-just-in-time-compilation

static LLVMContext Context;
static auto MyModule = make_unique<Module>("myjit", Context);

Function *createFunc(IRBuilder<> &Builder, std::string Name) {
    std::vector<Type*> Integers(2, Builder.getInt32Ty());
    auto *funcType = FunctionType::get(Builder.getInt32Ty(), Integers, false);
    auto *f = Function::Create(funcType, Function::ExternalLinkage, Name, MyModule.get());
    return f;
};

Function *createExtFunc(IRBuilder<> &Builder, std::string Name) {
    std::vector<Type*> Integers(2, Builder.getInt32Ty());
    auto *funcType = FunctionType::get(Builder.getInt32Ty(), Integers, false);
    // MyModule->getOrInsertFunction(Name, funcType);
    auto *f = Function::Create(funcType, Function::ExternalLinkage, Name, MyModule.get());
    return f;
}

int mymul(int i, int j) {
    return i * j;
}

int mymod(int i, int j) {
    return i % j;
}

int JITExample() {
    InitializeNativeTarget();
    InitializeNativeTargetAsmPrinter();
    InitializeNativeTargetAsmParser();


    // Build a simple program in IR which we'll JIT compile and run.
    {
        static IRBuilder<> Builder(Context);

        // Declare the function that will hold our logic.
        auto *fooFunc = createFunc(Builder, "mycalc");
        // Declare a code block which will be the function body.
        auto *entry = BasicBlock::Create(Context, "entry", fooFunc);
        Builder.SetInsertPoint(entry);

        // Fill the function body.
        auto args = fooFunc->arg_begin();
        Value *arg1 = &(*args);
        args = std::next(args);
        Value *arg2 = &(*args);
        auto *sum = Builder.CreateAdd(arg1, arg2, "tmp");

        // Declare the external func `mymod` and generate a function call to it.
        // Note that `mymod` is a C++ function, defined above, and exposed in `llvmpoc.h`.
        auto *modFunc = createExtFunc(Builder, "mymod");
        std::vector<Value *> call_args = {arg1, arg2};
        auto *mod = Builder.CreateCall(modFunc, call_args, "mymodtmp");

        // Bogus-collapse the results into one: res = mysum(a,b) + mymod(a,b).
        auto res = Builder.CreateAdd(sum, mod, "restmp");
        Builder.CreateRet(res);

        // Run some sanity checks on the function we put together.
        verifyFunction(*fooFunc);
    };

    orc::KaleidoscopeJIT jit;
    MyModule->setDataLayout(jit.getTargetMachine().createDataLayout());
    auto moduleHandle = jit.addModule(std::move(MyModule)); // JIT-compile MyModule.

    int result = 0;

    // Call into the IR we just compiled.
    {
        auto symbol = jit.findSymbol("mycalc"); // Get the compiled sum function.
        auto f = (int(*)(int, int)) cantFail(symbol.getAddress()); // Cast it.
        auto r = f(42, 13); // Call it.
        assert(r == 84); // (42+13) + (42 % 13) = 55 + 3 = 58
        result += r;
    }

    // Call into `mul` which is external (defined above). Note that this is not
    // the same as calling `mul` from inside the jit'ed code (which is what we
    // do with `mymod`). We're just testing that `findSymbol` works.
    {
        auto symbol = jit.findSymbol("mymul");
        auto f = (int(*)(int, int)) cantFail(symbol.getAddress());
        auto r = f(4, 5);
        assert(r == 20);
        result += r;
    }

    jit.removeModule(moduleHandle);
    return result;
}
