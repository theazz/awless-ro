package config

import (
	"fmt"
	"strings"
)

func GetAWSRegion() string {
	if reg, ok := Config[RegionConfigKey]; ok && reg != "" {
		return fmt.Sprint(reg)
	}
	if reg, ok := Defaults["region"]; ok && reg != "" { // Compatibility with old key
		return fmt.Sprint(reg)
	}
	return ""
}

// DefaultAWSProfile is what GetAWSProfile answers when nothing names a profile.
// It is exported because the difference between this implicit value and a profile
// somebody actually chose decides whether the AWS SDK is pinned to a shared-config
// profile at all, and that comparison is made in commands/hooks.go.
const DefaultAWSProfile = "default"

func GetAWSProfile() string {
	if profile, ok := Config[ProfileConfigKey]; ok && profile != "" {
		return fmt.Sprint(profile)
	}
	if profile, ok := Defaults[ProfileConfigKey]; ok && profile != "" { // Compatibility with old key
		return fmt.Sprint(profile)
	}
	return DefaultAWSProfile
}

func GetAutosync() bool {
	if autoSync, ok := Config[autosyncConfigKey].(bool); ok {
		return autoSync
	}
	if autoSync, ok := Defaults["sync.auto"].(bool); ok { //Compatibility with old key
		return autoSync
	}
	return true
}

func GetConfigWithPrefix(prefix string) map[string]interface{} {
	conf := make(map[string]interface{})
	for k, v := range Config {
		if strings.HasPrefix(k, prefix) {
			conf[k] = v
		}
	}
	return conf
}
