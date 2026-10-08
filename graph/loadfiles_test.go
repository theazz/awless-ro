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

package graph

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

// A missing file is a real error, and it has to stay distinguishable from any other
// read failure: the loader maps a not-exist error to an empty answer, and it can only
// do that if NewGraphFromFiles wraps the error with %w. The returned graph is non-nil
// so a caller that ignores the error cannot nil-deref.
func TestNewGraphFromFilesMissingFileIsNotExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.nt")

	g, err := NewGraphFromFiles(missing)
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error should satisfy fs.ErrNotExist, got %q", err)
	}
	if g == nil {
		t.Error("expected a non-nil graph alongside the error")
	}
}
