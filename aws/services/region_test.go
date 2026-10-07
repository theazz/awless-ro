package awsservices

import (
	"os"
	"path/filepath"
	"testing"
)

// isolateRegionLookup points the AWS SDK at a throwaway shared config file and clears
// everything else, so a developer's real profiles cannot decide the outcome.
func isolateRegionLookup(t *testing.T, sharedConfig string) {
	t.Helper()

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgFile, []byte(sharedConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("AWS_CONFIG_FILE", cfgFile)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials-that-do-not-exist"))
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	// Otherwise a test run on an EC2 instance would get a region from the instance
	// and never reach the assertion about the interactive prompt.
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// stubRegionSelector replaces the interactive prompt, which a test must never reach:
// it reads stdin and would block the whole run.
func stubRegionSelector(t *testing.T, answer string) *int {
	t.Helper()

	var calls int
	previous := regionSelector
	regionSelector = func() string {
		calls++
		return answer
	}
	t.Cleanup(func() { regionSelector = previous })
	return &calls
}

// A profile that names a region answers the first-run question, so `awless-ro -p beta`
// on a fresh machine does not ask for one. It used to: the region was resolved without
// the profile, found nothing because there is no [default] section, and dropped into
// the interactive selector — which then stored whatever was typed as the default
// region, for a profile that already said which region it meant.
func TestResolveRegionFromEnvUsesTheChosenProfilesRegion(t *testing.T) {
	isolateRegionLookup(t, "[profile beta]\nregion = us-east-2\n")
	calls := stubRegionSelector(t, "eu-west-3")

	if got := ResolveRegionFromEnv("beta"); got != "us-east-2" {
		t.Errorf("region = %q, want the one beta names", got)
	}
	if *calls != 0 {
		t.Errorf("the interactive selector was reached %d times", *calls)
	}
}

// With no profile chosen there is nothing to read a region from, and asking is still
// the right answer. Unchanged behaviour, pinned so the fix above cannot quietly turn
// a first run on a bare machine into a failure.
func TestResolveRegionFromEnvStillAsksWhenNothingNamesARegion(t *testing.T) {
	isolateRegionLookup(t, "[profile beta]\nregion = us-east-2\n")
	calls := stubRegionSelector(t, "eu-west-3")

	if got := ResolveRegionFromEnv(""); got != "eu-west-3" {
		t.Errorf("region = %q, want the answer from the selector", got)
	}
	if *calls != 1 {
		t.Errorf("the interactive selector ran %d times, want 1", *calls)
	}
}

// A profile name that does not exist is not a failure, it is "no region from the
// profile": a stale AWS_PROFILE or a typo should still let the environment answer
// rather than forcing the question.
func TestResolveRegionFromEnvToleratesAProfileThatDoesNotExist(t *testing.T) {
	isolateRegionLookup(t, "[profile beta]\nregion = us-east-2\n")
	t.Setenv("AWS_REGION", "eu-central-1")
	calls := stubRegionSelector(t, "eu-west-3")

	if got := ResolveRegionFromEnv("ghost"); got != "eu-central-1" {
		t.Errorf("region = %q, want the one in the environment", got)
	}
	if *calls != 0 {
		t.Errorf("the interactive selector was reached %d times", *calls)
	}
}

// Same tolerance when the stale name came from AWS_PROFILE rather than a flag. This
// needs its own case because the SDK reads AWS_PROFILE itself: simply retrying without
// the profile option loaded 'ghost' a second time — and, with AWS_PROFILE set, the SDK
// treats a missing profile as fatal — so the environment region was never reached and
// a first run dropped into the interactive selector.
func TestResolveRegionFromEnvToleratesAStaleProfileInTheEnvironment(t *testing.T) {
	isolateRegionLookup(t, "[profile beta]\nregion = us-east-2\n")
	t.Setenv("AWS_PROFILE", "ghost")
	t.Setenv("AWS_REGION", "eu-central-1")
	calls := stubRegionSelector(t, "eu-west-3")

	if got := ResolveRegionFromEnv("ghost"); got != "eu-central-1" {
		t.Errorf("region = %q, want the one in the environment", got)
	}
	if *calls != 0 {
		t.Errorf("the interactive selector was reached %d times", *calls)
	}
	if got := os.Getenv("AWS_PROFILE"); got != "ghost" {
		t.Errorf("AWS_PROFILE = %q after the call, want it left as it was", got)
	}
}
