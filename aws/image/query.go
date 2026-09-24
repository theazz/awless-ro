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

// Package awsimage finds official AMIs by describing what you want instead of by
// naming an id, since an id differs per region.
//
// A query is a colon-separated string, everything optional but the owner:
//
//	owner:distro:variant:arch:virtualization:store
//
// For example canonical, canonical:ubuntu:jammy, redhat::9, debian:::arm64, or
// amazonlinux:amzn2 for the previous Amazon Linux generation.
package awsimage

import (
	"fmt"
	"sort"
	"strings"
)

// QuerySpec is the query format, quoted in command help and error messages.
const QuerySpec = "owner:distro:variant:arch:virtualization:store"

// Owner is a vendor whose published images this package knows how to recognise.
type Owner struct {
	// Name is what a user types as the first token of a query.
	Name string

	// AccountID is the AWS account the vendor publishes from. Every lookup is
	// pinned to it, and that is a security control rather than an optimisation:
	// anyone may publish a public AMI under any name they like, so a search by name
	// with no owner will happily return a stranger's image. Published as the whoAMI
	// name confusion attack in 2025, which turned exactly this into code execution
	// for projects that fed the result to run-instances. awless-ro launches nothing,
	// but printing a stranger's id under the heading "latest official Ubuntu" makes
	// us a link in that chain.
	AccountID string

	// DistroName is the distribution token used when the query omits it.
	DistroName string

	// LatestVariant is the release used when the query omits it.
	LatestVariant string

	// NamePattern builds the value for the EC2 name filter, wildcards included, so
	// that matching happens in the API rather than over every public image the
	// vendor has ever published. Canonical alone has tens of thousands.
	//
	// Architecture is deliberately absent from these patterns even though vendors
	// put it in their image names: there is a dedicated architecture filter, and
	// spelling it twice would only add somewhere else to be wrong.
	NamePattern func(q Query) string
}

// Query is a parsed image query.
type Query struct {
	Owner   Owner
	Distro  string
	Variant string
	Arch    string
	Virt    string
	Store   string
}

func (q Query) String() string {
	return strings.Join([]string{q.Owner.Name, q.Distro, q.Variant, q.Arch, q.Virt, q.Store}, ":")
}

// NameFilter is the pattern this query will match image names against.
func (q Query) NameFilter() string {
	if q.Owner.NamePattern == nil {
		return "*"
	}
	return q.Owner.NamePattern(q)
}

var (
	// i386 is still a value EC2 accepts, and is kept for that reason, but none of
	// the vendors below have published a 32-bit image in years. arm64 is the
	// addition that matters: Graviton did not exist when this was first written.
	validArchs = []string{"i386", "x86_64", "arm64"}

	// paravirtual is likewise accepted and likewise dead: no current instance
	// family can boot it.
	validVirts  = []string{"paravirtual", "hvm"}
	validStores = []string{"ebs", "instance-store"}

	defaultArch  = "x86_64"
	defaultVirt  = "hvm"
	defaultStore = "ebs"
)

// The account ids below were each confirmed against vendor or AWS documentation in
// 2026. Two of them had drifted since this was first written: Debian moved from
// 379101102735, and Canonical's image names gained a storage-class segment, so
// ubuntu/images/hvm-ssd/ became ubuntu/images/hvm-ssd-gp3/ — hence the wildcard
// rather than a literal.
//
// Naming conventions are the part of this file that rots. When a pattern stops
// matching, the command says which owner and which pattern it used, and
// `search images --owner <id> --name <pattern>` gets the job done without waiting
// for a release.
//
// Two vendors were dropped rather than updated. CoreOS Container Linux reached end
// of life in 2020. CentOS Linux 7 did so in June 2024, and while CentOS Stream is
// alive, its publishing account could not be confirmed from a source belonging to
// the project or to AWS — the AWS Marketplace listings under that name are
// third-party rebuilds. Shipping an unverified account id is the mistake whoAMI
// relies on, so it is not shipped.
var Owners = map[string]Owner{
	"canonical": {
		Name: "canonical", AccountID: "099720109477",
		DistroName: "ubuntu", LatestVariant: "noble",
		// ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20260714
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s/images/*/%s-%s-*", q.Distro, q.Distro, q.Variant)
		},
	},
	"debian": {
		Name: "debian", AccountID: "136693071363",
		DistroName: "debian", LatestVariant: "13",
		// debian-13-amd64-20250810-2160
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s-%s-*", q.Distro, q.Variant)
		},
	},
	"redhat": {
		Name: "redhat", AccountID: "309956199498",
		DistroName: "RHEL", LatestVariant: "9",
		// RHEL-9.4.0_HVM-20240605-x86_64-82-Hourly2-GP3
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s-%s*", q.Distro, q.Variant)
		},
	},
	"amazonlinux": {
		Name: "amazonlinux", AccountID: "137112412989",
		// The generation lives in the distro token, so amazonlinux gives al2023 and
		// amazonlinux:amzn2 gives the previous one.
		DistroName: "al2023", LatestVariant: "",
		// al2023-ami-2023.5.20240722.0-kernel-6.1-x86_64
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s-ami-*", q.Distro)
		},
	},
	"suselinux": {
		Name: "suselinux", AccountID: "013907871322",
		DistroName: "sles", LatestVariant: "15",
		// suse-sles-15-sp6-v20240624-hvm-ssd-x86_64
		NamePattern: func(q Query) string {
			return fmt.Sprintf("suse-%s-%s-*", q.Distro, q.Variant)
		},
	},
	"windows": {
		Name: "windows", AccountID: "801119661308",
		DistroName: "Windows_Server", LatestVariant: "2022",
		// Windows_Server-2022-English-Full-Base-2024.07.10
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s-%s-English-Full-Base-*", q.Distro, q.Variant)
		},
	},
}

// SupportedOwners lists the owner names a query may start with, sorted so that help
// text and error messages do not reshuffle between runs.
func SupportedOwners() []string {
	names := make([]string, 0, len(Owners))
	for name := range Owners {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ParseQuery reads the colon-separated query format.
func ParseQuery(s string) (Query, error) {
	supported := strings.Join(SupportedOwners(), ", ")
	var q Query

	splits := strings.Split(s, ":")
	if len(splits) > 6 {
		return q, fmt.Errorf("malformed image query %q: too many tokens, expecting format: %s", s, QuerySpec)
	}

	// Owner names and the tokens that go into filters are matched case
	// insensitively, but the values themselves keep the case the vendor uses,
	// because RHEL and Windows_Server appear capitalised in image names.
	ownerName := strings.ToLower(strings.TrimSpace(splits[0]))
	owner, ok := Owners[ownerName]
	if !ok {
		if ownerName == "coreos" || ownerName == "centos" {
			return q, fmt.Errorf("owner %q is no longer supported: %s. Search it directly with --owner and --name (see `awless-ro search images -h`)",
				ownerName, retiredOwners[ownerName])
		}
		return q, fmt.Errorf("unsupported owner %q, expecting one of: %s (see `awless-ro search images -h`)", splits[0], supported)
	}
	q.Owner = owner

	token := func(i int) string {
		if len(splits) > i {
			return strings.TrimSpace(splits[i])
		}
		return ""
	}

	q.Distro = token(1)
	if q.Distro == "" {
		q.Distro = owner.DistroName
	}

	q.Variant = token(2)
	if q.Variant == "" {
		q.Variant = owner.LatestVariant
	}

	var err error
	if q.Arch, err = oneOf("architecture", token(3), defaultArch, validArchs); err != nil {
		return q, err
	}
	if q.Virt, err = oneOf("virtualization", token(4), defaultVirt, validVirts); err != nil {
		return q, err
	}
	if q.Store, err = oneOf("store", token(5), defaultStore, validStores); err != nil {
		return q, err
	}

	return q, nil
}

var retiredOwners = map[string]string{
	"coreos": "CoreOS Container Linux reached end of life in 2020",
	"centos": "CentOS Linux 7 reached end of life in June 2024, and the account publishing CentOS Stream images could not be verified",
}

func oneOf(what, given, fallback string, valid []string) (string, error) {
	if given == "" {
		return fallback, nil
	}
	given = strings.ToLower(given)
	for _, v := range valid {
		if v == given {
			return given, nil
		}
	}
	return "", fmt.Errorf("image query: invalid %s %q, expecting one of: %s", what, given, strings.Join(valid, ", "))
}
