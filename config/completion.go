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

package config

import (
	"reflect"
	"sort"

	awsconfig "github.com/theazz/awless-ro/aws/config"
)

// KeyHelp is a configuration key and the help text shown next to it.
type KeyHelp struct {
	Key, Help string
}

// Keys returns every configuration key, sorted, with its help text. It reads the
// definitions, not the database, so shell completion can offer keys on a machine where
// nothing has been configured yet — and without opening a database another
// awless-ro process may be holding.
func Keys() []KeyHelp {
	var keys []KeyHelp
	for k, d := range configDefinitions {
		keys = append(keys, KeyHelp{Key: k, Help: d.help})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })
	return keys
}

// ValuesFor returns the values worth suggesting for key, or nil when any value goes
// (or the key is unknown). Profiles are not listed here: which sections of ~/.aws
// count as profiles is the caller's to decide, see commands.completionProfiles.
func ValuesFor(key string) []string {
	if key == RegionConfigKey {
		return awsconfig.SuggestedRegions
	}
	// A boolean key is recognised by the parser it declares rather than by its default
	// value, which would make a key whose default merely reads "true" a boolean.
	if d, ok := configDefinitions[key]; ok && d.parseParamFn != nil &&
		reflect.ValueOf(d.parseParamFn).Pointer() == reflect.ValueOf(parseBool).Pointer() {
		return []string{"true", "false"}
	}
	return nil
}
