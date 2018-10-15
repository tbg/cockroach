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

// +build ignore

package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/base"
	"github.com/cockroachdb/cockroach/pkg/build"
	"github.com/cockroachdb/cockroach/pkg/cli/cliflags"
	"github.com/cockroachdb/cockroach/pkg/rpc"
	"github.com/cockroachdb/cockroach/pkg/server"
	"github.com/cockroachdb/cockroach/pkg/server/serverpb"
	"github.com/cockroachdb/cockroach/pkg/server/status"
	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/storage/engine"
	"github.com/cockroachdb/cockroach/pkg/util/envutil"
	"github.com/cockroachdb/cockroach/pkg/util/grpcutil"
	"github.com/cockroachdb/cockroach/pkg/util/hlc"
	"github.com/cockroachdb/cockroach/pkg/util/humanizeutil"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/log/logflags"
	"github.com/cockroachdb/cockroach/pkg/util/stop"
	"github.com/cockroachdb/cockroach/pkg/util/sysutil"
	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
	"github.com/pkg/errors"
	gopsnet "github.com/shirou/gopsutil/net"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// jemallocHeapDump is an optional function to be called at heap dump time.
// This will be non-nil when jemalloc is linked in with profiling enabled.
// The function takes a filename to write the profile to.
var jemallocHeapDump func(string) error

// StartCmd starts a node by initializing the stores and joining
// the cluster.
var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "start a node",
	Long: `
Start a CockroachDB node, which will export data from one or more
storage devices, specified via --store flags.

If no cluster exists yet and this is the first node, no additional
flags are required. If the cluster already exists, and this node is
uninitialized, specify the --join flag to point to any healthy node
(or list of nodes) already part of the cluster.
`,
	Example: `  cockroach start --insecure --store=attrs=ssd,path=/mnt/ssd1 [--join=host:port,[host:port]]`,
	Args:    cobra.NoArgs,
	RunE:    maybeShoutError(MaybeDecorateGRPCError(runStart)),
}

// maxSizePerProfile is the maximum total size in bytes for profiles per
// profile type.
var maxSizePerProfile = envutil.EnvOrDefaultInt64(
	"COCKROACH_MAX_SIZE_PER_PROFILE", 100<<20 /* 100 MB */)

// gcProfiles removes old profiles matching the specified prefix when the sum
// of newer profiles is larger than maxSize. Requires that the suffix used for
// the profiles indicates age (e.g. by using a date/timestamp suffix) such that
// sorting the filenames corresponds to ordering the profiles from oldest to
// newest.
func gcProfiles(dir, prefix string, maxSize int64) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Warning(context.Background(), err)
		return
	}
	var sum int64
	var found int
	for i := len(files) - 1; i >= 0; i-- {
		f := files[i]
		if !f.Mode().IsRegular() {
			continue
		}
		if !strings.HasPrefix(f.Name(), prefix) {
			continue
		}
		found++
		sum += f.Size()
		if found == 1 {
			// Always keep the most recent profile.
			continue
		}
		if sum <= maxSize {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil {
			log.Info(context.Background(), err)
		}
	}
}

func initMemProfile(ctx context.Context, dir string) {
	const jeprof = "jeprof."
	const memprof = "memprof."

	gcProfiles(dir, jeprof, maxSizePerProfile)
	gcProfiles(dir, memprof, maxSizePerProfile)

	memProfileInterval := envutil.EnvOrDefaultDuration("COCKROACH_MEMPROF_INTERVAL", -1)
	if memProfileInterval <= 0 {
		return
	}
	if min := time.Second; memProfileInterval < min {
		log.Infof(ctx, "fixing excessively short memory profiling interval: %s -> %s",
			memProfileInterval, min)
		memProfileInterval = min
	}

	if jemallocHeapDump != nil {
		log.Infof(ctx, "writing go and jemalloc memory profiles to %s every %s", dir, memProfileInterval)
	} else {
		log.Infof(ctx, "writing go only memory profiles to %s every %s", dir, memProfileInterval)
		log.Infof(ctx, `to enable jmalloc profiling: "export MALLOC_CONF=prof:true" or "ln -s prof:true /etc/malloc.conf"`)
	}

	go func() {
		ctx := context.Background()
		t := time.NewTicker(memProfileInterval)
		defer t.Stop()

		for {
			<-t.C

			func() {
				const format = "2006-01-02T15_04_05.999"
				suffix := timeutil.Now().Format(format)

				// Try jemalloc heap profile first, we only log errors.
				if jemallocHeapDump != nil {
					jepath := filepath.Join(dir, jeprof+suffix)
					if err := jemallocHeapDump(jepath); err != nil {
						log.Warningf(ctx, "error writing jemalloc heap %s: %s", jepath, err)
					}
					gcProfiles(dir, jeprof, maxSizePerProfile)
				}

				path := filepath.Join(dir, memprof+suffix)
				// Try writing a go heap profile.
				f, err := os.Create(path)
				if err != nil {
					log.Warningf(ctx, "error creating go heap file %s", err)
					return
				}
				defer f.Close()
				if err = pprof.WriteHeapProfile(f); err != nil {
					log.Warningf(ctx, "error writing go heap %s: %s", path, err)
					return
				}
				gcProfiles(dir, memprof, maxSizePerProfile)
			}()
		}
	}()
}

func initCPUProfile(ctx context.Context, dir string) {
	const cpuprof = "cpuprof."
	gcProfiles(dir, cpuprof, maxSizePerProfile)

	cpuProfileInterval := envutil.EnvOrDefaultDuration("COCKROACH_CPUPROF_INTERVAL", -1)
	if cpuProfileInterval <= 0 {
		return
	}
	if min := time.Second; cpuProfileInterval < min {
		log.Infof(ctx, "fixing excessively short cpu profiling interval: %s -> %s",
			cpuProfileInterval, min)
		cpuProfileInterval = min
	}

	go func() {
		defer log.RecoverAndReportPanic(ctx, &serverCfg.Settings.SV)

		ctx := context.Background()

		t := time.NewTicker(cpuProfileInterval)
		defer t.Stop()

		var currentProfile *os.File
		defer func() {
			if currentProfile != nil {
				pprof.StopCPUProfile()
				currentProfile.Close()
			}
		}()

		for {
			func() {
				const format = "2006-01-02T15_04_05.999"
				suffix := timeutil.Now().Add(cpuProfileInterval).Format(format)
				f, err := os.Create(filepath.Join(dir, cpuprof+suffix))
				if err != nil {
					log.Warningf(ctx, "error creating go cpu file %s", err)
					return
				}

				// Stop the current profile if it exists.
				if currentProfile != nil {
					pprof.StopCPUProfile()
					currentProfile.Close()
					currentProfile = nil
					gcProfiles(dir, cpuprof, maxSizePerProfile)
				}

				// Start the new profile.
				if err := pprof.StartCPUProfile(f); err != nil {
					log.Warningf(ctx, "unable to start cpu profile: %v", err)
					f.Close()
					return
				}
				currentProfile = f
			}()

			<-t.C
		}
	}()
}

func initBlockProfile() {
	// Enable the block profile for a sample of mutex and channel operations.
	// Smaller values provide more accurate profiles but are more
	// expensive. 0 and 1 are special: 0 disables the block profile and
	// 1 captures 100% of block events. For other values, the profiler
	// will sample one event per X nanoseconds spent blocking.
	//
	// The block profile can be viewed with `pprof http://HOST:PORT/debug/pprof/block`
	d := envutil.EnvOrDefaultInt64("COCKROACH_BLOCK_PROFILE_RATE",
		10000000 /* 1 sample per 10 milliseconds spent blocking */)
	runtime.SetBlockProfileRate(int(d))
}

func initMutexProfile() {
	// Enable the mutex profile for a fraction of mutex contention events.
	// Smaller values provide more accurate profiles but are more expensive. 0
	// and 1 are special: 0 disables the mutex profile and 1 captures 100% of
	// mutex contention events. For other values, the profiler will sample on
	// average 1/X events.
	//
	// The mutex profile can be viewed with `pprof http://HOST:PORT/debug/pprof/mutex`
	d := envutil.EnvOrDefaultInt("COCKROACH_MUTEX_PROFILE_RATE",
		1000 /* 1 sample per 1000 mutex contention events */)
	runtime.SetMutexProfileFraction(d)
}

var cacheSizeValue = newBytesOrPercentageValue(&serverCfg.CacheSize, memoryPercentResolver)
var sqlSizeValue = newBytesOrPercentageValue(&serverCfg.SQLMemoryPoolSize, memoryPercentResolver)
var diskTempStorageSizeValue = newBytesOrPercentageValue(nil /* v */, nil /* percentResolver */)

func initExternalIODir(ctx context.Context, firstStore base.StoreSpec) (string, error) {
	externalIODir := startCtx.externalIODir
	if externalIODir == "" && !firstStore.InMemory {
		externalIODir = filepath.Join(firstStore.Path, "extern")
	}
	if externalIODir == "" || externalIODir == "disabled" {
		return "", nil
	}
	if !filepath.IsAbs(externalIODir) {
		return "", errors.Errorf("%s path must be absolute", cliflags.ExternalIODir.Name)
	}
	return externalIODir, nil
}

func initTempStorageConfig(
	ctx context.Context,
	st *cluster.Settings,
	stopper *stop.Stopper,
	firstStore base.StoreSpec,
	specIdx int,
) (base.TempStorageConfig, error) {
	var recordPath string
	if !firstStore.InMemory {
		recordPath = filepath.Join(firstStore.Path, server.TempDirsRecordFilename)
	}

	var err error
	// Need to first clean up any abandoned temporary directories from
	// the temporary directory record file before creating any new
	// temporary directories in case the disk is completely full.
	if recordPath != "" {
		if err = engine.CleanupTempDirs(recordPath); err != nil {
			return base.TempStorageConfig{}, errors.Wrap(err, "could not cleanup temporary directories from record file")
		}
	}

	// The temp store size can depend on the location of the first regular store
	// (if it's expressed as a percentage), so we resolve that flag here.
	var tempStorePercentageResolver percentResolverFunc
	if !firstStore.InMemory {
		dir := firstStore.Path
		// Create the store dir, if it doesn't exist. The dir is required to exist
		// by diskPercentResolverFactory.
		if err = os.MkdirAll(dir, 0755); err != nil {
			return base.TempStorageConfig{}, errors.Wrapf(err, "failed to create dir for first store: %s", dir)
		}
		tempStorePercentageResolver, err = diskPercentResolverFactory(dir)
		if err != nil {
			return base.TempStorageConfig{}, errors.Wrapf(err, "failed to create resolver for: %s", dir)
		}
	} else {
		tempStorePercentageResolver = memoryPercentResolver
	}
	var tempStorageMaxSizeBytes int64
	if err = diskTempStorageSizeValue.Resolve(
		&tempStorageMaxSizeBytes, tempStorePercentageResolver,
	); err != nil {
		return base.TempStorageConfig{}, err
	}
	if !diskTempStorageSizeValue.IsSet() {
		// The default temp storage size is different when the temp
		// storage is in memory (which occurs when no temp directory
		// is specified and the first store is in memory).
		if startCtx.tempDir == "" && firstStore.InMemory {
			tempStorageMaxSizeBytes = base.DefaultInMemTempStorageMaxSizeBytes
		} else {
			tempStorageMaxSizeBytes = base.DefaultTempStorageMaxSizeBytes
		}
	}

	// Initialize a base.TempStorageConfig based on first store's spec and
	// cli flags.
	tempStorageConfig := base.TempStorageConfigFromEnv(
		ctx,
		st,
		firstStore,
		startCtx.tempDir,
		tempStorageMaxSizeBytes,
		specIdx,
	)

	// Set temp directory to first store's path if the temp storage is not
	// in memory.
	tempDir := startCtx.tempDir
	if tempDir == "" && !tempStorageConfig.InMemory {
		tempDir = firstStore.Path
	}
	// Create the temporary subdirectory for the temp engine.
	if tempStorageConfig.Path, err = engine.CreateTempDir(tempDir, server.TempDirPrefix, stopper); err != nil {
		return base.TempStorageConfig{}, errors.Wrap(err, "could not create temporary directory for temp storage")
	}

	// We record the new temporary directory in the record file (if it
	// exists) for cleanup in case the node crashes.
	if recordPath != "" {
		if err = engine.RecordTempDir(recordPath, tempStorageConfig.Path); err != nil {
			return base.TempStorageConfig{}, errors.Wrapf(
				err,
				"could not record temporary directory path to record file: %s",
				recordPath,
			)
		}
	}

	return tempStorageConfig, nil
}

// runStart starts the cockroach node using --store as the list of
// storage devices ("stores") on this machine and --join as the list
// of other active nodes used to join this node to the cockroach
// cluster, if this is its first time connecting.
func runStart(cmd *cobra.Command, args []string) error {
	_, err := gopsnet.IOCountersWithContext(context.TODO(), true)
	if err != nil {
		return err
	}
	return nil
}

func hintServerCmdFlags(ctx context.Context, cmd *cobra.Command) {
	pf := cmd.Flags()

	listenAddrSpecified := pf.Lookup(cliflags.ListenAddr.Name).Changed || pf.Lookup(cliflags.ServerHost.Name).Changed
	advAddrSpecified := pf.Lookup(cliflags.AdvertiseAddr.Name).Changed || pf.Lookup(cliflags.AdvertiseHost.Name).Changed

	if !listenAddrSpecified && !advAddrSpecified {
		host, _, _ := net.SplitHostPort(serverCfg.AdvertiseAddr)
		log.Shout(ctx, log.Severity_WARNING,
			"neither --listen-addr nor --advertise-addr was specified.\n"+
				"The server will advertise "+fmt.Sprintf("%q", host)+" to other nodes, is this routable?\n\n"+
				"Consider using:\n"+
				"- for local-only servers:  --listen-addr=localhost\n"+
				"- for multi-node clusters: --advertise-addr=<host/IP addr>\n")
	}
}

func clientFlags() string {
	flags := []string{os.Args[0], "<client cmd>"}
	host, port, err := net.SplitHostPort(serverCfg.AdvertiseAddr)
	if err == nil {
		flags = append(flags, "--host="+host+":"+port)
	}
	if startCtx.serverInsecure {
		flags = append(flags, "--insecure")
	} else {
		flags = append(flags, "--certs-dir="+startCtx.serverSSLCertsDir)
	}
	return strings.Join(flags, " ")
}

func reportConfiguration(ctx context.Context) {
	serverCfg.Report(ctx)
	if envVarsUsed := envutil.GetEnvVarsUsed(); len(envVarsUsed) > 0 {
		log.Infof(ctx, "using local environment variables: %s", strings.Join(envVarsUsed, ", "))
	}
	// If a user ever reports "bad things have happened", any
	// troubleshooting steps will want to rule out that the user was
	// running as root in a multi-user environment, or using different
	// uid/gid across runs in the same data directory. To determine
	// this, it's easier if the information appears in the log file.
	log.Infof(ctx, "process identity: %s", sysutil.ProcessIdentity())
}

func maybeWarnMemorySizes(ctx context.Context) {
	// Is the cache configuration OK?
	if !cacheSizeValue.IsSet() {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Using the default setting for --cache (%s).\n", cacheSizeValue)
		fmt.Fprintf(&buf, "  A significantly larger value is usually needed for good performance.\n")
		if size, err := status.GetTotalMemory(context.Background()); err == nil {
			fmt.Fprintf(&buf, "  If you have a dedicated server a reasonable setting is --cache=.25 (%s).",
				humanizeutil.IBytes(size/4))
		} else {
			fmt.Fprintf(&buf, "  If you have a dedicated server a reasonable setting is 25%% of physical memory.")
		}
		log.Warning(ctx, buf.String())
	}

	if !sqlSizeValue.IsSet() {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Using the default setting for --max-sql-memory (%s).\n", sqlSizeValue)
		fmt.Fprintf(&buf, "  A significantly larger value is usually needed in production.\n")
		if size, err := status.GetTotalMemory(context.Background()); err == nil {
			fmt.Fprintf(&buf, "  If you have a dedicated server a reasonable setting is --max-sql-memory=.25 (%s).",
				humanizeutil.IBytes(size/4))
		} else {
			fmt.Fprintf(&buf, "  If you have a dedicated server a reasonable setting is 25%% of physical memory.")
		}
		log.Warning(ctx, buf.String())
	}

	// Check that the total suggested "max" memory is well below the available memory.
	if maxMemory, err := status.GetTotalMemory(ctx); err == nil {
		requestedMem := serverCfg.CacheSize + serverCfg.SQLMemoryPoolSize
		maxRecommendedMem := int64(.75 * float64(maxMemory))
		if requestedMem > maxRecommendedMem {
			log.Shout(ctx, log.Severity_WARNING, fmt.Sprintf(
				"the sum of --max-sql-memory (%s) and --cache (%s) is larger than 75%% of total RAM (%s).\nThis server is running at increased risk of memory-related failures.",
				sqlSizeValue, cacheSizeValue, humanizeutil.IBytes(maxRecommendedMem)))
		}
	}
}

func logOutputDirectory() string {
	return startCtx.logDir.String()
}

// setupAndInitializeLoggingAndProfiling does what it says on the label.
// Prior to this however it determines suitable defaults for the
// logging output directory and the verbosity level of stderr logging.
// We only do this for the "start" command which is why this work
// occurs here and not in an OnInitialize function.
func setupAndInitializeLoggingAndProfiling(ctx context.Context) (*stop.Stopper, error) {
	// Default the log directory to the "logs" subdirectory of the first
	// non-memory store. If more than one non-memory stores is detected,
	// print a warning.
	ambiguousLogDirs := false
	if !startCtx.logDir.IsSet() && !startCtx.logDirFlag.Changed {
		// We only override the log directory if the user has not explicitly
		// disabled file logging using --log-dir="".
		newDir := ""
		for _, spec := range serverCfg.Stores.Specs {
			if spec.InMemory {
				continue
			}
			if newDir != "" {
				ambiguousLogDirs = true
				break
			}
			newDir = filepath.Join(spec.Path, "logs")
		}
		if err := startCtx.logDir.Set(newDir); err != nil {
			return nil, err
		}
	}

	if logDir := startCtx.logDir.String(); logDir != "" {
		ls := cockroachCmd.PersistentFlags().Lookup(logflags.LogToStderrName)
		if !ls.Changed {
			// Unless the settings were overridden by the user, silence
			// logging to stderr because the messages will go to a log file.
			if err := ls.Value.Set(log.Severity_NONE.String()); err != nil {
				return nil, err
			}
		}

		// Note that we configured the --log-dir flag to set
		// startContext.logDir. This is the point at which we set log-dir for the
		// util/log package. We don't want to set it earlier to avoid spuriously
		// creating a file in an incorrect log directory or if something is
		// accidentally logging after flag parsing but before the --background
		// dispatch has occurred.
		if err := flag.Lookup(logflags.LogDirName).Value.Set(logDir); err != nil {
			return nil, err
		}

		// Make sure the path exists.
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, err
		}
		log.Eventf(ctx, "created log directory %s", logDir)

		// Start the log file GC daemon to remove files that make the log
		// directory too large.
		log.StartGCDaemon()
	}

	outputDirectory := "."
	if p := logOutputDirectory(); p != "" {
		outputDirectory = p
	}

	if ambiguousLogDirs {
		// Note that we can't report this message earlier, because the log directory
		// may not have been ready before the call to MkdirAll() above.
		log.Shout(ctx, log.Severity_WARNING, "multiple stores configured"+
			" and --log-dir not specified, you may want to specify --log-dir to disambiguate.")
	}

	if auditLogDir := serverCfg.SQLAuditLogDirName.String(); auditLogDir != "" && auditLogDir != outputDirectory {
		// Make sure the path for the audit log exists, if it's a different path than
		// the main log.
		if err := os.MkdirAll(auditLogDir, 0755); err != nil {
			return nil, err
		}
		log.Eventf(ctx, "created SQL audit log directory %s", auditLogDir)
	}

	if startCtx.serverInsecure {
		// Use a non-annotated context here since the annotation just looks funny,
		// particularly to new users (made worse by it always printing as [n?]).
		addr := startCtx.serverListenAddr
		if addr == "" {
			addr = "<all your IP addresses>"
		}
		log.Shout(context.Background(), log.Severity_WARNING,
			"RUNNING IN INSECURE MODE!\n\n"+
				"- Your cluster is open for any client that can access "+addr+".\n"+
				"- Any user, even root, can log in without providing a password.\n"+
				"- Any user, connecting as root, can read or write any data in your cluster.\n"+
				"- There is no network encryption nor authentication, and thus no confidentiality.\n\n"+
				"Check out how to secure your cluster: "+base.DocsURL("secure-a-cluster.html"))
	}

	maybeWarnMemorySizes(ctx)

	// We log build information to stdout (for the short summary), but also
	// to stderr to coincide with the full logs.
	info := build.GetInfo()
	log.Infof(ctx, info.Short())

	initMemProfile(ctx, outputDirectory)
	initCPUProfile(ctx, outputDirectory)
	initBlockProfile()
	initMutexProfile()

	// Disable Stopper task tracking as performing that call site tracking is
	// moderately expensive (certainly outweighing the infrequent benefit it
	// provides).
	stopper := initBacktrace(outputDirectory)
	log.Event(ctx, "initialized profiles")

	return stopper, nil
}

func addrWithDefaultHost(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if host == "" {
		host = "localhost"
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return net.JoinHostPort(host, port), nil
}

// getClientGRPCConn returns a ClientConn, a Clock and a method that blocks
// until the connection (and its associated goroutines) have terminated.
func getClientGRPCConn(ctx context.Context) (*grpc.ClientConn, *hlc.Clock, func(), error) {
	if ctx.Done() == nil {
		return nil, nil, nil, errors.New("context must be cancellable")
	}
	// 0 to disable max offset checks; this RPC context is not a member of the
	// cluster, so there's no need to enforce that its max offset is the same
	// as that of nodes in the cluster.
	clock := hlc.NewClock(hlc.UnixNano, 0)
	stopper := stop.NewStopper()
	rpcContext := rpc.NewContext(
		log.AmbientContext{Tracer: serverCfg.Settings.Tracer},
		serverCfg.Config,
		clock,
		stopper,
		&serverCfg.Settings.Version,
	)
	addr, err := addrWithDefaultHost(serverCfg.AdvertiseAddr)
	if err != nil {
		stopper.Stop(ctx)
		return nil, nil, nil, err
	}
	conn, err := rpcContext.GRPCDial(addr).Connect(ctx)
	if err != nil {
		stopper.Stop(ctx)
		return nil, nil, nil, err
	}
	stopper.AddCloser(stop.CloserFn(func() {
		_ = conn.Close()
	}))

	// Tie the lifetime of the stopper to that of the context.
	closer := func() {
		stopper.Stop(ctx)
	}
	return conn, clock, closer, nil
}

// getAdminClient returns an AdminClient and a closure that must be invoked
// to free associated resources.
func getAdminClient(ctx context.Context) (serverpb.AdminClient, func(), error) {
	conn, _, finish, err := getClientGRPCConn(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to connect to the node")
	}
	return serverpb.NewAdminClient(conn), finish, nil
}

// quitCmd command shuts down the node server.
var quitCmd = &cobra.Command{
	Use:   "quit",
	Short: "drain and shutdown node\n",
	Long: `
Shutdown the server. The first stage is drain, where any new requests
will be ignored by the server. When all extant requests have been
completed, the server exits.
`,
	Args: cobra.NoArgs,
	RunE: MaybeDecorateGRPCError(runQuit),
}

// checkNodeRunning performs a no-op RPC and returns an error if it failed to
// connect to the server.
func checkNodeRunning(ctx context.Context, c serverpb.AdminClient) error {
	// Send a no-op Drain request.
	stream, err := c.Drain(ctx, &serverpb.DrainRequest{
		On:       nil,
		Shutdown: false,
	})
	if err != nil {
		return errors.Wrap(err, "Failed to connect to the node: error sending drain request")
	}
	// Ignore errors from the stream. We've managed to connect to the node above,
	// and that's all that this function is interested in.
	for {
		if _, err := stream.Recv(); err != nil {
			if server.IsWaitingForInit(err) {
				return err
			}
			if err != io.EOF {
				log.Warningf(ctx, "unexpected error from no-op Drain request: %s", err)
			}
			break
		}
	}
	return nil
}

// doShutdown attempts to trigger a server shutdown. When given an empty
// onModes slice, it's a hard shutdown.
//
// errTryHardShutdown is returned if the caller should do a hard-shutdown.
func doShutdown(ctx context.Context, c serverpb.AdminClient, onModes []int32) error {
	// We want to distinguish between the case in which we can't even connect to
	// the server (in which case we don't want our caller to try to come back with
	// a hard retry) and the case in which an attempt to shut down fails (times
	// out, or perhaps drops the connection while waiting). To that end, we first
	// run a noop DrainRequest. If that fails, we give up.
	if err := checkNodeRunning(ctx, c); err != nil {
		if server.IsWaitingForInit(err) {
			return fmt.Errorf("node cannot be shut down before it has been initialized")
		}
		if grpcutil.IsClosedConnection(err) {
			return nil
		}
		return err
	}
	// Send a drain request and continue reading until the connection drops (which
	// then counts as a success, for the connection dropping is likely the result
	// of the Stopper having reached the final stages of shutdown).
	stream, err := c.Drain(ctx, &serverpb.DrainRequest{
		On:       onModes,
		Shutdown: true,
	})
	if err != nil {
		//  This most likely means that we shut down successfully. Note that
		//  sometimes the connection can be shut down even before a DrainResponse gets
		//  sent back to us, so we don't require a response on the stream (see
		//  #14184).
		if grpcutil.IsClosedConnection(err) {
			return nil
		}
		return errors.Wrap(err, "Error sending drain request")
	}
	for {
		if _, err := stream.Recv(); err != nil {
			if grpcutil.IsClosedConnection(err) {
				return nil
			}
			// Unexpected error; the caller should try again (and harder).
			return errTryHardShutdown{err}
		}
	}
}

type errTryHardShutdown struct{ error }

// runQuit accesses the quit shutdown path.
func runQuit(cmd *cobra.Command, args []string) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		if err == nil {
			fmt.Println("ok")
		}
	}()
	onModes := make([]int32, len(server.GracefulDrainModes))
	for i, m := range server.GracefulDrainModes {
		onModes[i] = int32(m)
	}

	c, finish, err := getAdminClient(ctx)
	if err != nil {
		return err
	}
	defer finish()

	if quitCtx.serverDecommission {
		var myself []string // will remain empty, which means target yourself
		if err := runDecommissionNodeImpl(ctx, c, nodeDecommissionWaitAll, myself); err != nil {
			return err
		}
	}
	errChan := make(chan error, 1)
	go func() {
		errChan <- doShutdown(ctx, c, onModes)
	}()
	select {
	case err := <-errChan:
		if err != nil {
			if _, ok := err.(errTryHardShutdown); ok {
				log.Warningf(ctx, "graceful shutdown failed: %s; proceeding with hard shutdown\n", err)
				break
			}
			return err
		}
		return nil
	case <-time.After(time.Minute):
		log.Warningf(ctx, "timed out; proceeding with hard shutdown")
	}
	// Not passing drain modes tells the server to not bother and go
	// straight to shutdown. We try two times just in case there is a transient error.
	err = doShutdown(ctx, c, nil)
	if err != nil {
		log.Warningf(ctx, "hard shutdown attempt failed, retrying: %v", err)
		err = doShutdown(ctx, c, nil)
	}
	return errors.Wrap(doShutdown(ctx, c, nil), "hard shutdown failed")
}
