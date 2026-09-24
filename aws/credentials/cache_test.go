package awscredentials

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
)

// countingProvider records how often the credentials underneath were asked for. That
// count is the whole point of the cache: for an assume-role profile with MFA, each
// call means another token code typed in.
type countingProvider struct {
	creds awssdk.Credentials
	err   error
	calls int
}

func (p *countingProvider) Retrieve(context.Context) (awssdk.Credentials, error) {
	p.calls++
	return p.creds, p.err
}

func temporary(expires time.Time) awssdk.Credentials {
	return awssdk.Credentials{
		AccessKeyID:     "ASIAEXAMPLE000000000",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Source:          "AssumeRoleProvider",
		CanExpire:       true,
		Expires:         expires,
	}
}

func permanent() awssdk.Credentials {
	return awssdk.Credentials{
		AccessKeyID:     "AKIAEXAMPLE000000000",
		SecretAccessKey: "secret",
		Source:          "SharedConfigCredentials",
	}
}

func TestTemporaryCredentialsSurviveAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	expires := time.Now().UTC().Add(time.Hour)

	first := &countingProvider{creds: temporary(expires)}
	if _, err := newDiskCache(first, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}
	if first.calls != 1 {
		t.Fatalf("upstream called %d times, want 1", first.calls)
	}

	// A second cache stands in for the next command: a fresh process, so nothing in
	// memory, only the file.
	second := &countingProvider{creds: temporary(expires)}
	got, err := newDiskCache(second, dir, "prod", nil).Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.calls != 0 {
		t.Errorf("upstream was asked again despite a valid cache; MFA would re-prompt")
	}
	if got.AccessKeyID != "ASIAEXAMPLE000000000" || got.SessionToken != "token" {
		t.Errorf("cached credentials came back wrong: %+v", got)
	}
}

func TestRepeatedRetrieveAsksUpstreamOnce(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	upstream := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	cache := newDiskCache(upstream, dir, "prod", nil)

	for range 5 {
		if _, err := cache.Retrieve(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if upstream.calls != 1 {
		t.Fatalf("upstream called %d times, want 1", upstream.calls)
	}
}

func TestExpiredCacheIsRefreshed(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	stale := &countingProvider{creds: temporary(time.Now().UTC().Add(-time.Minute))}
	if _, err := newDiskCache(stale, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}

	fresh := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	if _, err := newDiskCache(fresh, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}
	if fresh.calls != 1 {
		t.Errorf("an expired cache was used; upstream called %d times, want 1", fresh.calls)
	}
}

// Credentials about to run out are treated as gone, because a command that starts
// with a minute left can still be making API calls when they lapse.
func TestCredentialsNearExpiryAreNotReused(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	nearlyDone := &countingProvider{creds: temporary(time.Now().UTC().Add(30 * time.Second))}
	if _, err := newDiskCache(nearlyDone, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}

	next := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	if _, err := newDiskCache(next, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}
	if next.calls != 1 {
		t.Errorf("credentials within the expiry margin were reused; upstream called %d times, want 1", next.calls)
	}
}

// Long-lived keys are not copied to a second location. They already sit in
// ~/.aws/credentials under the user's own management, and duplicating them would
// widen the exposure while saving nothing.
func TestPermanentCredentialsAreNotWrittenToDisk(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	upstream := &countingProvider{creds: permanent()}
	if _, err := newDiskCache(upstream, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("permanent credentials were cached on disk: %v", entries)
	}
}

func TestCacheFilePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	ctx := context.Background()

	upstream := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	if _, err := newDiskCache(upstream, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("cache directory mode %#o, want 0700", got)
	}

	fileInfo, err := os.Stat(filepath.Join(dir, "aws-profile-prod.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("cache file mode %#o, want 0600 — it holds a live session token", got)
	}
}

// A profile name reaches this from a flag or the environment and goes into a path, so
// it must not be able to point the cache outside its directory.
func TestProfileNameCannotEscapeTheCacheDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cache")

	cases := []struct{ profile, want string }{
		{"prod", "aws-profile-prod.json"},
		{"../../escape", "aws-profile-escape.json"},
		{"/etc/passwd", "aws-profile-passwd.json"},
		{"", "aws-profile-default.json"},
		{".", "aws-profile-default.json"},
	}

	for _, tc := range cases {
		p := &diskCacheProvider{dir: dir, profile: tc.profile}
		if got, want := p.path(), filepath.Join(dir, tc.want); got != want {
			t.Errorf("profile %q gave path %q, want %q", tc.profile, got, want)
		}
	}
}

// A cache is disposable, so damage to it must not stop the command: the file is
// dropped and the real provider asked again.
func TestDamagedCacheIsIgnored(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(dir, "aws-profile-prod.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	upstream := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	if _, err := newDiskCache(upstream, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatalf("a damaged cache broke resolution: %s", err)
	}
	if upstream.calls != 1 {
		t.Errorf("upstream called %d times, want 1", upstream.calls)
	}
}

// A cache holding keys but no expiry would otherwise look valid forever.
func TestCacheWithoutKeysIsIgnored(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	content, err := json.Marshal(cacheFile{Source: "test", Expires: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aws-profile-prod.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}

	upstream := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	if _, err := newDiskCache(upstream, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}
	if upstream.calls != 1 {
		t.Errorf("a cache entry with no keys was used; upstream called %d times, want 1", upstream.calls)
	}
}

// With no cache directory configured, the wrapper gets out of the way rather than
// failing: caching is a convenience, not a requirement.
func TestNoCacheDirectoryMeansNoWrapper(t *testing.T) {
	upstream := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	if got := newDiskCache(upstream, "", "prod", nil); got != awssdk.CredentialsProvider(upstream) {
		t.Errorf("expected the upstream provider unchanged, got %T", got)
	}
}

func TestUpstreamErrorIsReported(t *testing.T) {
	dir := t.TempDir()
	boom := errors.New("boom")

	upstream := &countingProvider{err: boom}
	if _, err := newDiskCache(upstream, dir, "prod", nil).Retrieve(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
}

// A crash partway through a write must not leave something later reads mistake for a
// live credential, so the file is written elsewhere and renamed into place.
func TestWriteLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	upstream := &countingProvider{creds: temporary(time.Now().UTC().Add(time.Hour))}
	if _, err := newDiskCache(upstream, dir, "prod", nil).Retrieve(ctx); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("got %d files in the cache directory, want 1", len(entries))
	}
}
