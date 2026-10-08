package awsservices

import (
	"context"
	"reflect"
	"sort"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"

	"github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/match"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/fetch"
)

// Regression tests for wallix/awless#287: `list users --tag k=v` found nothing
// because IAM tags never reached the graph.

// taggedIamMock answers ListUsers and GetAccountAuthorizationDetails from canned
// data, honouring the entity Filter the way IAM does, so each per-type
// pagination the fetcher runs gets only its own entities. Local to this file so
// the shared mockIam stays as the other graph tests expect it.
type taggedIamMock struct {
	*mockIam
	listUsers []iamtypes.User
	details   iam.GetAccountAuthorizationDetailsOutput
}

func (m *taggedIamMock) ListUsers(_ context.Context, _ *iam.ListUsersInput, _ ...func(*iam.Options)) (*iam.ListUsersOutput, error) {
	return &iam.ListUsersOutput{Users: m.listUsers}, nil
}

func (m *taggedIamMock) GetAccountAuthorizationDetails(_ context.Context, in *iam.GetAccountAuthorizationDetailsInput, _ ...func(*iam.Options)) (*iam.GetAccountAuthorizationDetailsOutput, error) {
	out := &iam.GetAccountAuthorizationDetailsOutput{}
	for _, e := range in.Filter {
		switch e {
		case iamtypes.EntityTypeUser:
			out.UserDetailList = m.details.UserDetailList
		case iamtypes.EntityTypeGroup:
			out.GroupDetailList = m.details.GroupDetailList
		case iamtypes.EntityTypeRole:
			out.RoleDetailList = m.details.RoleDetailList
		case iamtypes.EntityTypeLocalManagedPolicy:
			out.Policies = append(out.Policies, m.details.Policies...)
		}
	}
	return out, nil
}

func iamTags(kv ...string) []iamtypes.Tag {
	tags := []iamtypes.Tag{}
	for i := 0; i+1 < len(kv); i += 2 {
		tags = append(tags, iamtypes.Tag{Key: awssdk.String(kv[i]), Value: awssdk.String(kv[i+1])})
	}
	return tags
}

func newTaggedIamMock() *taggedIamMock {
	type user struct {
		id, name string
		tags     []iamtypes.Tag
	}
	users := []user{
		{"usr-tagged", "tagged-user", iamTags("team", "platform-engineering")},
		{"usr-untagged", "untagged-user", nil},             // what AWS returns for no tags
		{"usr-empty", "empty-tags-user", []iamtypes.Tag{}}, // non-nil, zero length
		{"usr-multi", "multi-tag-user", iamTags("team", "data", "env", "prod", "owner", "")},
	}
	m := &taggedIamMock{mockIam: &mockIam{}}
	for _, u := range users {
		arn := awssdk.String("arn:aws:iam::123456789012:user/" + u.name)
		// Both shapes carry tags; the graph merges the two resources by id.
		m.listUsers = append(m.listUsers, iamtypes.User{UserId: awssdk.String(u.id), UserName: awssdk.String(u.name), Arn: arn, Tags: u.tags})
		m.details.UserDetailList = append(m.details.UserDetailList, iamtypes.UserDetail{UserId: awssdk.String(u.id), UserName: awssdk.String(u.name), Arn: arn, Tags: u.tags})
	}
	m.details.RoleDetailList = []iamtypes.RoleDetail{
		{RoleId: awssdk.String("rol-tagged"), RoleName: awssdk.String("tagged-role"), Tags: iamTags("team", "platform-engineering")},
		{RoleId: awssdk.String("rol-untagged"), RoleName: awssdk.String("untagged-role")},
	}
	m.details.GroupDetailList = []iamtypes.GroupDetail{
		{GroupId: awssdk.String("grp-1"), GroupName: awssdk.String("platform-engineering")},
	}
	m.details.Policies = []iamtypes.ManagedPolicyDetail{
		{PolicyId: awssdk.String("pol-1"), PolicyName: awssdk.String("platform-engineering"), Arn: awssdk.String("arn:aws:iam::123456789012:policy/platform-engineering")},
	}
	return m
}

func newAccessForTags(m *taggedIamMock) *Access {
	return &Access{
		IamAPI:  m,
		region:  "global",
		profile: "default",
		fetcher: fetch.NewFetcher(awsfetch.BuildAccessFetchFuncs(awsfetch.NewConfig(&awsfetch.AWSAPI{Iam: m}))),
	}
}

func fetchAccessByID(t *testing.T, resourceType string) map[string]cloud.Resource {
	t.Helper()
	g, err := newAccessForTags(newTaggedIamMock()).FetchByType(context.WithValue(context.Background(), "force", true), resourceType)
	if err != nil {
		t.Fatalf("fetch %s: %s", resourceType, err)
	}
	found, err := g.Find(cloud.NewQuery(resourceType))
	if err != nil {
		t.Fatalf("find %s: %s", resourceType, err)
	}
	out := make(map[string]cloud.Resource)
	for _, r := range found {
		out[r.Id()] = r
	}
	return out
}

// The three predicates `--tag`, `--tag-key` and `--tag-value` build.
var platformMatchers = map[string]cloud.Matcher{
	"--tag team=platform-engineering":  match.Tag("team", "platform-engineering"),
	"--tag-key team":                   match.TagKey("team"),
	"--tag-value platform-engineering": match.TagValue("platform-engineering"),
}

func assertMatchesAll(t *testing.T, r cloud.Resource, want bool) {
	t.Helper()
	for flag, m := range platformMatchers {
		if got := m.Match(r); got != want {
			t.Errorf("%s: %s matched = %t, want %t (Tags = %#v)", r.Id(), flag, got, want, r.Properties()[properties.Tags])
		}
	}
}

func TestIamUserTagsReachTheGraph(t *testing.T) {
	users := fetchAccessByID(t, cloud.User)

	tagged, ok := users["usr-tagged"]
	if !ok {
		t.Fatalf("tagged user not fetched; got %v", users)
	}
	// cloud/match/matchers.go:121 (and :145, :169) type-assert exactly this shape.
	tags, ok := tagged.Properties()[properties.Tags].([]string)
	if !ok {
		t.Fatalf("Tags is %T, want []string of k=v", tagged.Properties()[properties.Tags])
	}
	if want := []string{"team=platform-engineering"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("Tags = %#v, want %#v", tags, want)
	}
	assertMatchesAll(t, tagged, true)

	untagged := users["usr-untagged"]
	if v, present := untagged.Properties()[properties.Tags]; present {
		t.Errorf("untagged user has a Tags key (%#v), want none at all", v)
	}
	assertMatchesAll(t, untagged, false)

	// A non-nil empty slice: no k=v pair survives into the graph, and nothing matches.
	assertMatchesAll(t, users["usr-empty"], false)

	multi := users["usr-multi"]
	multiTags, _ := multi.Properties()[properties.Tags].([]string)
	multiTags = append([]string(nil), multiTags...)
	sort.Strings(multiTags)
	if want := []string{"env=prod", "owner=", "team=data"}; !reflect.DeepEqual(multiTags, want) {
		t.Errorf("multi-tag user: Tags = %#v, want %#v", multiTags, want)
	}
	if !match.Tag("env", "prod").Match(multi) || !match.TagKey("owner").Match(multi) {
		t.Error("multi-tag user: --tag env=prod or --tag-key owner (empty value) did not match")
	}
	// team=data: the key matches, the platform-engineering pair and value do not.
	if match.Tag("team", "platform-engineering").Match(multi) || match.TagValue("platform-engineering").Match(multi) {
		t.Error("multi-tag user (team=data) matched team=platform-engineering")
	}
}

// The empty-slice case before the graph: straight out of the real user fetch
// function, a non-nil empty Tags is a present key holding a zero-length
// []string — the same as for every EC2 type — and the tag matchers iterate it
// to nothing. (Marshalling writes no triple for an empty list, so after the
// graph the key is simply gone; TestIamUserTagsReachTheGraph covers that side.)
func TestIamUserWithEmptyTagsSliceBeforeTheGraph(t *testing.T) {
	m := newTaggedIamMock()
	funcs := awsfetch.BuildAccessFetchFuncs(awsfetch.NewConfig(&awsfetch.AWSAPI{Iam: m}))
	f := fetch.NewFetcher(funcs)
	resources, _, err := funcs["user"](context.WithValue(context.Background(), "force", true), f)
	if err != nil {
		t.Fatal(err)
	}
	var checked int
	for _, r := range resources {
		if r.Id() != "usr-empty" {
			continue
		}
		checked++
		v, present := r.Properties()[properties.Tags]
		if !present {
			t.Fatal("empty-tags user: Tags key absent, want present")
		}
		tags, ok := v.([]string)
		if !ok || len(tags) != 0 {
			t.Fatalf("empty-tags user: Tags = %#v, want a zero-length []string", v)
		}
		assertMatchesAll(t, r, false)
	}
	if checked == 0 {
		t.Fatal("empty-tags user not produced by the user fetch function")
	}
}

func TestIamRoleTagsReachTheGraph(t *testing.T) {
	roles := fetchAccessByID(t, cloud.Role)
	assertMatchesAll(t, roles["rol-tagged"], true)
	assertMatchesAll(t, roles["rol-untagged"], false)
}

func TestIamGroupAndPolicyCarryNoTags(t *testing.T) {
	// Groups: IAM groups cannot be tagged; iamtypes.GroupDetail has no Tags field.
	// Policies: built from iamtypes.ManagedPolicyDetail, which carries no tags —
	// policy tags need their own API call. Both are named platform-engineering so
	// a matcher that wrongly looked at the name would show up here.
	for _, rt := range []string{cloud.Group, cloud.Policy} {
		resources := fetchAccessByID(t, rt)
		if len(resources) == 0 {
			t.Fatalf("no %s fetched, fixture is wrong", rt)
		}
		for _, r := range resources {
			if v, present := r.Properties()[properties.Tags]; present {
				t.Errorf("%s %s has Tags %#v, want none", rt, r.Id(), v)
			}
			assertMatchesAll(t, r, false)
		}
	}
}
