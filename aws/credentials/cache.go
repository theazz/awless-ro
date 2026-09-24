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

package awscredentials

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/theazz/awless-ro/logger"
)

// The SDK caches credentials in memory for the life of a process. That is no help
// to a CLI: every `awless-ro list instances` is a new process, so an assume-role
// profile with MFA would ask for a token code every single time. This cache puts
// them on disk instead, keyed by profile, so one token code covers a session.
//
// Only credentials that expire are written. Long-lived keys from
// ~/.aws/credentials are already on disk under the user's own management; copying
// them somewhere else would widen the exposure and buy nothing, since reading the
// original costs no more than reading a cache of it.

// diskCacheProvider wraps a credentials provider with a file on disk.
type diskCacheProvider struct {
	upstream awssdk.CredentialsProvider
	dir      string
	profile  string
	log      *logger.Logger

	mu     sync.Mutex
	cached *awssdk.Credentials
}

// cacheFile is the on-disk shape. It is written by this version only: the cache
// directory moved with the rest of the state when ~/.awless became ~/.awless-ro,
// so there is nothing older to read, and a credential that lives for an hour is
// not worth a migration.
type cacheFile struct {
	AccessKeyID     string    `json:"AccessKeyID"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	SessionToken    string    `json:"SessionToken"`
	Source          string    `json:"Source"`
	AccountID       string    `json:"AccountID,omitempty"`
	Expires         time.Time `json:"Expires"`
}

// newDiskCache returns upstream unchanged when there is nowhere to cache, so that a
// missing cache directory degrades to no caching rather than to an error.
func newDiskCache(upstream awssdk.CredentialsProvider, dir, profile string, log *logger.Logger) awssdk.CredentialsProvider {
	if dir == "" {
		return upstream
	}
	if log == nil {
		log = logger.DiscardLogger
	}
	return &diskCacheProvider{upstream: upstream, dir: dir, profile: profile, log: log}
}

func (p *diskCacheProvider) path() string {
	// The profile name reaches this from a flag or the environment, and it goes
	// into a path, so a name like "../../x" would escape the cache directory.
	// Taking only the last element keeps it inside.
	name := filepath.Base(filepath.Clean(p.profile))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "default"
	}
	return filepath.Join(p.dir, fmt.Sprintf("aws-profile-%s.json", name))
}

func (p *diskCacheProvider) Retrieve(ctx context.Context) (awssdk.Credentials, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cached != nil && !expired(*p.cached) {
		return *p.cached, nil
	}

	if creds, ok := p.readFile(); ok {
		p.cached = &creds
		return creds, nil
	}

	creds, err := p.upstream.Retrieve(ctx)
	if err != nil {
		return creds, err
	}

	if creds.CanExpire {
		p.writeFile(creds)
	}
	p.cached = &creds

	return creds, nil
}

// expired treats credentials as spent slightly before they really are, because a
// command that starts with two minutes left can still be making calls when they run
// out.
const expiryMargin = 2 * time.Minute

func expired(c awssdk.Credentials) bool {
	return c.CanExpire && !c.Expires.After(time.Now().UTC().Add(expiryMargin))
}

func (p *diskCacheProvider) readFile() (awssdk.Credentials, bool) {
	path := p.path()

	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			p.log.Verbosef("cannot read cached credentials at %q: %s", path, err)
		}
		return awssdk.Credentials{}, false
	}

	warnIfReadableByOthers(path, p.log)

	var file cacheFile
	if err := json.Unmarshal(content, &file); err != nil {
		// A cache is disposable, so a damaged one is not an error worth stopping
		// for: it is dropped and the upstream provider asked again.
		p.log.Verbosef("discarding unreadable cached credentials at %q: %s", path, err)
		return awssdk.Credentials{}, false
	}

	creds := awssdk.Credentials{
		AccessKeyID:     file.AccessKeyID,
		SecretAccessKey: file.SecretAccessKey,
		SessionToken:    file.SessionToken,
		Source:          file.Source,
		AccountID:       file.AccountID,
		CanExpire:       true,
		Expires:         file.Expires,
	}
	if !creds.HasKeys() || expired(creds) {
		return awssdk.Credentials{}, false
	}

	p.log.ExtraVerbosef("using credentials cached at %q, valid until %s", path, file.Expires.Format(time.RFC3339))
	return creds, true
}

func (p *diskCacheProvider) writeFile(creds awssdk.Credentials) {
	path := p.path()

	content, err := json.Marshal(cacheFile{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Source:          creds.Source,
		AccountID:       creds.AccountID,
		Expires:         creds.Expires.UTC(),
	})
	if err != nil {
		p.log.Verbosef("cannot serialise credentials for caching: %s", err)
		return
	}

	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		p.log.Verbosef("cannot create the credentials cache directory %q: %s", p.dir, err)
		return
	}

	// Written to a temporary file and renamed, so that a crash halfway through
	// cannot leave a truncated file that later reads treat as a live credential.
	// The temporary name carries the process id to keep two concurrent commands
	// from writing the same file.
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		p.log.Verbosef("cannot write cached credentials to %q: %s", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		p.log.Verbosef("cannot install cached credentials at %q: %s", path, err)
		return
	}

	p.log.ExtraVerbosef("cached credentials at %q until %s", path, creds.Expires.UTC().Format(time.RFC3339))
}

// warnIfReadableByOthers says something when the cache has ended up with wider
// permissions than it was written with, since it holds a live session token.
func warnIfReadableByOthers(path string, log *logger.Logger) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		log.Warningf("permissions %#o on cached credentials %q allow other users to read a live session token", mode, path)
	}
}
