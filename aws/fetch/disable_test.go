package awsfetch

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// `awless-ro config set aws.infra.instance.sync false` exists so that a resource type
// that is slow, huge or simply not interesting can be left out of a sync. The point is
// to avoid the API call, not just to drop the result, so these tests assert the client
// was never touched.
func TestDisabledResourceMakesNoAPICall(t *testing.T) {
	api := &pagedEc2{vpcPages: [][]ec2types.Vpc{{{VpcId: awssdk.String("vpc-1")}}}}

	conf := NewConfig(&AWSAPI{Ec2: api})
	conf.Extra["aws.infra.vpc.sync"] = false

	funcs := BuildInfraFetchFuncs(conf)
	resources := runFetcher(t, funcs, "vpc")

	if len(resources) != 0 {
		t.Errorf("got %d resources from a disabled fetcher, want none", len(resources))
	}
	if api.calls != 0 {
		t.Errorf("a disabled fetcher called the API %d times; the point is to skip the call", api.calls)
	}
}

// Disabling one resource must not disable its neighbours: the keys are per resource,
// and a prefix match would take out everything under aws.infra.
func TestDisablingOneResourceLeavesOthersAlone(t *testing.T) {
	api := &pagedEc2{
		vpcPages:      [][]ec2types.Vpc{{{VpcId: awssdk.String("vpc-1")}}},
		instancePages: [][]ec2types.Reservation{{{Instances: []ec2types.Instance{{InstanceId: awssdk.String("i-1")}}}}},
	}

	conf := NewConfig(&AWSAPI{Ec2: api})
	conf.Extra["aws.infra.vpc.sync"] = false

	funcs := BuildInfraFetchFuncs(conf)

	if got := runFetcher(t, funcs, "vpc"); len(got) != 0 {
		t.Errorf("vpc should be disabled, got %d resources", len(got))
	}
	if got := runFetcher(t, funcs, "instance"); len(got) != 1 {
		t.Errorf("instance should still be fetched, got %d resources", len(got))
	}
}

// Anything other than an explicit false leaves the resource enabled, so a typo in the
// value cannot quietly stop a resource being synced.
func TestOnlyAnExplicitFalseDisables(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
	}{
		{"absent", nil},
		{"true", true},
		{"string false", "false"},
		{"zero", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &pagedEc2{vpcPages: [][]ec2types.Vpc{{{VpcId: awssdk.String("vpc-1")}}}}

			conf := NewConfig(&AWSAPI{Ec2: api})
			if tc.value != nil {
				conf.Extra["aws.infra.vpc.sync"] = tc.value
			}

			funcs := BuildInfraFetchFuncs(conf)
			if got := runFetcher(t, funcs, "vpc"); len(got) != 1 {
				t.Errorf("value %#v disabled the fetcher; only a boolean false should", tc.value)
			}
		})
	}
}

// The force flag in the context overrides the setting, which is what makes an explicit
// `awless-ro sync` fetch what the configuration normally skips.
func TestForceOverridesADisabledResource(t *testing.T) {
	api := &pagedEc2{vpcPages: [][]ec2types.Vpc{{{VpcId: awssdk.String("vpc-1")}}}}

	conf := NewConfig(&AWSAPI{Ec2: api})
	conf.Extra["aws.infra.vpc.sync"] = false

	funcs := BuildInfraFetchFuncs(conf)
	fn := funcs["vpc"]

	ctx := context.WithValue(context.Background(), "force", true)
	resources, _, err := fn(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Errorf("got %d resources, want 1: force should fetch what the configuration skips", len(resources))
	}
}
