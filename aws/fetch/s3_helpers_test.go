package awsfetch

import (
	"context"
	"reflect"
	"sort"
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

func (m *stubS3) ListBuckets(_ context.Context, input *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	// Keyed by region, as S3 answers with BucketRegion set; "" is us-east-1, as the
	// location constraint used to say.
	var buckets []s3types.Bucket
	for region, bs := range m.buckets {
		if region == "" {
			region = "us-east-1"
		}
		if r := awssdk.ToString(input.BucketRegion); r != "" && r != region {
			continue
		}
		buckets = append(buckets, bs...)
	}
	return &s3.ListBucketsOutput{Buckets: buckets}, nil
}

func (m *stubS3) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{Contents: m.objects[awssdk.ToString(input.Bucket)]}, nil
}

// recordingS3 notes the region each ListBuckets asked for.
type recordingS3 struct {
	stubS3
	asked []string
}

func (m *recordingS3) ListBuckets(ctx context.Context, input *s3.ListBucketsInput, opts ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	m.asked = append(m.asked, awssdk.ToString(input.BucketRegion))
	return m.stubS3.ListBuckets(ctx, input, opts...)
}

func TestBucketsOfTheRegionComeFromOneFilteredListing(t *testing.T) {
	api := &recordingS3{stubS3: stubS3{buckets: map[string][]s3types.Bucket{
		"eu-west-1": {{Name: awssdk.String("logs-eu")}, {Name: awssdk.String("app-eu")}},
		"":          {{Name: awssdk.String("legacy-us")}},
		"us-west-2": {{Name: awssdk.String("logs-us")}},
	}}}

	cases := []struct {
		region string
		want   []string
	}{
		{"eu-west-1", []string{"app-eu", "logs-eu"}},
		{"us-east-1", []string{"legacy-us"}},
		{"ap-southeast-1", nil},
	}
	for _, tc := range cases {
		api.asked = nil
		ctx := context.WithValue(context.Background(), "region", tc.region)
		buckets, err := getBucketsPerRegion(ctx, api)
		if err != nil {
			t.Fatalf("%s: %s", tc.region, err)
		}
		var got []string
		for _, b := range buckets {
			got = append(got, awssdk.ToString(b.Name))
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.region, got, tc.want)
		}
		// S3 does the filtering: one listing, asked for this region. There is no
		// per-bucket call left to make — GetBucketLocation is not in S3API.
		if !reflect.DeepEqual(api.asked, []string{tc.region}) {
			t.Errorf("%s: ListBuckets was asked for %q, want exactly one call for the region", tc.region, api.asked)
		}
	}
}

func TestBucketsAreNotListedWithoutARegion(t *testing.T) {
	api := &recordingS3{stubS3: stubS3{buckets: map[string][]s3types.Bucket{
		"eu-west-1": {{Name: awssdk.String("logs-eu")}},
	}}}

	if _, err := getBucketsPerRegion(context.Background(), api); err == nil {
		t.Error("with no region the filter would be dropped and every bucket shown as local")
	}
	if len(api.asked) != 0 {
		t.Errorf("nothing should have been listed, got %d calls", len(api.asked))
	}
}

func TestBucketNameFilterStillApplies(t *testing.T) {
	api := &recordingS3{stubS3: stubS3{buckets: map[string][]s3types.Bucket{
		"eu-west-1": {{Name: awssdk.String("logs-eu")}, {Name: awssdk.String("app-eu")}},
	}}}
	ctx := context.WithValue(context.Background(), "region", "eu-west-1")
	ctx = context.WithValue(ctx, "filters", []string{"bucket=LOGS"})

	buckets, err := getBucketsPerRegion(ctx, api)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 1 || awssdk.ToString(buckets[0].Name) != "logs-eu" {
		t.Errorf("got %v, want only logs-eu", buckets)
	}
}
