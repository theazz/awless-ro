package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	awsconfig "github.com/theazz/awless-ro/aws/config"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/database"
)

func TestSwitchWarnsWhenEnvironmentOverridesSavedProfile(t *testing.T) {
	output := captureLog(t)

	awsHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(awsHome, "config"), []byte("[profile current]\n[profile staging]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(awsHome, "credentials"), []byte("[current]\naws_access_key_id = test\naws_secret_access_key = test\n[staging]\naws_access_key_id = test\naws_secret_access_key = test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	previousAWSHomeDir := awsconfig.AWSHomeDir
	awsconfig.AWSHomeDir = func() string { return awsHome }
	t.Cleanup(func() { awsconfig.AWSHomeDir = previousAWSHomeDir })
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(awsHome, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(awsHome, "credentials"))
	t.Setenv("AWS_PROFILE", "current")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	t.Setenv("AWS_REGION", "")

	awlessHome := t.TempDir()
	previousHome, previousDBPath, previousKeysDir := config.AwlessHome, config.DBPath, config.KeysDir
	previousConfig, previousDefaults := config.Config, config.Defaults
	previousSync := config.TriggerSyncOnConfigUpdate
	previousProfileFlag, previousRegionFlag := awsProfileGlobalFlag, awsRegionGlobalFlag
	previousProfileOverride, previousRegionOverride := profileOverridenThrough, regionOverridenThrough
	config.AwlessHome = awlessHome
	config.DBPath = filepath.Join(awlessHome, database.Filename)
	config.KeysDir = filepath.Join(awlessHome, "keys")
	t.Setenv("__AWLESS_HOME", awlessHome)
	config.Config = map[string]interface{}{}
	config.Defaults = map[string]interface{}{}
	config.TriggerSyncOnConfigUpdate = false
	awsProfileGlobalFlag, awsRegionGlobalFlag = "", ""
	profileOverridenThrough, regionOverridenThrough = "", ""
	t.Cleanup(func() {
		config.AwlessHome, config.DBPath, config.KeysDir = previousHome, previousDBPath, previousKeysDir
		config.Config, config.Defaults = previousConfig, previousDefaults
		config.TriggerSyncOnConfigUpdate = previousSync
		awsProfileGlobalFlag, awsRegionGlobalFlag = previousProfileFlag, previousRegionFlag
		profileOverridenThrough, regionOverridenThrough = previousProfileOverride, previousRegionOverride
	})

	if err := config.InitAwlessEnv("current"); err != nil {
		t.Fatalf("initialize isolated config: %v", err)
	}
	switchCmd.Run(switchCmd, []string{"staging"})
	switchCmd.PersistentPostRun(switchCmd, nil)

	if !strings.Contains(output.String(), "switch saved profile 'staging'") || !strings.Contains(output.String(), "AWS_PROFILE variable overrides it with 'current'") || !strings.Contains(output.String(), "unset AWS_PROFILE") {
		t.Errorf("switch did not explain that AWS_PROFILE overrides the saved profile; output: %s", output.String())
	}
	if got := config.GetAWSProfile(); got != "current" {
		t.Errorf("effective profile = %q, want current", got)
	}

	switchCmd.Run(switchCmd, []string{"eu-west-2"})
	switchCmd.PersistentPostRun(switchCmd, nil)
	if !strings.Contains(output.String(), "switch saved region 'eu-west-2'") || !strings.Contains(output.String(), "AWS_DEFAULT_REGION variable overrides it with 'us-east-1'") {
		t.Errorf("switch did not explain that AWS_DEFAULT_REGION overrides the saved region; output: %s", output.String())
	}
	if err := database.Execute(func(db *database.DB) error {
		values, err := db.GetConfigs("userconfig")
		if err != nil {
			return err
		}
		if got := values[config.ProfileConfigKey]; got != "staging" {
			t.Errorf("persisted profile = %v, want staging", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
}

func TestSwitchOverrideAdvice(t *testing.T) {
	cases := []struct{ setting, source, requested, want string }{
		{"profile", "AWS_PROFILE variable", "staging",
			"unset AWS_PROFILE to use it, export AWS_PROFILE=staging, or pass --aws-profile staging on each command"},
		{"profile", "AWS_DEFAULT_PROFILE variable", "staging",
			"unset AWS_DEFAULT_PROFILE to use it, export AWS_DEFAULT_PROFILE=staging, or pass --aws-profile staging on each command"},
		{"region", "AWS_DEFAULT_REGION variable", "eu-west-3",
			"unset AWS_DEFAULT_REGION to use it, export AWS_DEFAULT_REGION=eu-west-3, or pass --aws-region eu-west-3 on each command"},
		{"region", "profile 'withregion' (see AWS config files $HOME/.aws/{credentials,config})", "eu-west-3",
			"a region saved by switch is not used while that profile sets one: remove `region` from the profile, " +
				"export AWS_DEFAULT_REGION=eu-west-3, or pass --aws-region eu-west-3 on each command"},
		// A value the shell would split or interpret is quoted, so the advice can be copied as is.
		{"profile", "AWS_PROFILE variable", "x;rm",
			"unset AWS_PROFILE to use it, export AWS_PROFILE='x;rm', or pass --aws-profile 'x;rm' on each command"},
		{"profile", "AWS_PROFILE variable", "it's",
			`unset AWS_PROFILE to use it, export AWS_PROFILE='it'\''s', or pass --aws-profile 'it'\''s' on each command`},
	}
	for _, tc := range cases {
		if got := switchOverrideAdvice(tc.setting, tc.source, tc.requested); got != tc.want {
			t.Errorf("switchOverrideAdvice(%q, %q, %q)\n got %q\nwant %q", tc.setting, tc.source, tc.requested, got, tc.want)
		}
	}
}

// -p and -r last for one command and switch saves for every later one, so switch
// refuses them rather than guess which one the user meant.
func TestSwitchRefusesProfileAndRegionFlags(t *testing.T) {
	cases := []struct {
		profile, region string
		refused         bool
	}{
		{"", "", false},
		{"prod", "", true},
		{"", "eu-west-1", true},
		{"prod", "eu-west-1", true},
	}
	for _, tc := range cases {
		err := switchFlagsError(tc.profile, tc.region)
		if refused := err != nil; refused != tc.refused {
			t.Errorf("switchFlagsError(%q, %q) = %v, want refused=%v", tc.profile, tc.region, err, tc.refused)
		}
		if err != nil && !strings.Contains(err.Error(), "awless-ro switch PROFILE REGION") {
			t.Errorf("error does not say how to pass them: %v", err)
		}
	}
}
