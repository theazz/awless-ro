package awsservices

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// stubSTS satisfies awsfetch.StsAPI outright: the only STS operation awless-ro
// calls is GetCallerIdentity, so the narrow interface has exactly one method.
type stubSTS struct {
	output *sts.GetCallerIdentityOutput
}

func (m *stubSTS) GetCallerIdentity(_ context.Context, _ *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return m.output, nil
}

func TestGetIdentityParseAllTypesOfUsername(t *testing.T) {
	tcases := []struct {
		arn, expResource, expResourceType, expResourcePath string
	}{
		{arn: "", expResource: "", expResourceType: "", expResourcePath: ""},
		{arn: "arn:", expResource: "", expResourceType: "", expResourcePath: ""},
		{arn: "arn:aws:iam::123456789012:root", expResource: "root", expResourceType: "user", expResourcePath: "root"},
		{arn: "arn:aws:iam::123456789012:user/Bob", expResource: "Bob", expResourceType: "user", expResourcePath: "user/Bob"},
		{arn: "arn:aws:iam::123456789012:user/division_abc/subdivision_xyz/Donald", expResource: "division_abc/subdivision_xyz/Donald", expResourceType: "user", expResourcePath: "user/division_abc/subdivision_xyz/Donald"},
	}

	for _, tcase := range tcases {
		out := &sts.GetCallerIdentityOutput{Arn: awssdk.String(tcase.arn)}
		access := Access{StsAPI: &stubSTS{output: out}}

		id, err := access.GetIdentity(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got, want := id.Resource, tcase.expResource; got != want {
			t.Errorf("got '%s', want '%s'", got, want)
		}
		if got, want := id.ResourceType, tcase.expResourceType; got != want {
			t.Errorf("got '%s', want '%s'", got, want)
		}
		if got, want := id.ResourcePath, tcase.expResourcePath; got != want {
			t.Errorf("got '%s', want '%s'", got, want)
		}
	}
}
