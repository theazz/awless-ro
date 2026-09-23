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

// Upstream awless phoned home to https://updates.awless.io on almost every
// command to advertise new releases. That service belonged to WALLIX and is
// gone, so the check was removed rather than repointed: a read-only inspection
// tool has no business making an unsolicited network call on every invocation.
//
// The semver comparison stays, because it still drives the one-off "you just
// upgraded" notice.

import (
	"errors"
	"strconv"
	"strings"
)

const semverLen = 3

type semver [semverLen]int

var SemverInvalidFormatErr = errors.New("semver invalid format")

func IsSemverUpgrade(current, latest string) bool {
	i, err := CompareSemver(current, latest)
	if err != nil {
		return false
	}

	return i < 0
}

func CompareSemver(current, latest string) (int, error) {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	dot := func(r rune) bool {
		return r == '.'
	}
	cFields := strings.FieldsFunc(current, dot)
	lFields := strings.FieldsFunc(latest, dot)

	if len(cFields) != semverLen || len(lFields) != semverLen {
		return 0, SemverInvalidFormatErr
	}

	currents := new(semver)
	for i, f := range cFields {
		num, err := strconv.Atoi(f)
		if err != nil {
			return 0, SemverInvalidFormatErr
		}
		currents[i] = num
	}

	latests := new(semver)
	for i, f := range lFields {
		num, err := strconv.Atoi(f)
		if err != nil {
			return 0, SemverInvalidFormatErr
		}
		latests[i] = num
	}

	for i := 0; i < semverLen; i++ {
		if latests[i] > currents[i] {
			return -1, nil
		} else if latests[i] == currents[i] {
			continue
		} else {
			return 1, nil
		}
	}

	return 0, nil
}
