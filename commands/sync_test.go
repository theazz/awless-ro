package commands

import (
	"testing"

	"github.com/theazz/awless-ro/config"
)

// Two resource types are off by default because they cost an API call per parent —
// one per hosted zone, one per bucket. Reporting them as "0" said something about the
// account that was not true: `-> dns: 2 zones, 0 record` against an account holding 84
// records. The sync has to distinguish "looked and found none" from "did not look".
func TestDisabledResourceTypesAreRecognised(t *testing.T) {
	previous := config.Config
	config.Config = map[string]interface{}{
		"aws.dns.record.sync":       false,
		"aws.storage.s3object.sync": false,
		"aws.infra.sync":            true,
		"aws.monitoring.sync":       false,
	}
	t.Cleanup(func() { config.Config = previous })

	cases := []struct {
		service, resourceType string
		wantKey               string
		wantOff               bool
	}{
		// Off by its own per-type key.
		{"dns", "record", "aws.dns.record.sync", true},
		{"storage", "s3object", "aws.storage.s3object.sync", true},
		// On, even though a sibling type of the same service is off.
		{"dns", "zone", "", false},
		{"storage", "bucket", "", false},
		// Off because the whole service is off, which the per-type key does not say.
		{"monitoring", "alarm", "aws.monitoring.sync", true},
		{"monitoring", "metric", "aws.monitoring.sync", true},
		// On.
		{"infra", "instance", "", false},
		// A service with no entry at all is not reported as off, or every unknown
		// type would claim to be disabled.
		{"nosuchservice", "nosuchtype", "", false},
	}

	for _, tc := range cases {
		key, off := syncDisabledFor(tc.service, tc.resourceType)
		if off != tc.wantOff {
			t.Errorf("%s/%s: off = %v, want %v", tc.service, tc.resourceType, off, tc.wantOff)
		}
		if key != tc.wantKey {
			t.Errorf("%s/%s: key = %q, want %q", tc.service, tc.resourceType, key, tc.wantKey)
		}
	}
}

// Turning a service off turns its types off whatever their own keys say, because
// that is what the service does: IsSyncDisabled returns an empty graph before the
// per-type key is ever read. The message has to name the setting that is actually
// having the effect, or it sends the user to change the wrong one.
func TestTheServiceSwitchWinsOverThePerTypeOne(t *testing.T) {
	previous := config.Config
	config.Config = map[string]interface{}{
		"aws.monitoring.sync":       false,
		"aws.monitoring.alarm.sync": true,
	}
	t.Cleanup(func() { config.Config = previous })

	key, off := syncDisabledFor("monitoring", "alarm")
	if !off {
		t.Fatal("alarm reported on, but its whole service is off")
	}
	if key != "aws.monitoring.sync" {
		t.Errorf("key = %q, want aws.monitoring.sync: the per-type key is not what stops it", key)
	}
}

// A value that is not a bool must not be read as "off": the config store holds
// strings for some keys, and treating one as false would hide a synced type.
func TestNonBooleanSettingIsNotTreatedAsOff(t *testing.T) {
	previous := config.Config
	config.Config = map[string]interface{}{"aws.dns.record.sync": "false"}
	t.Cleanup(func() { config.Config = previous })

	if key, off := syncDisabledFor("dns", "record"); off {
		t.Errorf("a string value under %q was read as a disabled flag", key)
	}
}
