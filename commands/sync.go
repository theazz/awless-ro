/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package commands

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/logger"
	"github.com/theazz/awless-ro/sync"
)

var (
	servicesToSyncFlags map[string]*bool
	profileSyncFlag     bool
)

func init() {
	RootCmd.AddCommand(syncCmd)
	syncCmd.Flags().BoolVar(&profileSyncFlag, "profile-sync", false, "Will dump a cpu and mem profiling file")

	servicesToSyncFlags = make(map[string]*bool)
	for _, service := range awsservices.ServiceNames {
		servicesToSyncFlags[service] = new(bool)
		syncCmd.Flags().BoolVar(servicesToSyncFlags[service], service, false, fmt.Sprintf("Sync '%s' service only", service))
	}
}

var syncCmd = &cobra.Command{
	Use:               "sync",
	ValidArgsFunction: cobra.NoFileCompletions,
	Short:             "Manual sync of remote resources to the local store (ex: when autosync is unset)",
	PersistentPreRun:  applyHooks(initLoggerHook, initAwlessEnvHook, initCloudServicesHook, initSyncerHook, firstInstallDoneHook),
	PersistentPostRun: applyHooks(onVersionUpgrade, networkMonitorHook),

	RunE: func(cmd *cobra.Command, args []string) error {
		var services []cloud.Service
		displayAllServices := true
		for _, srv := range cloud.ServiceRegistry {
			if *servicesToSyncFlags[srv.Name()] {
				displayAllServices = false
			}
		}
		for _, srv := range cloud.ServiceRegistry {
			if displayAllServices || *servicesToSyncFlags[srv.Name()] {
				services = append(services, srv)
			}
		}
		logger.Infof("running sync for region '%s'", config.GetAWSRegion())

		var syncErr error
		var graphs map[string]cloud.GraphAPI
		syncFn := func() {
			graphs, syncErr = sync.DefaultSyncer.Sync(services...)
		}

		start := time.Now()
		if profileSyncFlag {
			withProfiling(syncFn)
		} else {
			syncFn()
		}
		if syncErr != nil {
			logger.Verbose(syncErr)
		}

		for k, g := range graphs {
			displaySyncStats(k, g)
		}
		logger.Infof("sync took %s", time.Since(start))

		return nil
	},
}

func withProfiling(fn func()) {
	logger.Infof("sync profiling on")
	mem, err := os.Create("mem-sync.prof")
	if err != nil {
		log.Fatal("could not create mem profile: ", err)
	}
	logger.Infof("running garbage collection before profiling")
	runtime.GC() // cleaned up memeory before running function
	defer mem.Close()

	cpu, err := os.Create("cpu-sync.prof")
	if err != nil {
		log.Fatal("could not create cpu profile: ", err)
	}
	if err := pprof.StartCPUProfile(cpu); err != nil {
		log.Fatal("could not start cpu profile: ", err)
	}

	fn()

	pprof.StopCPUProfile()
	if err := pprof.WriteHeapProfile(mem); err != nil {
		log.Fatal("could not write memory profile: ", err)
	}
	logger.Infof("Generated profiling files %s and %s", cpu.Name(), mem.Name())
}

func displaySyncStats(serviceName string, g cloud.GraphAPI) {
	var strs []string
	for rt, service := range awsservices.ServicePerResourceType {
		if service != serviceName {
			continue
		}

		// A resource type nobody looked at reported "0", which reads as an answer
		// about the account: `-> dns: 2 zones, 0 record` against an account holding
		// 84 records. Two types are off by default because they cost a call per
		// parent — one per hosted zone, one per bucket — so this is the common case,
		// not an edge one.
		if key, off := syncDisabledFor(serviceName, rt); off {
			strs = append(strs, fmt.Sprintf("%s: off (%s)", cloud.PluralizeResource(rt), key))
			continue
		}

		res, err := g.Find(cloud.NewQuery(rt))
		if err != nil {
			continue
		}
		nbRes := len(res)
		if nbRes > 1 {
			strs = append(strs, fmt.Sprintf("%d %s", nbRes, cloud.PluralizeResource(rt)))
		} else {
			strs = append(strs, fmt.Sprintf("%d %s", nbRes, rt))
		}
	}
	logger.Infof("-> %s: %s", serviceName, strings.Join(strs, ", "))
}

// syncDisabledFor reports whether a resource type was skipped by configuration, and
// under which key, so that the message can name the setting to change.
//
// The fetchers read these keys themselves; this only has to agree with them about the
// name, which is why the shape is spelled out rather than guessed at.
func syncDisabledFor(serviceName, resourceType string) (string, bool) {
	// The service key is checked first because that is the order the services
	// themselves apply: IsSyncDisabled short-circuits Fetch into an empty graph
	// without ever looking at the per-type key. Reporting the per-type key here would
	// name a setting that is not the one having the effect.
	for _, key := range []string{
		fmt.Sprintf("aws.%s.sync", serviceName),
		fmt.Sprintf("aws.%s.%s.sync", serviceName, resourceType),
	} {
		v, ok := config.Get(key)
		if !ok {
			continue
		}
		if enabled, isBool := v.(bool); isBool && !enabled {
			return key, true
		}
	}
	return "", false
}
