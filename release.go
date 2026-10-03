//go:build ignore
// +build ignore

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

package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	releaseTag = flag.String("tag", "", "Git tag to be released")
	brew       = flag.Bool("brew", false, "Brew build (disable zipping and build only for specify os and arch)")
	buildOS    = flag.String("os", runtime.GOOS, "The OS to build")
	buildArch  = flag.String("arch", runtime.GOARCH, "The ARCH to build")
)

// 386 was dropped and arm64 added: Apple Silicon and Graviton are the platforms
// that matter now, and 32-bit x86 has not been a realistic target for years.
var builds = map[string][]string{
	"darwin":  {"amd64", "arm64"},
	"linux":   {"amd64", "arm64"},
	"windows": {"amd64"},
}

func main() {
	flag.Parse()

	allBuild := map[string][]string{
		*buildOS: {*buildArch},
	}

	if *releaseTag != "" && !*brew {
		// A release is built from the working tree, while the commit baked into the
		// binary comes from the tag. If the tree has uncommitted changes those two
		// are different things, and `awless-ro version` then names a commit that does
		// not contain the code it is running. Nothing downstream can detect that:
		// the checksums would be self-consistent and wrong.
		if err := refuseDirtyTree(); err != nil {
			printKo("%s", err)
			os.Exit(1)
		}
		allBuild = builds
		printInfo("RELEASING")
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		artefacts []string
		failed    bool
	)

	for osname, archs := range allBuild {
		for _, arch := range archs {
			wg.Add(1)
			go func(o, a string) {
				defer wg.Done()
				artefact, err := buildAndZip(o, a)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err)
					failed = true
					return
				}
				if artefact != "" {
					artefacts = append(artefacts, artefact)
				}
			}(osname, arch)
		}
	}

	wg.Wait()

	// A build failure used to be printed and then forgotten, so this exited zero
	// having produced a partial release. Whoever publishes it would have had to
	// notice by reading the log.
	if failed {
		printKo("some builds failed; nothing was checksummed")
		os.Exit(1)
	}

	if len(artefacts) > 1 {
		if err := writeChecksums(artefacts); err != nil {
			printKo("%s", err)
			os.Exit(1)
		}
	}
}

// refuseDirtyTree reports uncommitted or untracked changes, naming them, so that a
// release cannot be cut from a tree that differs from the commit it claims.
func refuseDirtyTree() error {
	out, err := runCmd(nil, "git", "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("cannot determine whether the tree is clean: %s", err)
	}

	var dirty []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		// The release artefacts themselves are written into the working directory,
		// so they are not evidence of a modified tree.
		if line == "" || isReleaseArtefact(line) {
			continue
		}
		dirty = append(dirty, line)
	}

	if len(dirty) > 0 {
		return fmt.Errorf("the working tree has uncommitted changes, so the commit recorded in the "+
			"binary would not be the code in it:\n\t%s\ncommit or stash them first",
			strings.Join(dirty, "\n\t"))
	}
	return nil
}

func isReleaseArtefact(statusLine string) bool {
	for _, suffix := range []string{".tar.gz", ".zip", "/" + checksumFile, " " + checksumFile} {
		if strings.HasSuffix(statusLine, suffix) {
			return true
		}
	}
	return strings.HasSuffix(statusLine, checksumFile)
}

const checksumFile = "SHA256SUMS"

// writeChecksums records a digest per artefact, so that whoever downloads one has
// something to check it against. Without this a user has no way to tell a tampered or
// truncated download from a good one.
//
// The layout is the one coreutils writes, digest and two spaces and name, so that
// `sha256sum -c SHA256SUMS` and `shasum -a 256 -c SHA256SUMS` both work as-is. Names
// are sorted, because the build order comes from goroutines and a file whose lines
// shuffle between runs is one nobody can diff.
func writeChecksums(artefacts []string) error {
	sort.Strings(artefacts)

	var out strings.Builder
	for _, name := range artefacts {
		f, err := os.Open(name)
		if err != nil {
			return fmt.Errorf("checksumming %s: %s", name, err)
		}
		digest := sha256.New()
		if _, err := io.Copy(digest, f); err != nil {
			f.Close()
			return fmt.Errorf("checksumming %s: %s", name, err)
		}
		f.Close()
		fmt.Fprintf(&out, "%x  %s\n", digest.Sum(nil), name)
	}

	if err := os.WriteFile(checksumFile, []byte(out.String()), 0644); err != nil {
		return fmt.Errorf("writing %s: %s", checksumFile, err)
	}

	printOk("wrote %s for %d artefacts", checksumFile, len(artefacts))
	fmt.Printf("    verify with: sha256sum -c %s   (macOS: shasum -a 256 -c %s)\n", checksumFile, checksumFile)
	return nil
}

// buildAndZip builds one platform and packages it, returning the name of the file it
// produced so that main can checksum it. A brew build produces a bare binary and
// returns no name, because there is no archive to publish.
func buildAndZip(osname, arch string) (string, error) {
	// Added to the environment rather than replacing it. Setting cmd.Env to just
	// these three wiped everything else, including the module cache location, so the
	// build only worked on a machine where GOPATH happened to be set — and on any
	// other it failed with "module cache not found".
	env := append(os.Environ(),
		fmt.Sprintf("GOARCH=%s", arch),
		fmt.Sprintf("GOOS=%s", osname),
	)

	builddir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(builddir)

	printInfo("Building artefact for %s %s", osname, arch)

	var binName string

	switch osname {
	case "windows":
		binName = "awless-ro.exe"
	default:
		binName = "awless-ro"
	}

	artefactPath := filepath.Join(builddir, binName)

	gitRef := "refs/heads/master"
	if *releaseTag != "" {
		if tag, _ := runCmd(nil, "git", "describe", "--exact-match", "--tags"); strings.TrimSpace(tag) != *releaseTag {
			return "", fmt.Errorf("the git repository is not at tag '%s'", *releaseTag)
		}
		gitRef = fmt.Sprintf("refs/tags/%s", *releaseTag)
	}

	// The commit, resolved through the ref rather than read off it. `show-ref -s` on
	// an annotated tag returns the tag object's sha, not the commit's, so
	// `awless-ro version` reported an id that matches no commit in the repository —
	// which is the one thing that field is for. ^{commit} peels the tag; for a branch
	// or a lightweight tag it changes nothing.
	sha, err := runCmd(nil, "git", "rev-parse", gitRef+"^{commit}")
	if err != nil {
		return "", err
	}

	buildFor := "targz"
	if *brew {
		buildFor = "brew"
	} else if osname == "windows" {
		buildFor = "zip"
	}

	buildInfo := fmt.Sprintf("-X github.com/theazz/awless-ro/config.buildDate=%s -X github.com/theazz/awless-ro/config.buildSha=%s -X github.com/theazz/awless-ro/config.buildOS=%s -X github.com/theazz/awless-ro/config.buildArch=%s -X github.com/theazz/awless-ro/config.BuildFor=%s",
		time.Now().Format(time.RFC3339),
		strings.TrimSpace(sha),
		osname,
		arch,
		buildFor,
	)

	ldflags := fmt.Sprintf("-ldflags=-s -w %s", buildInfo)

	// -trimpath: without it every source file's absolute path on the build machine
	// (the checkout and the module cache under the builder's home directory) is
	// embedded in the binary, which leaks the builder's username and layout and
	// makes two builds of the same commit differ by where they were run.
	if _, err := runCmd(env, "go", "build", "-trimpath", "-o", artefactPath, ldflags); err != nil {
		return "", err
	}

	archiveName := fmt.Sprintf("%s-%s-%s", strings.Split(binName, ".")[0], osname, arch)

	switch buildFor {
	case "brew": //No zipping
		fmt.Println("DO NOT forget to update the brew formula.")
		return "", os.Rename(artefactPath, "awless-ro")
	case "zip":
		name := archiveName + ".zip"
		if err := writeZip(name, binName, artefactPath); err != nil {
			return "", err
		}
		return name, nil
	case "targz":
		name := archiveName + ".tar.gz"
		if err := writeTarGz(name, artefactPath); err != nil {
			return "", err
		}
		return name, nil
	default:
		return "", errors.New("missing packaging method")
	}
}

// Both packagers close everything explicitly and report the error from doing so. A
// close that fails on an archive writer means the archive is truncated, and the
// previous code both leaked the zip file's descriptor and returned before the
// deferred closes could say anything went wrong — which matters now that the file is
// hashed straight afterwards.
func writeZip(name, entryName, binaryPath string) error {
	out, err := os.OpenFile(name, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	w := zip.NewWriter(out)

	entry, err := w.Create(entryName)
	if err != nil {
		out.Close()
		return err
	}

	binary, err := os.Open(binaryPath)
	if err != nil {
		out.Close()
		return err
	}
	if _, err := io.Copy(entry, binary); err != nil {
		binary.Close()
		out.Close()
		return err
	}
	binary.Close()

	if err := w.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writeTarGz(name, binaryPath string) error {
	out, err := os.Create(name)
	if err != nil {
		return err
	}

	gw := gzip.NewWriter(out)
	tw := tar.NewWriter(gw)

	binary, err := os.Open(binaryPath)
	if err != nil {
		out.Close()
		return err
	}
	defer binary.Close()

	stat, err := binary.Stat()
	if err != nil {
		out.Close()
		return err
	}
	header, err := tar.FileInfoHeader(stat, "")
	if err != nil {
		out.Close()
		return err
	}
	if err := tw.WriteHeader(header); err != nil {
		out.Close()
		return err
	}
	if _, err := io.Copy(tw, binary); err != nil {
		out.Close()
		return err
	}

	for _, closer := range []io.Closer{tw, gw, out} {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	return nil
}

type environment []string

func runCmd(env environment, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env

	out, err := cmd.Output()
	if err != nil {
		printKo("error running command [%s %s] with env %v", name, strings.Join(args, " "), env)

		if e, ok := err.(*exec.ExitError); ok {
			fmt.Println()
			fmt.Printf("%s\n", e.Stderr)
			fmt.Println()
		}

		return string(out), err
	}

	printOk("%s %s", name, strings.Join(args, " "))

	return string(out), nil
}

func printOk(s string, a ...interface{}) {
	fmt.Printf("\033[32m[OK]\033[m %s\n", fmt.Sprintf(s, a...))
}

func printKo(s string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "\033[31m[KO]\033[m %s\n", fmt.Sprintf(s, a...))
}

func printInfo(s string, a ...interface{}) {
	fmt.Printf("[+] %s\n", fmt.Sprintf(s, a...))
}
