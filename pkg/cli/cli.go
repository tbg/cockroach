// Copyright 2015 The Cockroach Authors.
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

package cli

import (
	"context"
	_ "fmt"
	"os"
	_ "strings"
	_ "text/tabwriter"

	_ "github.com/benesch/cgosymbolizer" // calls runtime.SetCgoTraceback on import
	_ "github.com/cockroachdb/cockroach/pkg/build"
	_ "github.com/cockroachdb/cockroach/pkg/util/log"
	_ "github.com/cockroachdb/cockroach/pkg/workload/bank" // intentionally not all the workloads in pkg/ccl/workloadccl/allccl
	_ "github.com/cockroachdb/cockroach/pkg/workload/examples"
	_ "github.com/cockroachdb/cockroach/pkg/workload/tpcc"
	gopsnet "github.com/shirou/gopsutil/net"
	_ "github.com/spf13/cobra"
)

// Main is the entry point for the cli, with a single line calling it intended
// to be the body of an action package main `main` func elsewhere. It is
// abstracted for reuse by duplicated `main` funcs in different distributions.
func Main() {
	_, err := gopsnet.IOCountersWithContext(context.TODO(), true)
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
