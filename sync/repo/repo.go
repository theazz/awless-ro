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

// Package repo locates and prepares the on-disk directory holding the locally
// synced graph.
//
// Upstream awless additionally kept this directory under git and committed the
// N-Triples files after every sync, which powered the hidden `awless history`
// command. That was dropped: the go-git v4 dependency it relied on is
// abandoned, and versioning the snapshots is a storage-retention design of its
// own rather than part of syncing. The graph files themselves are unchanged.
package repo

import (
	"os"
	"path/filepath"
)

// Repo gives access to the directory the synced graph lives in.
type Repo interface {
	BaseDir() string
}

// NullRepo is a Repo that points nowhere, used by the no-op syncer.
type NullRepo struct{}

func (NullRepo) BaseDir() string { return "" }

type dirRepo struct {
	basedir string
}

func (r *dirRepo) BaseDir() string { return r.basedir }

// BaseDir returns the directory the synced N-Triples files are stored in.
func BaseDir() string {
	return filepath.Join(os.Getenv("__AWLESS_HOME"), "aws", "rdf")
}

// New creates the storage directory if needed and returns a Repo for it.
func New() (Repo, error) {
	dir := BaseDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &dirRepo{basedir: dir}, nil
}
