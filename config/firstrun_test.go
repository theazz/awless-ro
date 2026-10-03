package config

import (
	"os"
	"testing"
)

// The first run must not sync.
//
// Writing the region for the first time went through the same code path as changing
// it, and a region change triggers a sync of the new region. So the first command
// anyone ever ran pulled the whole account before doing its own work: `whoami`, which
// needs a single GetCallerIdentity, waited on nine services and several thousand
// resources. Nothing needed that — commands fetch what they need when they are not
// given --local, and a local copy is what `sync` is for.
func TestFirstRunDoesNotTriggerASync(t *testing.T) {
	restore := isolateConfig(t)
	defer restore()

	AwlessFirstInstall = true
	TriggerSyncOnConfigUpdate = false

	if err := InitConfig(map[string]string{RegionConfigKey: "eu-west-1"}); err != nil {
		t.Fatal(err)
	}

	if TriggerSyncOnConfigUpdate {
		t.Error("the first run scheduled a full sync; whoami would wait on the whole account")
	}
}

// Changing the region afterwards still does, because the local graph is per-region:
// the one already on disk is about somewhere else.
func TestChangingTheRegionLaterDoesTriggerASync(t *testing.T) {
	restore := isolateConfig(t)
	defer restore()

	AwlessFirstInstall = true
	if err := InitConfig(map[string]string{RegionConfigKey: "eu-west-1"}); err != nil {
		t.Fatal(err)
	}

	// The install is over; this is a user changing their mind.
	AwlessFirstInstall = false
	TriggerSyncOnConfigUpdate = false

	if err := Set(RegionConfigKey, "us-east-1"); err != nil {
		t.Fatal(err)
	}

	if !TriggerSyncOnConfigUpdate {
		t.Error("changing the region did not schedule a sync, so the local graph would " +
			"describe the region the user just left")
	}
}

// Turning autosync off is respected for a real region change.
func TestAutosyncOffSuppressesTheSync(t *testing.T) {
	restore := isolateConfig(t)
	defer restore()

	AwlessFirstInstall = true
	if err := InitConfig(map[string]string{RegionConfigKey: "eu-west-1"}); err != nil {
		t.Fatal(err)
	}

	AwlessFirstInstall = false
	if err := Set(autosyncConfigKey, "false"); err != nil {
		t.Fatal(err)
	}
	TriggerSyncOnConfigUpdate = false

	if err := Set(RegionConfigKey, "us-east-1"); err != nil {
		t.Fatal(err)
	}

	if TriggerSyncOnConfigUpdate {
		t.Error("autosync is off and a sync was scheduled anyway")
	}
}

// isolateConfig points the config store at a temporary home and restores the package
// state the tests here mutate.
func isolateConfig(t *testing.T) func() {
	t.Helper()

	dir, err := os.MkdirTemp("", "awless-firstrun")
	if err != nil {
		t.Fatal(err)
	}
	previousHome := os.Getenv("__AWLESS_HOME")
	os.Setenv("__AWLESS_HOME", dir)

	prevDefs, prevDefaults := configDefinitions, defaultsDefinitions
	prevConfig, prevFirst, prevTrigger := Config, AwlessFirstInstall, TriggerSyncOnConfigUpdate

	// Only the two keys these tests turn on, so an unrelated definition cannot make
	// InitConfig fail for reasons of its own.
	configDefinitions = map[string]*Definition{
		autosyncConfigKey: {defaultValue: "true", parseParamFn: parseBool},
		RegionConfigKey:   {onUpdateFns: []onUpdateFunc{runSyncWithUpdatedRegion}},
	}
	defaultsDefinitions = map[string]*Definition{}
	Config = make(map[string]interface{})

	return func() {
		configDefinitions, defaultsDefinitions = prevDefs, prevDefaults
		Config, AwlessFirstInstall, TriggerSyncOnConfigUpdate = prevConfig, prevFirst, prevTrigger
		os.Setenv("__AWLESS_HOME", previousHome)
		os.RemoveAll(dir)
	}
}
