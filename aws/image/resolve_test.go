package awsimage

import (
	"context"
	"errors"
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

		stub := &stubEC2{pages: [][]ec2types.Image{{ami("ami-1", "n", "2026-01-01T00:00:00.000Z")}}}
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
		{ami("ami-1", "a", "2024-01-01T00:00:00.000Z")},
		{ami("ami-2", "b", "2025-01-01T00:00:00.000Z")},
		{ami("ami-3", "c", "2026-01-01T00:00:00.000Z")},
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
	stub := &stubEC2{pages: [][]ec2types.Image{{ami("ami-1", "a", "2026-01-01T00:00:00.000Z")}}}

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
	stub.pages = [][]ec2types.Image{{ami("ami-2", "b", "2026-01-01T00:00:00.000Z")}}
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
	stub.pages = [][]ec2types.Image{{ami("ami-1", "a", "2026-01-01T00:00:00.000Z")}}
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
		ami("ami-nodate", "a", "not a date"),
		ami("ami-dated", "b", "2020-01-01T00:00:00.000Z"),
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
	stub := &stubEC2{pages: [][]ec2types.Image{
		{ami("ami-1", "a", "2026-01-01T00:00:00.000Z")},
		{ami("ami-1", "a", "2026-01-01T00:00:00.000Z")},
		{ami("ami-1", "a", "2026-01-01T00:00:00.000Z")},
		{ami("ami-1", "a", "2026-01-01T00:00:00.000Z")},
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
