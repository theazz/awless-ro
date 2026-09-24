package awsimage

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// stubEC2 records every request it is given, so the tests can assert on what would
// have gone to AWS as well as on what comes back.
type stubEC2 struct {
	requests []*ec2.DescribeImagesInput
	pages    [][]ec2types.Image
	err      error
	calls    int
}

func (s *stubEC2) DescribeImages(_ context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	s.requests = append(s.requests, in)
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if len(s.pages) == 0 {
		return &ec2.DescribeImagesOutput{}, nil
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	out := &ec2.DescribeImagesOutput{Images: page}
	if len(s.pages) > 0 {
		out.NextToken = awssdk.String("more")
	}
	return out, nil
}

func ami(id, name, created string) ec2types.Image {
	return ec2types.Image{
		ImageId:            awssdk.String(id),
		Name:               awssdk.String(name),
		CreationDate:       awssdk.String(created),
		OwnerId:            awssdk.String("099720109477"),
		Architecture:       ec2types.ArchitectureValuesX8664,
		VirtualizationType: ec2types.VirtualizationTypeHvm,
		RootDeviceType:     ec2types.DeviceTypeEbs,
		Hypervisor:         ec2types.HypervisorTypeXen,
	}
}

// nameFixture is one image name, and where it came from.
//
// The fixtures in this package used to read "a", "b", "ami-1", which is why the tests
// had nothing to say when the resolver began returning al2023-ami-minimal- and
// suse-…-chost-byos- in place of the ordinary images: no fixture looked enough like a
// real name for the difference to exist. Names are now copied from a live account
// wherever possible, and where a name is constructed instead it says so, because a
// test resting on a guess about a naming convention is worth less than one resting on
// a name AWS returned.
type nameFixture struct {
	name string

	// ordinary marks the vendor's general-purpose line, the thing a person asking
	// for "the latest Ubuntu" means. Everything else is a specialised build that
	// must not be returned in its place.
	ordinary bool

	// observed records that this exact name was returned by DescribeImages against
	// account 123456789012 in eu-west-1 in September 2026. Constructed names are
	// shapes the vendor documents or has published elsewhere, kept so that an
	// expression loosened later is still caught.
	observed bool
}

var liveNames = map[string][]nameFixture{
	"canonical": {
		{name: "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20260923", ordinary: true, observed: true},
		// The storage-class segment changed once already, hvm-ssd to hvm-ssd-gp3,
		// which is what the wildcard in the middle of the pattern is for.
		{name: "ubuntu/images/hvm-ssd/ubuntu-noble-24.04-amd64-server-20260714", ordinary: true, observed: true},

		// Canonical publishes a great deal beside the server image, and all of it
		// carries the release and architecture the ordinary name does.
		{name: "ubuntu-pro-server/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-pro-server-20260923", observed: true},
		{name: "ubuntu-minimal/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-minimal-20260923", observed: true},
		{name: "ubuntu-eks/k8s_1.34/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20260923", observed: true},
		{name: "ubuntu/images-testing/hvm-ssd-gp3/ubuntu-noble-daily-amd64-server-20260923", observed: true},
	},
	"debian": {
		{name: "debian-13-amd64-20260914-2601", ordinary: true, observed: true},
		// Published four seconds after the ordinary image, and tracking the
		// backports kernel rather than the one the release ships.
		{name: "debian-13-backports-amd64-20260914-2601", observed: true},
	},
	"redhat": {
		{name: "RHEL-9.8.0_HVM-20260908-x86_64-0-Hourly2-GP3", ordinary: true, observed: true},
		// A release-day marker Red Hat adds occasionally.
		{name: "RHEL-9.8.0_HVM_GA-20260521-x86_64-0-Hourly2-GP3", observed: true},
		// The bring-your-own-subscription counterpart to Hourly2. Not published in
		// the region checked, but it is the distinction the expression turns on.
		{name: "RHEL-9.8.0_HVM-20260908-x86_64-0-Access2-GP3"},
	},
	"amazonlinux": {
		{name: "al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64", ordinary: true, observed: true},
		{name: "al2023-ami-2023.12.20260918.0-kernel-6.1-x86_64", ordinary: true, observed: true},
		// Minimal leaves out most of the tooling, including the AWS CLI, and is
		// published three minutes after the ordinary image.
		{name: "al2023-ami-minimal-2023.12.20260918.0-kernel-6.18-x86_64", observed: true},
	},
	"suselinux": {
		{name: "suse-sles-15-sp6-v20260919-hvm-ssd-x86_64", ordinary: true, observed: true},
		// byos expects a subscription of one's own, so the instance comes up
		// unregistered; chost is a container host rather than a general-purpose
		// server. This is the image the tool actually returned for
		// `search images suselinux --latest-id`, being two days newer.
		{name: "suse-sles-15-sp6-chost-byos-v20260921-hvm-ssd-x86_64", observed: true},
		{name: "suse-sles-15-sp5-sapcal-v20260922-hvm-ssd-x86_64", observed: true},
		// -ecs- sits in the middle, exactly where a glob cannot see it.
		{name: "suse-sles-15-sp7-v20260922-ecs-hvm-ssd-x86_64", observed: true},
	},
	"windows": {
		{name: "Windows_Server-2022-English-Full-Base-2026.09.17", ordinary: true, observed: true},
		// Nothing else was published under this pattern in the region checked: the
		// edition is already pinned by the glob, and the four names it returned were
		// all dates. These keep the tail of the expression honest anyway.
		{name: "Windows_Server-2022-English-Full-Base-2026.09.17-Beta"},
		{name: "Windows_Server-2022-English-Full-SQL_2022_Standard-2026.09.17"},
	},
}

func ordinaryNames(owner string) []string {
	var out []string
	for _, f := range liveNames[owner] {
		if f.ordinary {
			out = append(out, f.name)
		}
	}
	return out
}

func specialisedNames(owner string) []string {
	var out []string
	for _, f := range liveNames[owner] {
		if !f.ordinary {
			out = append(out, f.name)
		}
	}
	return out
}

// ubuntuName builds a Canonical name of the current form, for the tests that need
// several images and do not care which release.
func ubuntuName(variant, date string) string {
	version := map[string]string{"noble": "24.04", "jammy": "22.04"}[variant]
	if version == "" {
		version = "24.04"
	}
	return fmt.Sprintf("ubuntu/images/hvm-ssd-gp3/ubuntu-%s-%s-amd64-server-%s", variant, version, date)
}

func filterValues(in *ec2.DescribeImagesInput, name string) []string {
	for _, f := range in.Filters {
		if awssdk.ToString(f.Name) == name {
			return f.Values
		}
	}
	return nil
}

// The account is pinned on every request. This is the test that matters most in this
// package: anyone may publish a public AMI under any name, so a search that does not
// name an account returns whatever a stranger has called "ubuntu-noble-latest". The
// whoAMI name confusion attack turned that into code execution for tooling that fed
// the result to run-instances.
func TestEverySearchPinsThePublishingAccount(t *testing.T) {
	for _, owner := range SupportedOwners() {
		q, err := ParseQuery(owner)
		if err != nil {
			t.Fatalf("%s: %s", owner, err)
		}

		name := ordinaryNames(owner)[0]
		stub := &stubEC2{pages: [][]ec2types.Image{{ami("ami-1", name, "2026-01-01T00:00:00.000Z")}}}
		if _, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q}); err != nil {
			t.Fatalf("%s: %s", owner, err)
		}

		if len(stub.requests) != 1 {
			t.Fatalf("%s: got %d requests, want 1", owner, len(stub.requests))
		}
		got := stub.requests[0].Owners
		if len(got) != 1 || got[0] == "" {
			t.Errorf("%s: Owners = %v, want exactly one non-empty account id", owner, got)
		}
		if got[0] != Owners[owner].AccountID {
			t.Errorf("%s: Owners = %v, want %q", owner, got, Owners[owner].AccountID)
		}
	}
}

// A search with a name pattern and no account is refused before any request is made.
func TestNameWithoutAccountIsRefused(t *testing.T) {
	stub := &stubEC2{}

	_, err := NewResolver(stub).Resolve(context.Background(), Search{NamePattern: "ubuntu-noble-*"})
	if !errors.Is(err, ErrNoOwner) {
		t.Fatalf("got %v, want %v", err, ErrNoOwner)
	}
	if stub.calls != 0 {
		t.Error("a request was sent without an account id")
	}
}

func TestResolveFiltersAndSorting(t *testing.T) {
	q, err := ParseQuery("canonical:ubuntu:jammy:arm64:hvm:ebs")
	if err != nil {
		t.Fatal(err)
	}

	stub := &stubEC2{pages: [][]ec2types.Image{{
		ami("ami-old", "ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-arm64-server-20240101", "2024-01-01T00:00:00.000Z"),
		ami("ami-new", "ubuntu/images/hvm-ssd-gp3/ubuntu-jammy-22.04-arm64-server-20260101", "2026-01-01T00:00:00.000Z"),
		ami("ami-mid", "ubuntu/images/hvm-ssd-gp3/ubuntu-jammy-22.04-arm64-server-20250101", "2025-01-01T00:00:00.000Z"),
	}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatal(err)
	}

	// Newest first: --latest-id is the first element.
	want := []string{"ami-new", "ami-mid", "ami-old"}
	for i, id := range want {
		if images[i].ID != id {
			t.Errorf("position %d: got %s, want %s", i, images[i].ID, id)
		}
	}

	in := stub.requests[0]
	for _, tc := range []struct{ filter, want string }{
		{"state", "available"},
		{"is-public", "true"},
		{"architecture", "arm64"},
		{"virtualization-type", "hvm"},
		{"root-device-type", "ebs"},
	} {
		got := filterValues(in, tc.filter)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("filter %s = %v, want [%s]", tc.filter, got, tc.want)
		}
	}

	// Matching happens in the API rather than over every image the vendor ever
	// published; Canonical alone has tens of thousands.
	name := filterValues(in, "name")
	if len(name) != 1 || !strings.Contains(name[0], "jammy") {
		t.Errorf("name filter = %v, want a pattern containing the variant", name)
	}
}

// The wildcard in Canonical's pattern is there to survive the storage-class segment
// changing, which it already did once: hvm-ssd became hvm-ssd-gp3.
func TestCanonicalPatternSpansTheStorageClassChange(t *testing.T) {
	q, err := ParseQuery("canonical")
	if err != nil {
		t.Fatal(err)
	}
	pattern := q.NameFilter()

	for _, name := range []string{
		"ubuntu/images/hvm-ssd/ubuntu-noble-24.04-amd64-server-20240101",
		"ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20260714",
	} {
		if !globMatch(pattern, name) {
			t.Errorf("pattern %q does not match %q", pattern, name)
		}
	}
	if globMatch(pattern, "ubuntu/images/hvm-ssd-gp3/ubuntu-jammy-22.04-amd64-server-20260714") {
		t.Errorf("pattern %q should not match a different release", pattern)
	}
}

// globMatch approximates the EC2 name filter, which supports * and ?, so that a test
// can say something about a pattern without calling AWS.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, part := range parts[1 : len(parts)-1] {
		idx := strings.Index(s, part)
		if idx < 0 {
			return false
		}
		s = s[idx+len(part):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

func TestResolveFollowsPages(t *testing.T) {
	q, _ := ParseQuery("canonical")

	stub := &stubEC2{pages: [][]ec2types.Image{
		{ami("ami-1", ubuntuName("noble", "20240101"), "2024-01-01T00:00:00.000Z")},
		{ami("ami-2", ubuntuName("noble", "20250101"), "2025-01-01T00:00:00.000Z")},
		{ami("ami-3", ubuntuName("noble", "20260101"), "2026-01-01T00:00:00.000Z")},
	}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 3 {
		t.Fatalf("got %d images, want 3 — a paginated response was truncated", len(images))
	}
	if images[0].ID != "ami-3" {
		t.Errorf("newest image is %s, want ami-3", images[0].ID)
	}
}

// Matching nothing is an error, not an empty list: the usual cause is a vendor naming
// convention that has moved on, and the message has to carry enough to act on.
func TestNoMatchExplainsItself(t *testing.T) {
	q, _ := ParseQuery("canonical")

	_, err := NewResolver(&stubEC2{}).Resolve(context.Background(), Search{Query: &q})
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v, want %v", err, ErrNoMatch)
	}
	for _, want := range []string{"099720109477", "noble", "x86_64", "hvm", "ebs", "names images"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should mention %q, got:\n%s", want, err)
		}
	}
}

func TestResultsAreCachedWithinTheProcess(t *testing.T) {
	q, _ := ParseQuery("canonical")
	stub := &stubEC2{pages: [][]ec2types.Image{{ami("ami-1", ubuntuName("noble", "20260101"), "2026-01-01T00:00:00.000Z")}}}

	resolver := NewResolver(stub)
	for range 3 {
		if _, err := resolver.Resolve(context.Background(), Search{Query: &q}); err != nil {
			t.Fatal(err)
		}
	}
	if stub.calls != 1 {
		t.Errorf("AWS was called %d times for the same query, want 1", stub.calls)
	}

	// A different query is a different question.
	other, _ := ParseQuery("canonical:ubuntu:jammy")
	stub.pages = [][]ec2types.Image{{ami("ami-2", ubuntuName("jammy", "20260101"), "2026-01-01T00:00:00.000Z")}}
	if _, err := resolver.Resolve(context.Background(), Search{Query: &other}); err != nil {
		t.Fatal(err)
	}
	if stub.calls != 2 {
		t.Errorf("AWS was called %d times, want 2", stub.calls)
	}
}

// A failed search is not cached, or a transient error would poison the rest of the
// process.
func TestFailuresAreNotCached(t *testing.T) {
	q, _ := ParseQuery("canonical")
	boom := errors.New("throttled")
	stub := &stubEC2{err: boom}

	resolver := NewResolver(stub)
	if _, err := resolver.Resolve(context.Background(), Search{Query: &q}); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}

	stub.err = nil
	stub.pages = [][]ec2types.Image{{ami("ami-1", ubuntuName("noble", "20260101"), "2026-01-01T00:00:00.000Z")}}
	images, err := resolver.Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatalf("the second attempt should succeed: %s", err)
	}
	if len(images) != 1 {
		t.Errorf("got %d images, want 1", len(images))
	}
}

// An unparseable CreationDate must not sort to the front and be reported as the
// latest image.
func TestImagesWithoutADateSortLast(t *testing.T) {
	q, _ := ParseQuery("canonical")

	stub := &stubEC2{pages: [][]ec2types.Image{{
		ami("ami-nodate", ubuntuName("noble", "20260101"), "not a date"),
		ami("ami-dated", ubuntuName("noble", "20200101"), "2020-01-01T00:00:00.000Z"),
	}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatal(err)
	}
	if images[0].ID != "ami-dated" {
		t.Errorf("latest image is %s, want ami-dated: an image with no date must not win", images[0].ID)
	}
}

func TestSearchByAccountAndName(t *testing.T) {
	stub := &stubEC2{pages: [][]ec2types.Image{{ami("ami-1", "my-own-image", "2026-01-01T00:00:00.000Z")}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{
		AccountID:   "123456789012",
		NamePattern: "my-own-*",
		Arch:        "arm64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("got %d images, want 1", len(images))
	}

	in := stub.requests[0]
	if got := in.Owners; len(got) != 1 || got[0] != "123456789012" {
		t.Errorf("Owners = %v, want [123456789012]", got)
	}
	if got := filterValues(in, "name"); len(got) != 1 || got[0] != "my-own-*" {
		t.Errorf("name filter = %v, want [my-own-*]", got)
	}
	if got := filterValues(in, "architecture"); len(got) != 1 || got[0] != "arm64" {
		t.Errorf("architecture filter = %v, want [arm64]", got)
	}
}

// Defaults still apply to a raw search, so that omitting them does not produce a
// request with empty filter values.
func TestRawSearchUsesDefaults(t *testing.T) {
	stub := &stubEC2{pages: [][]ec2types.Image{{ami("ami-1", "x", "2026-01-01T00:00:00.000Z")}}}

	if _, err := NewResolver(stub).Resolve(context.Background(), Search{AccountID: "123456789012"}); err != nil {
		t.Fatal(err)
	}

	in := stub.requests[0]
	for _, tc := range []struct{ filter, want string }{
		{"name", "*"},
		{"architecture", defaultArch},
		{"virtualization-type", defaultVirt},
		{"root-device-type", defaultStore},
	} {
		got := filterValues(in, tc.filter)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("filter %s = %v, want [%s]", tc.filter, got, tc.want)
		}
	}
}

func TestResolverIsSafeForConcurrentUse(t *testing.T) {
	q, _ := ParseQuery("canonical")
	name := ubuntuName("noble", "20260101")
	stub := &stubEC2{pages: [][]ec2types.Image{
		{ami("ami-1", name, "2026-01-01T00:00:00.000Z")},
		{ami("ami-1", name, "2026-01-01T00:00:00.000Z")},
		{ami("ami-1", name, "2026-01-01T00:00:00.000Z")},
		{ami("ami-1", name, "2026-01-01T00:00:00.000Z")},
	}}
	// The stub itself is not concurrency-safe, so serialise the calls through it and
	// let the race detector look at the resolver's own state.
	resolver := NewResolver(&lockedEC2{inner: stub})

	done := make(chan struct{})
	for range 4 {
		go func() {
			defer func() { done <- struct{}{} }()
			resolver.Resolve(context.Background(), Search{Query: &q})
		}()
	}
	for range 4 {
		<-done
	}
}

type lockedEC2 struct {
	mu    sync.Mutex
	inner DescribeImagesAPI
}

func (l *lockedEC2) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inner.DescribeImages(ctx, in, opts...)
}

// The defect this guards against, in the form it was found in: a specialised build
// published a couple of days after the ordinary one wins on date and is returned as
// the vendor's latest image, with nothing in the id to say so.
//
// Against a live account in September 2026 the tool answered
// suse-sles-15-sp6-chost-byos-… for `search images suselinux --latest-id`. byos means
// the instance comes up without a subscription; chost is a container host rather than
// a general-purpose server.
func TestSpecialisedBuildsDoNotWinOnDate(t *testing.T) {
	for _, owner := range SupportedOwners() {
		ordinary, specialised := ordinaryNames(owner), specialisedNames(owner)
		if len(specialised) == 0 {
			t.Fatalf("%s: no specialised name recorded, so this test says nothing about it", owner)
		}

		q, err := ParseQuery(owner)
		if err != nil {
			t.Fatalf("%s: %s", owner, err)
		}

		// Every specialised build newer than the ordinary one, which is the
		// arrangement that produced wrong answers.
		var page []ec2types.Image
		page = append(page, ami("ami-ordinary", ordinary[0], "2026-09-19T00:00:00.000Z"))
		for i, name := range specialised {
			page = append(page, ami(fmt.Sprintf("ami-special-%d", i), name, "2026-09-21T00:00:00.000Z"))
		}

		images, err := NewResolver(&stubEC2{pages: [][]ec2types.Image{page}}).
			Resolve(context.Background(), Search{Query: &q})
		if err != nil {
			t.Fatalf("%s: %s", owner, err)
		}

		if images[0].ID != "ami-ordinary" {
			t.Errorf("%s: latest is %s (%s), want the ordinary image %s",
				owner, images[0].ID, images[0].Name, ordinary[0])
		}
		for _, img := range images {
			if img.ID != "ami-ordinary" {
				t.Errorf("%s: %s is in the results and should have been set aside", owner, img.Name)
			}
		}
	}
}

// Every name recorded from the live account is classified the way it was observed.
// Kept apart from the test above so that a mistake in an expression is reported per
// name rather than as one owner failing.
func TestMainstreamExpressionsAgreeWithObservedNames(t *testing.T) {
	for _, owner := range SupportedOwners() {
		q, err := ParseQuery(owner)
		if err != nil {
			t.Fatalf("%s: %s", owner, err)
		}
		re, err := q.MainstreamFilter()
		if err != nil {
			t.Fatalf("%s: %s", owner, err)
		}
		if re == nil {
			t.Errorf("%s: declares no mainstream expression, so specialised builds can win on date", owner)
			continue
		}

		for _, f := range liveNames[owner] {
			got := re.MatchString(f.name)
			if got == f.ordinary {
				continue
			}
			provenance := "constructed"
			if f.observed {
				provenance = "observed in a live account"
			}
			if f.ordinary {
				t.Errorf("%s: %q is the ordinary image (%s) and does not match %q",
					owner, f.name, provenance, re)
			} else {
				t.Errorf("%s: %q is a specialised build (%s) and matches %q",
					owner, f.name, provenance, re)
			}
		}
	}
}

// Each owner needs at least one real name on each side, or the expression above it was
// written against nothing.
func TestEveryOwnerHasObservedNames(t *testing.T) {
	for _, owner := range SupportedOwners() {
		fixtures := liveNames[owner]
		if len(fixtures) == 0 {
			t.Errorf("%s: no names recorded", owner)
			continue
		}

		var ordinary, specialised int
		for _, f := range fixtures {
			if !f.observed {
				continue
			}
			if f.ordinary {
				ordinary++
			} else {
				specialised++
			}
		}
		if ordinary == 0 {
			t.Errorf("%s: no observed name for the ordinary line", owner)
		}
		// windows is the exception and says why at the fixture: the glob pins the
		// edition, and the account published nothing else under it.
		if specialised == 0 && owner != "windows" {
			t.Errorf("%s: no observed specialised build, so the expression is only tested "+
				"against names someone thought up", owner)
		}
	}
}

// The storage class is left open in the RHEL expression because it is not always GP3:
// the RHEL 7 era images end in GP2. Asking for an older release must not come back
// empty on that account.
func TestRedhatExpressionSpansTheStorageClassChange(t *testing.T) {
	q, err := ParseQuery("redhat::7")
	if err != nil {
		t.Fatal(err)
	}
	re, err := q.MainstreamFilter()
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("RHEL-7.9_HVM-20230208-x86_64-0-Hourly2-GP2") {
		t.Errorf("%q does not match a GP2-era RHEL name", re)
	}
}

// The previous Amazon Linux generation is named differently enough to need its own
// expression, and it has a minimal build too.
func TestAmazonLinux2IsRecognisedSeparately(t *testing.T) {
	q, err := ParseQuery("amazonlinux:amzn2")
	if err != nil {
		t.Fatal(err)
	}
	re, err := q.MainstreamFilter()
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"amzn2-ami-kernel-5.10-hvm-2.0.20260923.0-x86_64-gp2",
		"amzn2-ami-hvm-2.0.20260923.0-x86_64-ebs",
	} {
		if !re.MatchString(name) {
			t.Errorf("%q is an ordinary amzn2 image and does not match %q", name, re)
		}
	}
	for _, name := range []string{
		"amzn2-ami-minimal-hvm-2.0.20260923.0-x86_64-gp2",
		"amzn2-ami-minimal-kernel-5.10-hvm-2.0.20260923.0-x86_64-gp2",
		"amzn2-ami-minimal-selinux-enforcing-hvm-2.0.20260923.0-x86_64-gp2",
	} {
		if re.MatchString(name) {
			t.Errorf("%q is a specialised amzn2 build and matches %q", name, re)
		}
	}
}

// Dropping everything is reported as its own situation. The user needs to know the
// account is publishing but the naming has moved on, because the fix is different
// from the fix for nothing being published at all — and both need the escape hatch.
func TestEverythingSetAsideSaysSo(t *testing.T) {
	q, _ := ParseQuery("suselinux")

	stub := &stubEC2{pages: [][]ec2types.Image{{
		ami("ami-1", "suse-sles-15-sp6-chost-byos-v20260921-hvm-ssd-x86_64", "2026-09-21T00:00:00.000Z"),
		ami("ami-2", "suse-sles-15-sp5-sapcal-v20260922-hvm-ssd-x86_64", "2026-09-22T00:00:00.000Z"),
	}}}

	_, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v, want %v", err, ErrNoMatch)
	}
	for _, want := range []string{
		"2 image(s)", // the account is publishing
		"--owner",    // and here is how to look at them anyway
		"013907871322",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should mention %q, got:\n%s", want, err)
		}
	}
}

// A search by account and name is the escape hatch, so it is not filtered: the caller
// has said which names they want.
func TestRawSearchIsNotRestrictedToMainstreamNames(t *testing.T) {
	stub := &stubEC2{pages: [][]ec2types.Image{{
		ami("ami-1", "suse-sles-15-sp6-chost-byos-v20260921-hvm-ssd-x86_64", "2026-09-21T00:00:00.000Z"),
	}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{
		AccountID:   "013907871322",
		NamePattern: "suse-sles-15-*",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("got %d images, want 1: --owner with --name must not be filtered", len(images))
	}
}

// One release published as several images left the answer to whatever order the API
// returned. Amazon Linux ships a kernel 6.1, a 6.12 and a 6.18 build of the same
// version, seconds apart, so which one `--latest-id` printed depended on the response
// order — no answer at all for a command whose entire output is one id.
func TestEqualDatesAreBrokenDeterministically(t *testing.T) {
	q, _ := ParseQuery("amazonlinux")
	const created = "2026-09-18T04:42:42.000Z"

	forward := []ec2types.Image{
		ami("ami-61", "al2023-ami-2023.12.20260918.0-kernel-6.1-x86_64", created),
		ami("ami-612", "al2023-ami-2023.12.20260918.0-kernel-6.12-x86_64", created),
		ami("ami-618", "al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64", created),
	}
	reversed := []ec2types.Image{forward[2], forward[1], forward[0]}

	var first []string
	for _, page := range [][]ec2types.Image{forward, reversed} {
		images, err := NewResolver(&stubEC2{pages: [][]ec2types.Image{page}}).
			Resolve(context.Background(), Search{Query: &q})
		if err != nil {
			t.Fatal(err)
		}
		first = append(first, images[0].ID)
	}

	if first[0] != first[1] {
		t.Errorf("the answer depends on the order AWS returned images: %v", first)
	}
	// Descending by name, which prefers the newer kernel of a release. That is also
	// what AWS's own al2023-ami-kernel-default pointer resolved to in September 2026.
	if first[0] != "ami-618" {
		t.Errorf("latest is %s, want ami-618: the highest kernel of the release", first[0])
	}
}

// The situation as it was found live, which the equal-dates test above does not
// reproduce: the kernel builds of one Amazon Linux release are published seconds
// apart, and in September 2026 the 6.1 build came one second after the 6.18 build. So
// the newest-by-date answer was kernel 6.1, while AWS's own al2023-ami-kernel-default
// pointer named the 6.18 image.
func TestOneSecondApartWithinAReleaseDoesNotDecideIt(t *testing.T) {
	q, _ := ParseQuery("amazonlinux")

	// Dates exactly as observed.
	stub := &stubEC2{pages: [][]ec2types.Image{{
		ami("ami-618", "al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64", "2026-09-18T04:42:42.000Z"),
		ami("ami-612", "al2023-ami-2023.12.20260918.0-kernel-6.12-x86_64", "2026-09-18T04:42:42.000Z"),
		ami("ami-61", "al2023-ami-2023.12.20260918.0-kernel-6.1-x86_64", "2026-09-18T04:42:43.000Z"),
	}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatal(err)
	}
	if images[0].ID != "ami-618" {
		t.Errorf("latest is %s (%s), want ami-618: one second of publication order "+
			"must not outrank the kernel version within a release", images[0].ID, images[0].Name)
	}
}

// Grouping by release must not let an older release jump a newer one, which is what
// would happen if the group's date were taken from its lowest member.
func TestAnOlderReleaseStaysOlder(t *testing.T) {
	q, _ := ParseQuery("amazonlinux")

	stub := &stubEC2{pages: [][]ec2types.Image{{
		// The older release, but with the higher kernel number.
		ami("ami-old-618", "al2023-ami-2023.11.20260801.0-kernel-6.18-x86_64", "2026-08-01T00:00:00.000Z"),
		// The current release, whose only build is an older kernel.
		ami("ami-new-61", "al2023-ami-2023.12.20260918.0-kernel-6.1-x86_64", "2026-09-18T00:00:00.000Z"),
	}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatal(err)
	}
	if images[0].ID != "ami-new-61" {
		t.Errorf("latest is %s (%s), want ami-new-61: the newer release wins even with "+
			"a lower kernel", images[0].ID, images[0].Name)
	}
}

// A release is as new as its newest image. Taking the date from its oldest member
// instead would sink the whole release below a neighbour, which matters when a vendor
// adds a build to a release weeks after the rest of it.
func TestAReleaseIsDatedByItsNewestImage(t *testing.T) {
	q, _ := ParseQuery("amazonlinux")

	stub := &stubEC2{pages: [][]ec2types.Image{{
		// The current release, whose kernel 6.1 build lags six weeks behind.
		ami("ami-current-618", "al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64", "2026-09-18T00:00:00.000Z"),
		ami("ami-current-61", "al2023-ami-2023.12.20260918.0-kernel-6.1-x86_64", "2026-08-01T00:00:00.000Z"),
		// The previous release, published between the two above.
		ami("ami-previous", "al2023-ami-2023.11.20260910.0-kernel-6.18-x86_64", "2026-09-10T00:00:00.000Z"),
	}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatal(err)
	}
	if images[0].ID != "ami-current-618" {
		t.Errorf("latest is %s (%s), want ami-current-618: a release ranks by its newest "+
			"image, not its oldest", images[0].ID, images[0].Name)
	}
}

// Release grouping is per vendor, and a vendor that publishes one image per release
// declares none. Canonical is the one to check, because its names put the storage
// class before the date, so ordering its images by name would return the oldest.
func TestVendorsWithoutReleaseGroupingAreOrderedByDate(t *testing.T) {
	q, _ := ParseQuery("canonical")
	if Owners["canonical"].Release != nil {
		t.Fatal("canonical declares release grouping, which this test assumes it does not")
	}

	stub := &stubEC2{pages: [][]ec2types.Image{{
		// The older image sorts first by name: '/' outranks '-', so hvm-ssd/ beats
		// hvm-ssd-gp3/.
		ami("ami-old", "ubuntu/images/hvm-ssd/ubuntu-noble-24.04-amd64-server-20260714", "2026-07-14T00:00:00.000Z"),
		ami("ami-new", "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20260923", "2026-09-23T00:00:00.000Z"),
	}}}

	images, err := NewResolver(stub).Resolve(context.Background(), Search{Query: &q})
	if err != nil {
		t.Fatal(err)
	}
	if images[0].ID != "ami-new" {
		t.Errorf("latest is %s (%s), want ami-new", images[0].ID, images[0].Name)
	}
}

// The release stamp has to be found in both generations' naming, and nowhere else.
func TestAmazonReleaseStamps(t *testing.T) {
	release := Owners["amazonlinux"].Release

	cases := map[string]string{
		"al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64":                  "2023.12.20260918.0",
		"al2023-ami-2023.12.20260918.0-kernel-6.1-x86_64":                   "2023.12.20260918.0",
		"al2023-ami-minimal-2023.12.20260918.0-kernel-6.18-x86_64":          "2023.12.20260918.0",
		"amzn2-ami-kernel-5.10-hvm-2.0.20260923.0-x86_64-gp2":               "2.0.20260923.0",
		"amzn2-ami-hvm-2.0.20260923.0-x86_64-ebs":                           "2.0.20260923.0",
		"amzn2-ami-minimal-selinux-enforcing-hvm-2.0.20260923.0-x86_64-gp2": "2.0.20260923.0",
		// Not an Amazon Linux name at all.
		"debian-13-amd64-20260914-2601": "",
	}

	for name, want := range cases {
		if got := release(name); got != want {
			t.Errorf("%s: release = %q, want %q", name, got, want)
		}
	}

	// The kernel builds of one release must land in the same group, or the grouping
	// does nothing.
	a := release("al2023-ami-2023.12.20260918.0-kernel-6.18-x86_64")
	b := release("al2023-ami-2023.12.20260918.0-kernel-6.1-x86_64")
	if a == "" || a != b {
		t.Errorf("the kernel builds of one release are in different groups: %q and %q", a, b)
	}
}

// A query token reaching the expression as a regular expression metacharacter must
// not panic or match more than it says.
func TestQueryTokensAreQuotedIntoTheExpression(t *testing.T) {
	for _, in := range []string{`redhat::9.`, `debian::1+`, `canonical::(noble)`, `suselinux::15|16`} {
		q, err := ParseQuery(in)
		if err != nil {
			t.Fatalf("%s: %s", in, err)
		}
		re, err := q.MainstreamFilter()
		if err != nil {
			t.Fatalf("%s: %s", in, err)
		}
		if re == nil {
			t.Fatalf("%s: no expression", in)
		}
		if !strings.Contains(re.String(), regexp.QuoteMeta(q.Variant)) {
			t.Errorf("%s: variant %q is not quoted into %q", in, q.Variant, re)
		}
	}

	// The dot in a RHEL minor version is a literal, so it must not match any
	// character: RHEL-9x… is not RHEL-9.…
	q, _ := ParseQuery("redhat::9.8")
	re, _ := q.MainstreamFilter()
	if re.MatchString("RHEL-9x8.0_HVM-20260908-x86_64-0-Hourly2-GP3") {
		t.Errorf("%q treated the dot in the variant as a wildcard", re)
	}
}
