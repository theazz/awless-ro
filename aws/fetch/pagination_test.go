package awsfetch

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"

	"github.com/theazz/awless-ro/fetch"
	"github.com/theazz/awless-ro/graph"
)

// Every fetcher that can paginate has to accumulate pages rather than return the
// first one. This was not idle worry: ten operations were fetching a single page
// under the old SDK and silently truncating the graph, which is why the generator
// now derives the paginator from the definitions.
//
// The mocks in aws/services hand back one page each, deliberately, because the
// continuation token is named differently by every service and generating
// multi-page mocks would encode more SDK trivia than it is worth. So the
// multi-page cases live here, hand-written, and cover the three shapes the
// generator emits rather than all thirty-two operations: testing a generated
// pattern thirty-two times tests the same code thirty-two times.

// pagedEc2 answers DescribeVpcs and DescribeInstances across several pages, echoing
// the continuation token the way EC2 does.
type pagedEc2 struct {
	Ec2API
	vpcPages      [][]ec2types.Vpc
	instancePages [][]ec2types.Reservation
	calls         int
}

func (p *pagedEc2) DescribeVpcs(_ context.Context, in *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	p.calls++
	idx := tokenIndex(awssdk.ToString(in.NextToken))
	out := &ec2.DescribeVpcsOutput{Vpcs: p.vpcPages[idx]}
	if idx+1 < len(p.vpcPages) {
		out.NextToken = awssdk.String(strconv.Itoa(idx + 1))
	}
	return out, nil
}

func (p *pagedEc2) DescribeInstances(_ context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	p.calls++
	idx := tokenIndex(awssdk.ToString(in.NextToken))
	out := &ec2.DescribeInstancesOutput{Reservations: p.instancePages[idx]}
	if idx+1 < len(p.instancePages) {
		out.NextToken = awssdk.String(strconv.Itoa(idx + 1))
	}
	return out, nil
}

func tokenIndex(token string) int {
	if token == "" {
		return 0
	}
	idx, err := strconv.Atoi(token)
	if err != nil {
		return 0
	}
	return idx
}

func ids(resources []*graph.Resource) []string {
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.Id())
	}
	return out
}

func runFetcher(t *testing.T, funcs fetch.Funcs, name string) []*graph.Resource {
	t.Helper()
	fn, ok := funcs[name]
	if !ok {
		t.Fatalf("no fetcher named %q", name)
	}
	resources, _, err := fn(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return resources
}

// A flat list: DescribeVpcs puts its results straight in the output.
func TestPaginatedFetcherCollectsEveryPage(t *testing.T) {
	api := &pagedEc2{vpcPages: [][]ec2types.Vpc{
		{{VpcId: awssdk.String("vpc-1")}, {VpcId: awssdk.String("vpc-2")}},
		{{VpcId: awssdk.String("vpc-3")}},
		{{VpcId: awssdk.String("vpc-4")}, {VpcId: awssdk.String("vpc-5")}},
	}}

	funcs := BuildInfraFetchFuncs(NewConfig(&AWSAPI{Ec2: api}))
	got := ids(runFetcher(t, funcs, "vpc"))

	want := []string{"vpc-1", "vpc-2", "vpc-3", "vpc-4", "vpc-5"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v — pages after the first were dropped", got, want)
	}
	if api.calls != 3 {
		t.Errorf("the API was called %d times, want 3 (one per page)", api.calls)
	}
}

// A nested list: instances arrive inside reservations, so the generated fetcher has
// two loops and the paginator wraps both.
func TestPaginatedFetcherCollectsNestedResultsAcrossPages(t *testing.T) {
	reservation := func(instanceIDs ...string) ec2types.Reservation {
		var instances []ec2types.Instance
		for _, id := range instanceIDs {
			instances = append(instances, ec2types.Instance{InstanceId: awssdk.String(id)})
		}
		return ec2types.Reservation{Instances: instances}
	}

	api := &pagedEc2{instancePages: [][]ec2types.Reservation{
		{reservation("i-1", "i-2"), reservation("i-3")},
		{reservation("i-4")},
	}}

	funcs := BuildInfraFetchFuncs(NewConfig(&AWSAPI{Ec2: api}))
	got := ids(runFetcher(t, funcs, "instance"))

	want := []string{"i-1", "i-2", "i-3", "i-4"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// IAM pages with Marker and IsTruncated rather than NextToken. The paginator comes
// from the SDK, so what this checks is that the generated fetcher reached for the
// right one: an operation wired to the wrong paginator stops after a page.
type pagedIam struct {
	IamAPI
	pages [][]iamtypes.InstanceProfile
	calls int
}

func (p *pagedIam) ListInstanceProfiles(_ context.Context, in *iam.ListInstanceProfilesInput, _ ...func(*iam.Options)) (*iam.ListInstanceProfilesOutput, error) {
	p.calls++
	idx := tokenIndex(awssdk.ToString(in.Marker))
	out := &iam.ListInstanceProfilesOutput{InstanceProfiles: p.pages[idx]}
	if idx+1 < len(p.pages) {
		out.IsTruncated = true
		out.Marker = awssdk.String(strconv.Itoa(idx + 1))
	}
	return out, nil
}

func TestPaginationWithMarkerAndIsTruncated(t *testing.T) {
	profile := func(id string) iamtypes.InstanceProfile {
		return iamtypes.InstanceProfile{
			InstanceProfileId:   awssdk.String(id),
			InstanceProfileName: awssdk.String(id),
			Arn:                 awssdk.String("arn:aws:iam::123456789012:instance-profile/" + id),
		}
	}

	api := &pagedIam{pages: [][]iamtypes.InstanceProfile{
		{profile("ip-1")},
		{profile("ip-2")},
		{profile("ip-3")},
	}}

	funcs := BuildAccessFetchFuncs(NewConfig(&AWSAPI{Iam: api}))
	got := ids(runFetcher(t, funcs, "instanceprofile"))

	want := []string{"ip-1", "ip-2", "ip-3"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if api.calls != 3 {
		t.Errorf("the API was called %d times, want 3", api.calls)
	}
}

// elbv2 pages with NextMarker. This is one of the operations that was fetching a
// single page before the pagination audit.
type pagedElbv2 struct {
	Elbv2API
	pages [][]elbv2types.TargetGroup
	calls int
}

func (p *pagedElbv2) DescribeTargetGroups(_ context.Context, in *elasticloadbalancingv2.DescribeTargetGroupsInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error) {
	p.calls++
	idx := tokenIndex(awssdk.ToString(in.Marker))
	out := &elasticloadbalancingv2.DescribeTargetGroupsOutput{TargetGroups: p.pages[idx]}
	if idx+1 < len(p.pages) {
		out.NextMarker = awssdk.String(strconv.Itoa(idx + 1))
	}
	return out, nil
}

func TestPaginationWithNextMarker(t *testing.T) {
	api := &pagedElbv2{pages: [][]elbv2types.TargetGroup{
		{{TargetGroupArn: awssdk.String("tg-1")}},
		{{TargetGroupArn: awssdk.String("tg-2")}},
	}}

	funcs := BuildInfraFetchFuncs(NewConfig(&AWSAPI{Elbv2: api}))
	got := ids(runFetcher(t, funcs, "targetgroup"))

	want := []string{"tg-1", "tg-2"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v — NextMarker pagination stopped early", got, want)
	}
	if api.calls != 2 {
		t.Errorf("the API was called %d times, want 2", api.calls)
	}
}

// An empty first page must end the loop rather than spin.
func TestPaginationHandlesAnEmptyResult(t *testing.T) {
	api := &pagedEc2{vpcPages: [][]ec2types.Vpc{{}}}

	funcs := BuildInfraFetchFuncs(NewConfig(&AWSAPI{Ec2: api}))
	if got := runFetcher(t, funcs, "vpc"); len(got) != 0 {
		t.Errorf("got %d resources, want none", len(got))
	}
	if api.calls != 1 {
		t.Errorf("the API was called %d times, want 1", api.calls)
	}
}
