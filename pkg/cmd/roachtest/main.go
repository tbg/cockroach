// Copyright 2018 The Cockroach Authors.
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
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/user"
	"time"

	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
	"github.com/spf13/cobra"
)

func main() {
	rand.Seed(timeutil.Now().UnixNano())
	username := os.Getenv("ROACHPROD_USER")
	parallelism := 10
	var cpuQuota int
	// Path to a local dir where the test logs and artifacts collected from
	// cluster will be placed.
	var artifacts string
	var clusterLifetimeOverrideS string
	var clusterLifetimeOverride time.Duration
	var httpPort int
	var debugEnabled bool

	cobra.EnableCommandSorting = false

	var rootCmd = &cobra.Command{
		Use:   "roachtest [command] (flags)",
		Short: "roachtest tool for testing cockroach clusters",
		Long: `roachtest is a tool for testing cockroach clusters.
`,

		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Don't bother checking flags for the default help command.
			if cmd.Name() == "help" {
				return nil
			}

			if clusterLifetimeOverrideS != "" {
				var err error
				clusterLifetimeOverride, err = time.ParseDuration(clusterLifetimeOverrideS)
				if err != nil {
					return fmt.Errorf("invalid --cluster-lifetime-override: %s", clusterLifetimeOverrideS)
				}
			}

			if clusterName != "" && local {
				return fmt.Errorf("Cannot specify both an existing cluster (%s) and --local. However, if a local cluster already exists, --clusters=local will use it.", clusterName)
			}
			switch cmd.Name() {
			case "run", "bench", "store-gen":
				initBinaries()
			}
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVarP(
		&clusterName, "cluster", "c", "",
		"Comma-separated list of names existing cluster to use for running tests. "+
			"If fewer than --parallelism names are specified, then the parallelism "+
			"is capped to the number of clusters specified.")
	rootCmd.PersistentFlags().BoolVarP(
		&local, "local", "l", local, "run tests locally")
	rootCmd.PersistentFlags().StringVarP(
		&username, "user", "u", username,
		"Username to use as a cluster name prefix. "+
			"If blank, the current OS user is detected and specified.")
	rootCmd.PersistentFlags().StringVar(
		&cockroach, "cockroach", "", "path to cockroach binary to use")
	rootCmd.PersistentFlags().StringVar(
		&workload, "workload", "", "path to workload binary to use")
	f := rootCmd.PersistentFlags().VarPF(
		&encrypt, "encrypt", "", "start cluster with encryption at rest turned on")
	f.NoOptDefVal = "true"

	var listBench bool

	var listCmd = &cobra.Command{
		Use:   "list [tests]",
		Short: "list tests matching the patterns",
		Long: `List tests that match the given name patterns.

If no pattern is passed, all tests are matched.
Use --bench to list benchmarks instead of tests.

Each test has a set of tags. The tags are used to skip tests which don't match
the tag filter. The tag filter is specified by specifying a pattern with the
"tag:" prefix. The default tag filter is "tag:default" which matches any test
that has the "default" tag. Note that tests are selected based on their name,
and skipped based on their tag.

Examples:

   roachtest list acceptance copy/bank/.*false
   roachtest list tag:acceptance
   roachtest list tag:weekly
`,
		RunE: func(_ *cobra.Command, args []string) error {
			r := newRegistry(setBuildVersion)
			if !listBench {
				registerTests(r)
			} else {
				registerBenchmarks(r)
			}

			names := r.List(args)
			for _, name := range names {
				fmt.Println(name)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(
		&listBench, "bench", false, "list benchmarks instead of tests")

	var runCmd = &cobra.Command{
		Use:   "run [tests]",
		Short: "run automated tests on cockroach cluster",
		Long: `Run automated tests on existing or ephemeral cockroach clusters.

roachtest run takes a list of regex patterns and runs all the matching tests.
If no pattern is given, all tests are run. See "help list" for more details on
the test tags.
`,
		RunE: func(_ *cobra.Command, args []string) error {
			if count <= 0 {
				return fmt.Errorf("--count (%d) must by greater than 0", count)
			}
			r := newRegistry(setBuildVersion)
			registerTests(r)

			filter := newFilter(args)
			os.Exit(r.Run(
				context.Background(), filter, count, parallelism, cpuQuota,
				clusterName, local, artifacts, getUser(username),
				debugEnabled, clusterLifetimeOverride,
				os.Stdout, os.Stderr, httpPort))
			return nil
		},
	}

	runCmd.Flags().StringVar(
		&buildTag, "build-tag", "", "build tag (auto-detect if empty)")
	runCmd.Flags().StringVar(
		&slackToken, "slack-token", "", "Slack bot token")
	runCmd.Flags().BoolVar(
		&teamCity, "teamcity", false, "include teamcity-specific markers in output")

	var benchCmd = &cobra.Command{
		Use:   "bench [benchmarks]",
		Short: "run automated benchmarks on cockroach cluster",
		Long:  `Run automated benchmarks on existing or ephemeral cockroach clusters.`,
		RunE: func(_ *cobra.Command, args []string) error {
			if count <= 0 {
				return fmt.Errorf("--count (%d) must by greater than 0", count)
			}
			r := newRegistry(setBuildVersion)
			registerBenchmarks(r)

			filter := newFilter(args)
			os.Exit(r.Run(
				context.Background(), filter, count, parallelism, cpuQuota,
				clusterName, local, artifacts, getUser(username), debugEnabled, clusterLifetimeOverride,
				os.Stdout, os.Stderr, httpPort))
			return nil
		},
	}

	// Register flags shared between `run` and `bench`.
	for _, cmd := range []*cobra.Command{runCmd, benchCmd} {
		cmd.Flags().StringVar(
			&artifacts, "artifacts", "artifacts", "path to artifacts directory")
		cmd.Flags().StringVar(
			&cloud, "cloud", cloud, "cloud provider to use (aws or gce)")
		cmd.Flags().StringVar(
			&clusterID, "cluster-id", "", "an identifier to use in the test cluster's name")
		cmd.Flags().IntVar(
			&count, "count", 1, "the number of times to run each test")
		cmd.Flags().BoolVarP(
			&debugEnabled, "debug", "d", debugEnabled, "don't wipe and destroy cluster if test fails")
		cmd.Flags().IntVarP(
			&parallelism, "parallelism", "p", parallelism, "number of tests to run in parallel")
		cmd.Flags().StringVar(
			&roachprod, "roachprod", "", "path to roachprod binary to use")
		cmd.Flags().BoolVar(
			&clusterWipe, "wipe", true,
			"wipe existing cluster before starting test (for use with --cluster)")
		cmd.Flags().StringVar(
			&zonesF, "zones", "", "Zones for the cluster (use roachprod defaults if empty)")
		cmd.Flags().IntVar(
			&cpuQuota, "cpu-quota", 100,
			"The number of cloud CPUs roachtest is allowed to use at any one time.")
		cmd.Flags().IntVar(
			&httpPort, "port", 8080, "the port on which to ")
		cmd.Flags().StringVar(
			&clusterLifetimeOverrideS, "cluster-lifetime-override", "",
			"If set, this is a duration that will "+
				"override testSpec.Nodes[0].Lifetime for all tests. "+
				"Useful, for example, when running a large number of tests that can "+
				"reuse clusters and the default 12h destruction time for those clusters "+
				"is not desirable. E.g. \"24h10m\".")
	}

	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(benchCmd)

	if err := rootCmd.Execute(); err != nil {
		// Cobra has already printed the error message.
		os.Exit(1)
	}
}

// user takes the value passed on the command line and comes up with the
// username to use.
func getUser(userFlag string) string {
	if userFlag != "" {
		return userFlag
	}
	usr, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("user.Current: %s", err))
	}
	return usr.Username
}
