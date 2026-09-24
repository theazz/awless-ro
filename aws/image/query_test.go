package awsimage

import (
	"strings"
	"testing"
)

func TestParseQueryDefaults(t *testing.T) {
	cases := []struct {
		in                                 string
		distro, variant, arch, virt, store string
	}{
		{"canonical", "ubuntu", "noble", "x86_64", "hvm", "ebs"},
		{"canonical:ubuntu:jammy", "ubuntu", "jammy", "x86_64", "hvm", "ebs"},
		// An empty token falls back to the default, which is what lets a user pin
		// only the thing they care about.
		{"canonical::jammy", "ubuntu", "jammy", "x86_64", "hvm", "ebs"},
		{"canonical:::arm64", "ubuntu", "noble", "arm64", "hvm", "ebs"},
		{"canonical:::::instance-store", "ubuntu", "noble", "x86_64", "hvm", "instance-store"},
		{"debian", "debian", "13", "x86_64", "hvm", "ebs"},
		{"redhat::8", "RHEL", "8", "x86_64", "hvm", "ebs"},
		// The generation of Amazon Linux lives in the distro token.
		{"amazonlinux", "al2023", "", "x86_64", "hvm", "ebs"},
		{"amazonlinux:amzn2", "amzn2", "", "x86_64", "hvm", "ebs"},
		{"suselinux", "sles", "15", "x86_64", "hvm", "ebs"},
		{"windows", "Windows_Server", "2022", "x86_64", "hvm", "ebs"},
		// Owner names are matched case insensitively and whitespace is tolerated.
		{"  CANONICAL  ", "ubuntu", "noble", "x86_64", "hvm", "ebs"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			q, err := ParseQuery(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if q.Distro != tc.distro {
				t.Errorf("distro = %q, want %q", q.Distro, tc.distro)
			}
			if q.Variant != tc.variant {
				t.Errorf("variant = %q, want %q", q.Variant, tc.variant)
			}
			if q.Arch != tc.arch {
				t.Errorf("arch = %q, want %q", q.Arch, tc.arch)
			}
			if q.Virt != tc.virt {
				t.Errorf("virt = %q, want %q", q.Virt, tc.virt)
			}
			if q.Store != tc.store {
				t.Errorf("store = %q, want %q", q.Store, tc.store)
			}
		})
	}
}

func TestParseQueryRejections(t *testing.T) {
	cases := []struct {
		in       string
		mentions string
	}{
		{"nosuchvendor", "unsupported owner"},
		{"canonical:ubuntu:noble:x86_64:hvm:ebs:extra", "too many tokens"},
		{"canonical:::sparc", "invalid architecture"},
		{"canonical::::xen", "invalid virtualization"},
		{"canonical:::::tape", "invalid store"},
		{"", "unsupported owner"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			_, err := ParseQuery(tc.in)
			if err == nil {
				t.Fatalf("expected an error for %q", tc.in)
			}
			if !strings.Contains(err.Error(), tc.mentions) {
				t.Errorf("error should mention %q, got: %s", tc.mentions, err)
			}
		})
	}
}

// arm64 is the addition that matters: Graviton did not exist when this format was
// designed, and the original accepted only i386 and x86_64.
func TestArm64IsAccepted(t *testing.T) {
	q, err := ParseQuery("canonical:::arm64")
	if err != nil {
		t.Fatal(err)
	}
	if q.Arch != "arm64" {
		t.Errorf("arch = %q, want arm64", q.Arch)
	}
}

// A retired owner gets told why, and where to go instead, rather than being lumped in
// with a typo.
func TestRetiredOwnersExplainThemselves(t *testing.T) {
	for _, owner := range []string{"coreos", "centos"} {
		_, err := ParseQuery(owner)
		if err == nil {
			t.Fatalf("%s should not resolve", owner)
		}
		if !strings.Contains(err.Error(), "no longer supported") {
			t.Errorf("%s: error should say it was retired, got: %s", owner, err)
		}
		if !strings.Contains(err.Error(), "--owner") {
			t.Errorf("%s: error should point at the escape hatch, got: %s", owner, err)
		}
	}
}

func TestQueryStringRoundTrip(t *testing.T) {
	q, err := ParseQuery("canonical:ubuntu:jammy:arm64:hvm:ebs")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := q.String(), "canonical:ubuntu:jammy:arm64:hvm:ebs"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// The string form is the cache key, so two queries that differ must not collapse
	// to the same text.
	other, err := ParseQuery("canonical:ubuntu:noble:arm64:hvm:ebs")
	if err != nil {
		t.Fatal(err)
	}
	if q.String() == other.String() {
		t.Error("different queries produced the same string")
	}
}

// Every owner has to be complete, or a query naming it produces a request with an
// empty account or a pattern that matches everything.
func TestOwnerTableIsComplete(t *testing.T) {
	for name, owner := range Owners {
		if owner.Name != name {
			t.Errorf("%s: Name is %q, the map key and the field must agree", name, owner.Name)
		}
		if owner.AccountID == "" {
			t.Errorf("%s: no account id, so a search could not be pinned to a publisher", name)
		}
		if len(owner.AccountID) != 12 {
			t.Errorf("%s: account id %q is not 12 digits", name, owner.AccountID)
		}
		for _, r := range owner.AccountID {
			if r < '0' || r > '9' {
				t.Errorf("%s: account id %q is not all digits", name, owner.AccountID)
				break
			}
		}
		if owner.DistroName == "" {
			t.Errorf("%s: no default distro", name)
		}
		if owner.NamePattern == nil {
			t.Errorf("%s: no name pattern, so every image from the account would match", name)
			continue
		}

		q, err := ParseQuery(name)
		if err != nil {
			t.Errorf("%s: its own name does not parse: %s", name, err)
			continue
		}
		pattern := q.NameFilter()
		if pattern == "" || pattern == "*" {
			t.Errorf("%s: pattern for the default query is %q, which selects everything the account publishes", name, pattern)
		}
	}
}

func TestSupportedOwnersIsSorted(t *testing.T) {
	got := SupportedOwners()
	if len(got) != len(Owners) {
		t.Fatalf("got %d owners, want %d", len(got), len(Owners))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("not sorted: %v — help text would reshuffle between runs", got)
		}
	}
}
