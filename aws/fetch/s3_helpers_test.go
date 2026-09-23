package awsfetch

import (
	"context"
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/theazz/awless-ro/graph"
)

func TestFetchFunctions(t *testing.T) {
	t.Parallel()
	t.Run("fetchAndExtractGrants", func(t *testing.T) {
		bucketsACL := map[string][]s3types.Grant{
			"bucket_1": {
				{Permission: "Read", Grantee: &s3types.Grantee{ID: awssdk.String("usr_1"), Type: "my_type_1"}},
				{Permission: "Write", Grantee: &s3types.Grantee{ID: awssdk.String("usr_2"), DisplayName: awssdk.String("my_user_2"), Type: "my_type_2"}},
				{Permission: "Execute", Grantee: &s3types.Grantee{ID: awssdk.String("usr_3"), DisplayName: awssdk.String("my_user_3"), EmailAddress: awssdk.String("user@domain"), Type: "my_type_3"}},
			},
			"bucket_2": {
				{Permission: "Read", Grantee: &s3types.Grantee{URI: awssdk.String("group_uri"), Type: "Group"}},
				{Permission: "Write", Grantee: &s3types.Grantee{ID: awssdk.String("usr_1"), Type: "my_type_2"}},
			},
		}
		bucket1 := s3types.Bucket{Name: awssdk.String("bucket_1")}
		mock := &stubS3{grants: bucketsACL}
		res, err := fetchAndExtractGrantsFn(context.Background(), mock, awssdk.ToString(bucket1.Name))
		if err != nil {
			t.Fatal(err)
		}
		expected := []*graph.Grant{
			{Permission: "Read", Grantee: graph.Grantee{GranteeID: "usr_1", GranteeType: "my_type_1"}},
			{Permission: "Write", Grantee: graph.Grantee{GranteeID: "usr_2", GranteeDisplayName: "my_user_2", GranteeType: "my_type_2"}},
			{Permission: "Execute", Grantee: graph.Grantee{GranteeID: "usr_3", GranteeDisplayName: "my_user_3<user@domain>", GranteeType: "my_type_3"}},
		}
		if got, want := len(res), len(expected); got != want {
			t.Fatalf("got %d, want %d", got, want)
		}
		for i := range expected {
			if got, want := res[i].String(), expected[i].String(); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		}

		bucket2 := s3types.Bucket{Name: awssdk.String("bucket_2")}
		res, err = fetchAndExtractGrantsFn(context.Background(), mock, awssdk.ToString(bucket2.Name))
		if err != nil {
			t.Fatal(err)
		}
		expected = []*graph.Grant{
			// A group grantee is identified by its URI, which the extractor
			// appends to the id.
			{Permission: "Read", Grantee: graph.Grantee{GranteeID: "group_uri", GranteeType: "Group"}},
			{Permission: "Write", Grantee: graph.Grantee{GranteeID: "usr_1", GranteeType: "my_type_2"}},
		}
		for i := range expected {
			if got, want := res[i].String(), expected[i].String(); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		}
	})
}

// stubS3 answers the S3 operations the helpers use. It embeds S3API so that any
// operation it does not implement panics by name rather than returning a zero
// value.
type stubS3 struct {
	S3API
	buckets map[string][]s3types.Bucket
	objects map[string][]s3types.Object
	grants  map[string][]s3types.Grant
}

func (m *stubS3) GetBucketAcl(_ context.Context, input *s3.GetBucketAclInput, _ ...func(*s3.Options)) (*s3.GetBucketAclOutput, error) {
	return &s3.GetBucketAclOutput{Grants: m.grants[awssdk.ToString(input.Bucket)]}, nil
}

func (m *stubS3) ListBuckets(_ context.Context, _ *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	var buckets []s3types.Bucket
	for _, b := range m.buckets {
		buckets = append(buckets, b...)
	}
	return &s3.ListBucketsOutput{Buckets: buckets}, nil
}

func (m *stubS3) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{Contents: m.objects[awssdk.ToString(input.Bucket)]}, nil
}

func (m *stubS3) GetBucketLocation(_ context.Context, input *s3.GetBucketLocationInput, _ ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	for region, buckets := range m.buckets {
		for _, bucket := range buckets {
			if awssdk.ToString(bucket.Name) == awssdk.ToString(input.Bucket) {
				return &s3.GetBucketLocationOutput{
					LocationConstraint: s3types.BucketLocationConstraint(region),
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("bucket location mock: bucket %s not found", awssdk.ToString(input.Bucket))
}
