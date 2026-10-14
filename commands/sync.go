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
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/graph"
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
		failed, otherErrs := splitSyncErrors(syncErr)

		out := syncOutput{Profile: config.GetAWSProfile(), Region: config.GetAWSRegion(), Services: []syncedService{}, Errors: otherErrs}
		// A failed service may have no graph at all; it is listed all the same.
		if graphs == nil {
			graphs = map[string]cloud.GraphAPI{}
		}
		for k := range failed {
			if _, ok := graphs[k]; !ok {
				graphs[k] = graph.NewGraph()
			}
		}
		for k, g := range graphs {
			stats := syncStats(k, g)
			stats.Error = failed[k]
			out.Services = append(out.Services, stats)
			if stats.Error != "" {
				// The README promises a service that could not be read is reported.
				// It used to be said only with -v, so expired credentials looked like
				// an empty account.
				logger.Warningf("-> %s: not synced, or only in part: %s", k, stats.Error)
				continue
			}
			logger.Infof("-> %s", stats)
		}
		for _, e := range otherErrs {
			logger.Warning(e)
		}
		logger.Infof("sync took %s", time.Since(start))

		if jsonOutput() {
			sort.Slice(out.Services, func(i, j int) bool { return out.Services[i].Name < out.Services[j].Name })
			printJSON(out)
		}
		// One service refused is an account with limited permissions, and the rest is
		// still worth having; nothing synced at all is a failure.
		if len(failed) > 0 && len(failed) == len(graphs) {
			exitOn(errors.New("nothing could be synced"))
		}
		return nil
	},
}

// splitSyncErrors sorts what Sync reported into the services that failed, by name,
// and everything else.
func splitSyncErrors(err error) (map[string]string, []string) {
	failed, other := map[string]string{}, []string{}
	var all sync.Errors
	if !errors.As(err, &all) {
		if err != nil {
			other = append(other, err.Error())
		}
		return failed, other
	}
	for _, e := range all {
		var se *sync.ServiceError
		if errors.As(e, &se) {
			failed[se.Service] = se.Err.Error()
		} else {
			other = append(other, e.Error())
		}
	}
	return failed, other
}

// syncOutput is `sync --format json`: what was synced, per service. The progress and
// the timing stay on stderr, as in text.
type syncOutput struct {
	Profile  string          `json:"profile"`
	Region   string          `json:"region"`
	Services []syncedService `json:"services"`
	// Failures not tied to one service, such as a local file that could not be
	// written.
	Errors []string `json:"errors"`
}

type syncedService struct {
	Name string `json:"name"`
	// The number of resources of each type, by type name (as in show's "type").
	Resources map[string]int `json:"resources"`
	// The types configuration left out, with the key that did. They are not in
	// Resources: a count of zero would say the account has none.
	Off map[string]string `json:"off"`
	// Why the service could not be fetched, or not all of it. Its counts are then
	// what was read, not what the account holds.
	Error string `json:"error,omitempty"`
}

// String is the line sync logs for the service, in type order.
func (s syncedService) String() string {
	types := make([]string, 0, len(s.Resources)+len(s.Off))
	for rt := range s.Resources {
		types = append(types, rt)
	}
	for rt := range s.Off {
		types = append(types, rt)
	}
	sort.Strings(types)

	strs := make([]string, 0, len(types))
	for _, rt := range types {
		if key, off := s.Off[rt]; off {
			strs = append(strs, fmt.Sprintf("%s: off (%s)", cloud.PluralizeResource(rt), key))
			continue
		}
		n := s.Resources[rt]
		if n > 1 {
			rt = cloud.PluralizeResource(rt)
		}
		strs = append(strs, fmt.Sprintf("%d %s", n, rt))
	}
	return fmt.Sprintf("%s: %s", s.Name, strings.Join(strs, ", "))
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

func syncStats(serviceName string, g cloud.GraphAPI) syncedService {
	stats := syncedService{Name: serviceName, Resources: map[string]int{}, Off: map[string]string{}}
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
			stats.Off[rt] = key
			continue
		}

		res, err := g.Find(cloud.NewQuery(rt))
		if err != nil {
			continue
		}
		stats.Resources[rt] = len(res)
	}
	return stats
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
