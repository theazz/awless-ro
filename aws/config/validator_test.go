package awsconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestedRegions(t *testing.T) {
	if got, want := stringInSlice("eu-west-1", SuggestedRegions), true; got != want {
		t.Errorf("eu-west-1: got %t, want %t", got, want)
	}
	if got, want := stringInSlice("us-east-1", SuggestedRegions), true; got != want {
		t.Errorf("us-east-1: got %t, want %t", got, want)
	}
	if got, want := stringInSlice("us-west-1", SuggestedRegions), true; got != want {
		t.Errorf("us-west-1: got %t, want %t", got, want)
	}
	if got, want := stringInSlice("eu-test-1", SuggestedRegions), false; got != want {
		t.Errorf("eu-test-1: got %t, want %t", got, want)
	}

	// every suggestion must pass validation, otherwise completion would offer
	// values the tool then rejects
	for _, r := range SuggestedRegions {
		if !IsValidRegion(r) {
			t.Errorf("%q is suggested but not considered a valid region", r)
		}
	}

	// sorted and free of duplicates, so that completion output is stable
	seen := make(map[string]bool, len(SuggestedRegions))
	for i, r := range SuggestedRegions {
		if seen[r] {
			t.Errorf("%q appears more than once", r)
		}
		seen[r] = true
		if i > 0 && SuggestedRegions[i-1] > r {
			t.Errorf("not sorted: %q comes after %q", r, SuggestedRegions[i-1])
		}
	}
}

func TestIsValidRegion(t *testing.T) {
	tcases := []struct {
		region string
		expect bool
	}{
		// commercial geographies, including ones launched after the bundled
		// SDK's own region list was frozen
		{region: "us-east-1", expect: true},
		{region: "eu-west-3", expect: true},
		{region: "eu-central-2", expect: true},
		{region: "eu-south-2", expect: true},
		{region: "ap-southeast-4", expect: true},
		{region: "ap-south-2", expect: true},
		{region: "af-south-1", expect: true},
		{region: "me-central-1", expect: true},
		{region: "il-central-1", expect: true},
		{region: "ca-west-1", expect: true},
		{region: "mx-central-1", expect: true},
		// other partitions
		{region: "cn-northwest-1", expect: true},
		{region: "us-gov-east-1", expect: true},
		{region: "us-iso-west-1", expect: true},
		{region: "us-isob-east-1", expect: true},
		{region: "eusc-de-east-1", expect: true},
		// rejected
		{region: "aa-test-10", expect: false},
		{region: "", expect: false},
		{region: "us-east", expect: false},
		{region: "useast1", expect: false},
		{region: "US-EAST-1", expect: false},
		{region: "us-east-1a", expect: false},
	}
	for _, tcase := range tcases {
		if got, want := IsValidRegion(tcase.region), tcase.expect; got != want {
			t.Errorf("%q: got %t, want %t", tcase.region, got, want)
		}
	}
}

func TestProfileValid(t *testing.T) {
	awsHomeTmp, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.RemoveAll(awsHomeTmp)
	}()

	awsHomeFunc = func() string {
		return awsHomeTmp
	}

	os.WriteFile(filepath.Join(awsHomeTmp, "config"), []byte(`[profile mfa]
region = us-west-1
role_arn = arn:aws:iam::1234567890:role/my-role
source_profile = default
[janedoe-mfa]
source_profile = jdoe
mfa_serial = arn:aws:iam::1234567890:mfa/janedoe
role_arn = arn:aws:iam::1234567890:role/my-role
`), 0600)
	os.WriteFile(filepath.Join(awsHomeTmp, "credentials"), []byte(`[default]
aws_access_key_id = ABCDEXAMPLE01234
aws_secret_access_key = aSecretKeyInMycredentials

[readonly]
aws_access_key_id =  ABCDEXAMPLE01234567
aws_secret_access_key = anotherSecretKeyInMycredentials
`), 0600)

	tcases := []struct {
		profile string
		expect  bool
	}{
		{profile: "", expect: false},
		{profile: "nothere", expect: false},
		{profile: "default", expect: true},
		{profile: "readonly", expect: true},
		{profile: "mfa", expect: true},
		{profile: "janedoe-mfa", expect: true},
	}
	for i, tcase := range tcases {
		if got, want := IsValidProfile(tcase.profile), tcase.expect; got != want {
			t.Fatalf("%d: '%s': got %t, want %t", i+1, tcase.profile, got, want)
		}
	}
}

func TestInstanceTypeValid(t *testing.T) {
	tcases := []struct {
		str    string
		expect bool
	}{
		{"t2.micro", true},
		{"m3.large", true},
		{"t.", false},
		{".", false},
		{"a.", false},
	}
	for _, tcase := range tcases {
		if got, want := isValidInstanceType(tcase.str), tcase.expect; got != want {
			t.Errorf("%s: got %t, want %t", tcase.str, got, want)
		}
	}
}
