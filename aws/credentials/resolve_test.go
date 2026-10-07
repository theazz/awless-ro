package awscredentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points every AWS lookup at a throwaway home and clears the environment, so
// a developer's real profile cannot make a test pass or fail.
func isolate(t *testing.T) string {
	t.Helper()

	home := withAWSHome(t)
	awsDir := filepath.Join(home, ".aws")

	// HOME deliberately points somewhere else. Our own lookups go through
	// awsconfig.AWSHomeDir and the SDK's through the two file variables below; if
	// anything falls back to HOME instead, these tests fail rather than silently
	// reading the developer's real profiles.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(awsDir, "credentials"))
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(awsDir, "config"))
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	// Otherwise a test run on an EC2 instance would resolve the instance role and
	// every assertion about "no credentials" would be wrong.
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	return awsDir
}

func writeSharedFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePicksUpAnExistingProfile(t *testing.T) {
	awsDir := isolate(t)
	writeSharedFile(t, awsDir, "credentials", "[work]\naws_access_key_id = "+validKeyID+"\naws_secret_access_key = "+validSecret+"\n")

	cfg, err := Resolve(context.Background(), Params{Profile: "work", ProfileExplicit: true, Region: "eu-west-1"})
	if err != nil {
		t.Fatal(err)
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != validKeyID {
		t.Errorf("access key id = %q, want %q", creds.AccessKeyID, validKeyID)
	}
	if cfg.Region != "eu-west-1" {
		t.Errorf("region = %q, want %q", cfg.Region, "eu-west-1")
	}
}

// A region named in the profile is honoured when none was asked for, which is what
// makes `awless-ro -p work list instances` work without also naming a region.
func TestResolveTakesTheRegionFromTheProfile(t *testing.T) {
	awsDir := isolate(t)
	writeSharedFile(t, awsDir, "credentials", "[work]\naws_access_key_id = "+validKeyID+"\naws_secret_access_key = "+validSecret+"\n")
	writeSharedFile(t, awsDir, "config", "[profile work]\nregion = ap-southeast-2\n")

	cfg, err := Resolve(context.Background(), Params{Profile: "work", ProfileExplicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "ap-southeast-2" {
		t.Errorf("region = %q, want %q", cfg.Region, "ap-southeast-2")
	}
}

// With nothing configured and no permission to ask, the error has to tell the user
// what to do rather than repeating the SDK's last-resort failure about instance
// metadata.
func TestResolveWithoutCredentialsExplainsWhatToDo(t *testing.T) {
	isolate(t)

	_, err := Resolve(context.Background(), Params{Profile: "prod", ProfileExplicit: true, Region: "eu-west-1", AllowPrompt: false})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"prod", "aws configure", "--aws-profile", "AWS_ACCESS_KEY_ID"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should mention %q, got:\n%s", want, err)
		}
	}
	if strings.Contains(err.Error(), "IMDS") {
		t.Errorf("the error leaks the SDK's last-resort failure instead of advising:\n%s", err)
	}
}

// A profile that exists but cannot be used is a different problem, and offering to
// type access keys would be bad advice: the fix is to renew the session or repair the
// role, and an existing section is not rewritten anyway.
func TestResolveDoesNotOfferKeysForABrokenProfile(t *testing.T) {
	awsDir := isolate(t)
	// A role to assume, with no source credentials to assume it with.
	writeSharedFile(t, awsDir, "config", "[profile broken]\nrole_arn = arn:aws:iam::123456789012:role/nope\nsource_profile = absent\n")

	_, err := Resolve(context.Background(), Params{Profile: "broken", ProfileExplicit: true, Region: "eu-west-1", AllowPrompt: true})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "configured but cannot be used") {
		t.Errorf("expected the error to distinguish a broken profile from a missing one, got:\n%s", err)
	}
	if strings.Contains(err.Error(), "aws configure") {
		t.Errorf("a broken profile should not be answered with advice to create one:\n%s", err)
	}
}

func TestIsConfigured(t *testing.T) {
	awsDir := isolate(t)

	if isConfigured("work", false) {
		t.Error("nothing is configured yet")
	}

	writeSharedFile(t, awsDir, "credentials", "[work]\naws_access_key_id = x\n")
	if !isConfigured("work", false) {
		t.Error("a profile in the credentials file should count as configured")
	}
	if isConfigured("other", false) {
		t.Error("a different profile is not configured")
	}

	// An empty profile name means the default one.
	writeSharedFile(t, awsDir, "credentials", "[default]\naws_access_key_id = x\n")
	if !isConfigured("", false) {
		t.Error("an empty profile name should resolve to default")
	}
}

// Keys in the environment count as configured even with no files at all, which is the
// usual arrangement in CI.
func TestEnvironmentKeysCountAsConfigured(t *testing.T) {
	isolate(t)

	if isConfigured("anything", false) {
		t.Error("nothing is configured yet")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", validKeyID)
	if !isConfigured("anything", false) {
		t.Error("keys in the environment should count as configured")
	}
}

// Keys in the environment say nothing about a profile that was explicitly asked for:
// resolution was pinned to that profile and never looked at them. Counting them would
// answer `--aws-profile ghost` with "configured but cannot be used" instead of saying
// ghost does not exist.
func TestEnvironmentKeysDoNotVouchForAnExplicitProfile(t *testing.T) {
	isolate(t)
	t.Setenv("AWS_ACCESS_KEY_ID", validKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", validSecret)

	if isConfigured("ghost", true) {
		t.Error("an explicitly requested profile with no section anywhere is not configured")
	}
}

func TestResolveUsesEnvironmentCredentials(t *testing.T) {
	isolate(t)
	t.Setenv("AWS_ACCESS_KEY_ID", validKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", validSecret)

	cfg, err := Resolve(context.Background(), Params{Region: "eu-west-1"})
	if err != nil {
		t.Fatal(err)
	}
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != validKeyID {
		t.Errorf("access key id = %q, want %q", creds.AccessKeyID, validKeyID)
	}
}

// Keys in the environment and no ~/.aws at all — a CI job or a container — with the
// profile left at the implicit default, which is what every command passes when
// nobody named one.
//
// This used to fail outright. The default profile was pinned on the SDK, which treats
// a programmatically set profile as exclusive, so with no shared config file to read
// the load failed with "failed to get shared config profile, default" and the keys the
// tool's own advice tells you to export were never consulted.
func TestResolveUsesEnvironmentCredentialsWithTheImplicitDefaultProfile(t *testing.T) {
	isolate(t)
	t.Setenv("AWS_ACCESS_KEY_ID", validKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", validSecret)

	cfg, err := Resolve(context.Background(), Params{Profile: "default", Region: "eu-west-1"})
	if err != nil {
		t.Fatalf("credentials in the environment were not used: %s", err)
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != validKeyID {
		t.Errorf("access key id = %q, want %q", creds.AccessKeyID, validKeyID)
	}
	if !strings.Contains(creds.Source, "Env") {
		t.Errorf("credentials came from %q, want the environment", creds.Source)
	}
}

// Same arrangement, but with shared files that describe some other profile and have no
// [default] section. Pinning the default profile walked past the keys and down the
// chain to instance metadata; the keys have to win instead.
func TestResolveUsesEnvironmentCredentialsWhenSharedConfigHasNoDefaultSection(t *testing.T) {
	awsDir := isolate(t)
	writeSharedFile(t, awsDir, "config", "[profile beta]\nregion = ap-southeast-2\n")
	t.Setenv("AWS_ACCESS_KEY_ID", validKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", validSecret)

	cfg, err := Resolve(context.Background(), Params{Profile: "default", Region: "eu-west-1"})
	if err != nil {
		t.Fatalf("credentials in the environment were not used: %s", err)
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessKeyID != validKeyID {
		t.Errorf("access key id = %q, want %q", creds.AccessKeyID, validKeyID)
	}
}

// A profile somebody asked for outranks keys in the environment, in both directions:
// its credentials and its region. The profile decides where the synced graph is kept,
// so the credential chain and the profile the run reports have to name the same thing.
func TestResolveWithAnExplicitProfileOutranksEnvironmentKeys(t *testing.T) {
	awsDir := isolate(t)
	writeSharedFile(t, awsDir, "credentials", "[beta]\naws_access_key_id = "+betaKeyID+"\naws_secret_access_key = "+validSecret+"\n")
	writeSharedFile(t, awsDir, "config", "[profile beta]\nregion = ap-southeast-2\n")
	t.Setenv("AWS_ACCESS_KEY_ID", validKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", validSecret)

	for _, tc := range []struct {
		name  string
		chose func(t *testing.T) Params
	}{
		{
			// `awless-ro -p beta ...`
			name:  "command flag",
			chose: func(*testing.T) Params { return Params{Profile: "beta", ProfileExplicit: true} },
		},
		{
			// AWS_PROFILE=beta, which commands/hooks.go resolves to the same thing.
			name: "AWS_PROFILE",
			chose: func(t *testing.T) Params {
				t.Setenv("AWS_PROFILE", "beta")
				return Params{Profile: "beta", ProfileExplicit: true}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Resolve(context.Background(), tc.chose(t))
			if err != nil {
				t.Fatal(err)
			}

			creds, err := cfg.Credentials.Retrieve(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if creds.AccessKeyID != betaKeyID {
				t.Errorf("access key id = %q, want beta's %q", creds.AccessKeyID, betaKeyID)
			}
			if cfg.Region != "ap-southeast-2" {
				t.Errorf("region = %q, want the one beta names", cfg.Region)
			}
		})
	}
}

// A profile that was asked for and does not exist is an error naming it, even with
// keys in the environment that would otherwise have worked. Falling through to them
// would run against a different account than the one the command named.
func TestResolveReportsAnExplicitlyRequestedProfileThatDoesNotExist(t *testing.T) {
	isolate(t)
	t.Setenv("AWS_ACCESS_KEY_ID", validKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", validSecret)

	_, err := Resolve(context.Background(), Params{Profile: "ghost", ProfileExplicit: true, Region: "eu-west-1", AllowPrompt: false})
	if err == nil {
		t.Fatal("expected an error rather than a silent fallthrough to the environment")
	}
	for _, want := range []string{"ghost", "aws configure --profile ghost"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should mention %q, got:\n%s", want, err)
		}
	}
}

// Resolution retrieves once during start-up, so that a broken profile is reported
// before a command begins fetching rather than surfacing halfway through a sync.
func TestResolveVerifiesCredentialsUpFront(t *testing.T) {
	awsDir := isolate(t)
	// A section with a key and no secret: the SDK accepts the profile and fails on
	// retrieval.
	writeSharedFile(t, awsDir, "credentials", "[half]\naws_access_key_id = "+validKeyID+"\n")

	if _, err := Resolve(context.Background(), Params{Profile: "half", ProfileExplicit: true, Region: "eu-west-1"}); err == nil {
		t.Fatal("expected resolution to fail on an incomplete profile")
	}
}

// When asking for credentials fails for a specific reason, that reason is the whole
// error. The generic advice would contradict it: it names the profile resolution
// started with, while the user may have just typed a different one.
func TestSpecificFailuresAreNotBuriedUnderGenericAdvice(t *testing.T) {
	err := notFoundError("default", fmt.Errorf("%w: %q is already defined", ErrProfileExists, "demo"))

	if !errors.Is(err, ErrProfileExists) {
		t.Fatalf("the cause was lost: %v", err)
	}
	if strings.Contains(err.Error(), "aws configure --profile default") {
		t.Errorf("advice about the wrong profile was added:\n%s", err)
	}

	// No terminal is not a specific reason, so the advice is the useful part.
	err = notFoundError("prod", ErrNoTerminal)
	if !strings.Contains(err.Error(), "aws configure --profile prod") {
		t.Errorf("expected the advice, got:\n%s", err)
	}
}

func TestCacheDirFollowsTheAwlessCache(t *testing.T) {
	t.Setenv("__AWLESS_CACHE", "")
	if got := CacheDir(); got != "" {
		t.Errorf("with no cache configured, got %q, want empty", got)
	}

	t.Setenv("__AWLESS_CACHE", filepath.Join("/tmp", "awless-ro-test", "cache"))
	if got, want := CacheDir(), filepath.Join("/tmp", "awless-ro-test", "cache", "credentials"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
