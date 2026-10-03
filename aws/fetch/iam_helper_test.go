package awsfetch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// authDetailsIam answers GetAccountAuthorizationDetails the way IAM does: only the
// entity types in the filter, two pages per type, continued by Marker. Every first
// page waits until `together` paginations have started, so a fetch that pages the
// types one after another never gets past the first one.
type authDetailsIam struct {
	IamAPI
	together int
	fail     iamtypes.EntityType

	mu      sync.Mutex
	started int
	ready   chan struct{}
	filters []string
}

func (m *authDetailsIam) GetAccountAuthorizationDetails(ctx context.Context, in *iam.GetAccountAuthorizationDetailsInput, _ ...func(*iam.Options)) (*iam.GetAccountAuthorizationDetailsOutput, error) {
	page := tokenIndex(awssdk.ToString(in.Marker))
	if page == 0 {
		m.mu.Lock()
		m.filters = append(m.filters, fmt.Sprint(in.Filter))
		m.started++
		if m.started == m.together {
			close(m.ready)
		}
		m.mu.Unlock()
		select {
		case <-m.ready:
		case <-time.After(2 * time.Second):
			return nil, errors.New("paginations did not overlap: the entity types are fetched one after another")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	out := &iam.GetAccountAuthorizationDetailsOutput{}
	for _, e := range in.Filter {
		if e == m.fail {
			return nil, errors.New("AccessDenied on " + string(e))
		}
		name := string(e) + "-" + strconv.Itoa(page)
		switch e {
		case iamtypes.EntityTypeUser:
			out.UserDetailList = append(out.UserDetailList, iamtypes.UserDetail{UserName: awssdk.String(name)})
		case iamtypes.EntityTypeGroup:
			out.GroupDetailList = append(out.GroupDetailList, iamtypes.GroupDetail{GroupName: awssdk.String(name)})
		case iamtypes.EntityTypeRole:
			out.RoleDetailList = append(out.RoleDetailList, iamtypes.RoleDetail{RoleName: awssdk.String(name)})
		default:
			out.Policies = append(out.Policies, iamtypes.ManagedPolicyDetail{PolicyName: awssdk.String(name)})
		}
	}
	if page == 0 {
		out.IsTruncated = true
		out.Marker = awssdk.String("1")
	}
	return out, nil
}

var allEntityTypes = []iamtypes.EntityType{
	iamtypes.EntityTypeUser, iamtypes.EntityTypeGroup, iamtypes.EntityTypeRole,
	iamtypes.EntityTypeLocalManagedPolicy, iamtypes.EntityTypeAWSManagedPolicy,
}

func TestAccountDetailsPageEntityTypesSideBySide(t *testing.T) {
	api := &authDetailsIam{together: 4, ready: make(chan struct{})}

	details, err := fetchAccountAuthorizationDetails(context.Background(), allEntityTypes, api)
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(api.filters)
	want := []string{"[AWSManagedPolicy]", "[LocalManagedPolicy]", "[Role]", "[User Group]"}
	if fmt.Sprint(api.filters) != fmt.Sprint(want) {
		t.Errorf("paginations %v, want %v", api.filters, want)
	}

	// Every page of every type, nothing lost in the merge, and each type's pages in
	// the order IAM returned them.
	names := func(n int, get func(int) *string) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, awssdk.ToString(get(i)))
		}
		return out
	}
	if got := names(len(details.Users), func(i int) *string { return details.Users[i].UserName }); fmt.Sprint(got) != "[User-0 User-1]" {
		t.Errorf("users: %v", got)
	}
	if got := names(len(details.Groups), func(i int) *string { return details.Groups[i].GroupName }); fmt.Sprint(got) != "[Group-0 Group-1]" {
		t.Errorf("groups: %v", got)
	}
	if got := names(len(details.Roles), func(i int) *string { return details.Roles[i].RoleName }); fmt.Sprint(got) != "[Role-0 Role-1]" {
		t.Errorf("roles: %v", got)
	}
	if len(details.Policies) != 4 {
		t.Errorf("got %d policies, want 2 pages of each of 2 kinds", len(details.Policies))
	}
}

func TestAccountDetailsFailWhenOnePaginationFails(t *testing.T) {
	api := &authDetailsIam{together: 4, ready: make(chan struct{}), fail: iamtypes.EntityTypeRole}

	done := make(chan error, 1)
	go func() {
		_, err := fetchAccountAuthorizationDetails(context.Background(), allEntityTypes, api)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil || err.Error() != "AccessDenied on Role" {
			t.Errorf("got %v, want the failing pagination's error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a failing pagination left the fetch waiting")
	}
}

func TestPaginationGroups(t *testing.T) {
	cases := []struct {
		in   []iamtypes.EntityType
		want string
	}{
		{allEntityTypes, "[[User Group] [Role] [LocalManagedPolicy] [AWSManagedPolicy]]"},
		{[]iamtypes.EntityType{iamtypes.EntityTypeRole}, "[[Role]]"},
		{[]iamtypes.EntityType{iamtypes.EntityTypeLocalManagedPolicy, iamtypes.EntityTypeAWSManagedPolicy}, "[[LocalManagedPolicy] [AWSManagedPolicy]]"},
		{[]iamtypes.EntityType{iamtypes.EntityTypeUser}, "[[User]]"},
	}
	for _, tc := range cases {
		if got := fmt.Sprint(paginationGroups(tc.in)); got != tc.want {
			t.Errorf("%v: got %s, want %s", tc.in, got, tc.want)
		}
	}
}
