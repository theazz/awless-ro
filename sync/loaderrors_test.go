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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/cloud"
)

// A `--local` listing that cannot read its own synced file used to report "No results
// found." and exit 0 — the same answer as a genuinely empty account. These pin the
// fix: a missing file stays an empty answer, but an unreadable one is an error the
// caller can fail closed on. All offline, no credentials, no network.

const (
	goodInfraNT = "<i-1> <cloud:id> \"i-1\" .\n" +
		"<i-1> <cloud:name> \"web\" .\n" +
		"<i-1> <rdf:type> <cloud-owl:Instance> .\n"

	brokenAccessNT = "<pol-1> <cloud:id> \"pol-1\" .\n" +
		"THIS IS NOT N-TRIPLES\n" +
		"<pol-1> <rdf:type> <cloud-owl:Policy> .\n"
)

// writeNT writes arbitrary bytes to the synced path for a service, creating the
// directory. Unlike writeGraph in nothingsynced_test.go it does not insist on valid
// content, so it can plant a broken or oversized file.
func writeNT(t *testing.T, home, profile, regionDir, service, content string) {
	t.Helper()
	dir := filepath.Join(home, "aws", "rdf", profile, regionDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, service+fileExt), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// A service that was never synced is a legitimate empty answer, not an error.
func TestLoadMissingServiceFileIsEmptyNotAnError(t *testing.T) {
	t.Setenv("__AWLESS_HOME", t.TempDir())

	g, err := LoadLocalGraphForService("infra", "default", "eu-west-1")
	if err != nil {
		t.Fatalf("a never-synced service should load empty, got error: %s", err)
	}
	if g == nil {
		t.Fatal("expected a non-nil empty graph")
	}
	assertNoResources(t, g, "instance")
}

// A well-formed but empty graph still answers empty, with no error. A genuine empty
// result must stay a genuine empty result.
func TestLoadEmptyGraphFileIsEmptyNotAnError(t *testing.T) {
	for _, tc := range []struct {
		name, content string
	}{
		{"zero bytes", ""},
		{"comments and blank lines only", "# nothing here\n\n   \n# just a comment\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("__AWLESS_HOME", home)
			writeNT(t, home, "default", "eu-west-1", "infra", tc.content)

			g, err := LoadLocalGraphForService("infra", "default", "eu-west-1")
			if err != nil {
				t.Fatalf("an empty graph should load without error, got: %s", err)
			}
			assertNoResources(t, g, "instance")
		})
	}
}

// A file that exists but will not parse is a hard error naming the file and the line.
func TestLoadMalformedFileIsAnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("__AWLESS_HOME", home)
	writeNT(t, home, "default", "global", "access", brokenAccessNT)

	g, err := LoadLocalGraphForService("access", "default", "eu-west-1")
	if err == nil {
		t.Fatal("a malformed file must be an error, not an empty answer")
	}
	if !strings.Contains(err.Error(), "access.nt") {
		t.Errorf("error should name access.nt, got %q", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should name line 2, got %q", err)
	}
	if g == nil {
		t.Error("expected a non-nil graph alongside the error")
	}
}

// The multi-service rule: one unreadable file voids exactly the answers read out of
// it, no more and no less. A broken access.nt must not void a good infra.nt.
func TestLoadMultiServiceRule(t *testing.T) {
	home := t.TempDir()
	t.Setenv("__AWLESS_HOME", home)
	writeNT(t, home, "default", "eu-west-1", "infra", goodInfraNT)
	writeNT(t, home, "default", "global", "access", brokenAccessNT)

	// infra reads only its own file and must succeed.
	infra, err := LoadLocalGraphForService("infra", "default", "eu-west-1")
	if err != nil {
		t.Fatalf("a good infra.nt must load despite a broken sibling, got: %s", err)
	}
	assertHasResource(t, infra, "instance", "i-1")

	// access reads the broken file and must fail.
	if _, err := LoadLocalGraphForService("access", "default", "eu-west-1"); err == nil {
		t.Error("a broken access.nt must fail a listing that reads it")
	}

	// A command that reads every file (show, inspect) fails and names the broken one.
	if _, err := LoadLocalGraphs("default", "eu-west-1"); err == nil {
		t.Error("LoadLocalGraphs must fail when any file is broken")
	} else if !strings.Contains(err.Error(), "access.nt") {
		t.Errorf("LoadLocalGraphs error should name access.nt, got %q", err)
	}
}

// Part 2 at the loader level: a document literal past the old 8 MiB ceiling now loads,
// and the literal comes back at full length.
func TestLoadHugeLiteral(t *testing.T) {
	const docLen = 9 << 20 // past the old 8 MiB cap
	doc := strings.Repeat("x", docLen)
	hugeAccessNT := "<pol-huge> <cloud:id> \"pol-huge\" .\n" +
		"<pol-huge> <cloud:name> \"huge-policy\" .\n" +
		"<pol-huge> <cloud:document> \"" + doc + "\" .\n" +
		"<pol-huge> <rdf:type> <cloud-owl:Policy> .\n"

	home := t.TempDir()
	t.Setenv("__AWLESS_HOME", home)
	writeNT(t, home, "default", "global", "access", hugeAccessNT)

	g, err := LoadLocalGraphForService("access", "default", "eu-west-1")
	if err != nil {
		t.Fatalf("a huge literal should load, got: %s", err)
	}
	assertHasResource(t, g, "policy", "pol-huge")

	// Re-marshalling proves the literal survived the load at full length.
	var buf bytes.Buffer
	if err := g.MarshalTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), doc) {
		t.Errorf("the %d-byte document literal did not come back intact", docLen)
	}
}

func assertNoResources(t *testing.T, g cloud.GraphAPI, resourceType string) {
	t.Helper()
	res, err := g.Find(cloud.NewQuery(resourceType))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("expected no %s resources, got %d", resourceType, len(res))
	}
}
