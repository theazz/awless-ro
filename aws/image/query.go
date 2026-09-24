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
	"regexp"
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

	// Mainstream returns a regular expression, anchored at both ends, matching only
	// the vendor's ordinary general-purpose line.
	//
	// NamePattern cannot do this job. It is an EC2 name filter, whose only wildcard
	// matches any run of characters including hyphens, so a glob cannot say "and no
	// further words here" — suse-sles-15-sp6-v*-hvm-ssd-* matches
	// suse-sles-15-sp6-v20260919-ecs-hvm-ssd-x86_64 just as happily as the image
	// meant by it.
	//
	// That mattered, because vendors publish specialised images beside the ordinary
	// one, under names sharing its prefix, and sometimes a few seconds newer. Picking
	// the newest match therefore returned, against a live account in September 2026:
	//
	//	amazonlinux  al2023-ami-minimal-…      instead of al2023-ami-…
	//	suselinux    …-sp6-chost-byos-…        instead of …-sp6-v…-hvm-ssd-…
	//	debian       debian-13-backports-…     instead of debian-13-amd64-…
	//
	// The SUSE one is the worst of the three: byos means bring your own subscription,
	// so that image comes up unregistered, and chost is a container host rather than
	// a general-purpose server. None of it is detectable from the id we printed.
	//
	// Anything not matching is dropped. A convention that moves on therefore produces
	// ErrNoMatch, naming the pattern, rather than a quietly wrong id — the same
	// preference for a loud failure that ErrNoOwner is built on. `--owner` with
	// `--name` bypasses this entirely and is the escape hatch.
	//
	// Interpolated query tokens must go through regexp.QuoteMeta: distro and variant
	// come from the command line.
	Mainstream func(q Query) string

	// Release extracts the part of an image name identifying which release it is,
	// for vendors that publish one release as several images, and returns "" for a
	// name it does not recognise.
	//
	// Amazon Linux 2023 publishes a kernel 6.1, a 6.12 and a 6.18 build of every
	// version, within seconds of each other. Ordering those by publication time made
	// the answer to `search images amazonlinux --latest-id` turn on a one-second gap:
	// against a live account in September 2026 the 6.1 build was published one second
	// after the 6.18 build and so was reported as the latest image, while AWS's own
	// al2023-ami-kernel-default pointer named the 6.18 one.
	//
	// Where this is declared, images sharing a release are treated as published
	// together and ranked by name instead, descending, which prefers the higher
	// kernel. Releases are still ranked by date, newest first.
	//
	// Vendors publishing a single image per release, which is most of them, leave
	// this nil.
	Release func(name string) string
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

// MainstreamFilter is the expression names must match to count as the vendor's
// ordinary line, or nil if the owner declares none.
//
// The error is all but unreachable, because the builders quote the query tokens they
// interpolate, but a compile failure here would otherwise be a panic on input a user
// typed.
func (q Query) MainstreamFilter() (*regexp.Regexp, error) {
	if q.Owner.Mainstream == nil {
		return nil, nil
	}
	src := q.Owner.Mainstream(q)
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, fmt.Errorf("image query %s: owner %s built an invalid name expression %q: %w",
			q, q.Owner.Name, src, err)
	}
	return re, nil
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
		// The architecture segment is one word, which is what excludes the Pro
		// images: those read amd64-pro-server rather than amd64-server.
		Mainstream: func(q Query) string {
			distro, variant := regexp.QuoteMeta(q.Distro), regexp.QuoteMeta(q.Variant)
			return fmt.Sprintf(`^%s/images/[^/]+/%s-%s-[0-9.]+-[a-z0-9]+-server-[0-9.]+$`,
				distro, distro, variant)
		},
	},
	"debian": {
		Name: "debian", AccountID: "136693071363",
		DistroName: "debian", LatestVariant: "13",
		// debian-13-amd64-20250810-2160
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s-%s-*", q.Distro, q.Variant)
		},
		// One word for the architecture, so debian-13-backports-amd64-… does not
		// match: that image tracks the backports kernel, which is a different
		// support proposition from the release Debian ships.
		Mainstream: func(q Query) string {
			return fmt.Sprintf(`^%s-%s-[a-z0-9]+-[0-9]+-[0-9]+$`,
				regexp.QuoteMeta(q.Distro), regexp.QuoteMeta(q.Variant))
		},
	},
	"redhat": {
		Name: "redhat", AccountID: "309956199498",
		DistroName: "RHEL", LatestVariant: "9",
		// RHEL-9.4.0_HVM-20240605-x86_64-82-Hourly2-GP3
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s-%s*", q.Distro, q.Variant)
		},
		// Hourly2 is the pay-as-you-go build, the one that works without a
		// subscription of one's own; Access2 is the bring-your-own counterpart. The
		// storage class is left open because RHEL 7 era images end in GP2.
		// _HVM_GA, a release-day marker Red Hat occasionally adds, is excluded by
		// _HVM being followed directly by the date.
		Mainstream: func(q Query) string {
			return fmt.Sprintf(`^%s-%s[0-9.]*_HVM-[0-9]+-[a-z0-9_]+-[0-9]+-Hourly2-GP[0-9]+$`,
				regexp.QuoteMeta(q.Distro), regexp.QuoteMeta(q.Variant))
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
		// The two generations are named differently enough to need separate
		// expressions. Both turn on requiring the version stamp, or a word naming a
		// kernel, right after -ami-; that is what excludes -ami-minimal- and, for
		// amzn2, -ami-minimal-selinux-enforcing-.
		Mainstream: func(q Query) string {
			distro := regexp.QuoteMeta(q.Distro)
			if strings.HasPrefix(q.Distro, "al2") && !strings.HasPrefix(q.Distro, "amzn") {
				// al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64
				return fmt.Sprintf(`^%s-ami-[0-9][0-9.]*-kernel-[0-9.]+-[a-z0-9_]+$`, distro)
			}
			// amzn2-ami-kernel-5.10-hvm-2.0.20260923.0-x86_64-gp2, and the older
			// amzn2-ami-hvm-2.0.20260923.0-x86_64-gp2 without the kernel segment.
			return fmt.Sprintf(`^%s-ami-(kernel-[0-9.]+-)?hvm-[0-9][0-9.]*-[a-z0-9_]+-(ebs|gp[0-9]+)$`, distro)
		},
		// The version stamp, which is what the kernel builds of one release share.
		Release: func(name string) string {
			if m := amazonReleaseStamp.FindStringSubmatch(name); m != nil {
				return m[1]
			}
			return ""
		},
	},
	"suselinux": {
		Name: "suselinux", AccountID: "013907871322",
		DistroName: "sles", LatestVariant: "15",
		// suse-sles-15-sp6-v20240624-hvm-ssd-x86_64
		NamePattern: func(q Query) string {
			return fmt.Sprintf("suse-%s-%s-*", q.Distro, q.Variant)
		},
		// The service pack is the only word allowed before the build stamp, and the
		// stamp is followed directly by hvm-ssd. That is what rules out the
		// specialised builds SUSE publishes alongside: chost-byos, sapcal, and the
		// ECS variant, whose name inserts -ecs- exactly where a glob cannot see it.
		Mainstream: func(q Query) string {
			return fmt.Sprintf(`^suse-%s-%s(-sp[0-9]+)?-v[0-9]+-hvm-ssd-[a-z0-9_]+$`,
				regexp.QuoteMeta(q.Distro), regexp.QuoteMeta(q.Variant))
		},
	},
	"windows": {
		Name: "windows", AccountID: "801119661308",
		DistroName: "Windows_Server", LatestVariant: "2022",
		// Windows_Server-2022-English-Full-Base-2024.07.10
		NamePattern: func(q Query) string {
			return fmt.Sprintf("%s-%s-English-Full-Base-*", q.Distro, q.Variant)
		},
		// The glob already pins the edition; this only requires the name to end at
		// the date rather than carry a further word.
		Mainstream: func(q Query) string {
			return fmt.Sprintf(`^%s-%s-English-Full-Base-[0-9.]+$`,
				regexp.QuoteMeta(q.Distro), regexp.QuoteMeta(q.Variant))
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

// The version stamp in an Amazon Linux name, covering both generations:
// al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64 and
// amzn2-ami-kernel-5.10-hvm-2.0.20260923.0-x86_64-gp2.
var amazonReleaseStamp = regexp.MustCompile(`-ami-(?:minimal-)?(?:selinux-enforcing-)?(?:kernel-[0-9.]+-)?(?:hvm-)?([0-9][0-9.]*[0-9])`)

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
