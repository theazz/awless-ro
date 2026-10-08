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

package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/logger"
	"github.com/theazz/awless-ro/sync/repo"
)

const fileExt = ".nt"

var DefaultSyncer Syncer

type Syncer interface {
	repo.Repo
	Sync(...cloud.Service) (map[string]cloud.GraphAPI, error)
}

type noopsyncer struct {
	repo.NullRepo
}

func NoOpSyncer() Syncer { return new(noopsyncer) }

func (s *noopsyncer) Sync(services ...cloud.Service) (map[string]cloud.GraphAPI, error) {
	return map[string]cloud.GraphAPI{}, nil
}

type syncer struct {
	repo.Repo
	logger *logger.Logger
}

func NewSyncer(l ...*logger.Logger) Syncer {
	repo, err := repo.New()
	if err != nil {
		panic(err)
	}

	s := &syncer{Repo: repo}

	if len(l) > 0 {
		s.logger = l[0]
	} else {
		s.logger = logger.DiscardLogger
	}

	return s
}

func (s *syncer) Sync(services ...cloud.Service) (map[string]cloud.GraphAPI, error) {
	var workers gosync.WaitGroup

	type result struct {
		service cloud.Service
		gph     cloud.GraphAPI
		start   time.Time
		err     error
	}

	resultc := make(chan *result, len(services))

	for _, service := range services {
		if service.IsSyncDisabled() {
			s.logger.Verbosef("sync: *disabled* for service %s", service.Name())
			continue
		}
		workers.Add(1)
		go func(srv cloud.Service) {
			defer workers.Done()
			start := time.Now()
			g, err := srv.Fetch(context.Background())
			resultc <- &result{service: srv, gph: g, start: start, err: err}
		}(service)
	}

	go func() {
		workers.Wait()
		close(resultc)
	}()

	var allErrors []error
	graphs := make(map[string]cloud.GraphAPI)
	servicesByName := make(map[string]cloud.Service)
Loop:
	for {
		select {
		case res, ok := <-resultc:
			if !ok {
				break Loop
			}
			if res.err != nil {
				allErrors = append(allErrors, fmt.Errorf("syncing %s: %s", res.service.Name(), res.err))
			} else {
				s.logger.ExtraVerbosef("sync: fetched %s service took %s", res.service.Name(), time.Since(res.start))
			}
			if serv := res.service; serv != nil {
				servicesByName[serv.Name()] = serv
				if res.gph != nil {
					graphs[serv.Name()] = res.gph
				}
			}
		}
	}

	for name, g := range graphs {
		serviceRegion := servicesByName[name].Region()
		serviceProfile := servicesByName[name].Profile()
		serviceDir := filepath.Join(s.BaseDir(), serviceProfile, serviceRegion)
		// 0700 because the files below describe the account. The error used to be
		// dropped, which turned a permissions problem into a confusing failure to
		// open a file in a directory that was never there.
		if err := os.MkdirAll(serviceDir, 0700); err != nil {
			allErrors = append(allErrors, fmt.Errorf("creating %s: %s", serviceDir, err))
			continue
		}

		fullpath := filepath.Join(serviceDir, fmt.Sprintf("%s%s", name, fileExt))
		f, err := os.OpenFile(fullpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("opening %s: %s", fullpath, err))
			continue
		}
		closeFile := func() {
			if err := f.Close(); err != nil {
				allErrors = append(allErrors, fmt.Errorf("closing file %s: %s", fullpath, err))
			}
		}
		if err := g.MarshalTo(f); err != nil {
			allErrors = append(allErrors, fmt.Errorf("marshal to %s: %s", fullpath, err))
			closeFile()
			continue
		}
		closeFile()
	}

	return graphs, concatErrors(allErrors)
}

func concatErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	lines := []string{"syncing errors:"}
	for _, err := range errs {
		lines = append(lines, fmt.Sprintf("\t\t%s", err))
	}

	return errors.New(strings.Join(lines, "\n"))
}

// LoadLocalGraphForService reads the one synced file a `list --local` of this
// service's resources depends on.
//
// A file that was never synced is a legitimate empty answer, not an error, so a
// not-exist error maps to an empty graph and a nil error. Any other failure — the
// file exists but cannot be opened or parsed — is returned so the caller can fail
// closed instead of presenting unread data as "No results found.". The returned
// graph is always non-nil so a caller that ignores the error cannot nil-deref.
func LoadLocalGraphForService(serviceName, profile, region string) (cloud.GraphAPI, error) {
	regionDir := region
	if serviceName == "access" || serviceName == "dns" || serviceName == "cdn" {
		regionDir = "global"
	}
	path := filepath.Join(repo.BaseDir(), profile, regionDir, fmt.Sprintf("%s%s", serviceName, fileExt))
	g, err := graph.NewGraphFromFiles(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return graph.NewGraph(), nil
		}
		return g, err
	}
	return g, nil
}

func LoadLocalGraphs(profile, region string) (cloud.GraphAPI, error) {
	var files []string
	globalFiles, _ := filepath.Glob(filepath.Join(repo.BaseDir(), profile, "global", fmt.Sprintf("*%s", fileExt)))
	regionFiles, _ := filepath.Glob(filepath.Join(repo.BaseDir(), profile, region, fmt.Sprintf("*%s", fileExt)))

	files = append(files, globalFiles...)
	files = append(files, regionFiles...)

	return graph.NewGraphFromFiles(files...)
}

// NothingSyncedFor reports that this profile and region have no synced files at all.
//
// It exists so that `--local` can tell "the account holds none of these" apart from
// "nothing has been synced yet". Both produced "No results found.", which is an answer
// about the account in a situation where the account was never read — and since the
// first run no longer syncs, having synced nothing is the normal starting state rather
// than an unusual one.
func NothingSyncedFor(profile, region string) bool {
	for _, dir := range []string{"global", region} {
		files, _ := filepath.Glob(filepath.Join(repo.BaseDir(), profile, dir, fmt.Sprintf("*%s", fileExt)))
		if len(files) > 0 {
			return false
		}
	}
	return true
}

func LoadAllLocalGraphs(profile string) (cloud.GraphAPI, error) {
	path := filepath.Join(repo.BaseDir(), profile, "*", fmt.Sprintf("*%s", fileExt))
	files, _ := filepath.Glob(path)

	return graph.NewGraphFromFiles(files...)
}
