package awsfetch

import (
	"context"
	"reflect"
	"testing"
)

// The fetchers that narrow server-side read --filter out of the context, so they
// need the same grammar as the displayer. Splitting on the first "=" alone left
// the exact form's second sign in the value, and `--filter bucket==logs` asked
// the API for a bucket named "=logs".
func TestUserFiltersFromContext(t *testing.T) {
	tcases := []struct {
		in  []string
		out map[string]string
	}{
		{in: []string{"bucket=logs"}, out: map[string]string{"bucket": "logs"}},
		{in: []string{"bucket==logs-eu"}, out: map[string]string{"bucket": "logs-eu"}},
		{in: []string{"zone==mysite.com."}, out: map[string]string{"zone": "mysite.com."}},
		{in: []string{"Bucket==Logs"}, out: map[string]string{"bucket": "Logs"}},
		{in: []string{"name=a=b"}, out: map[string]string{"name": "a=b"}},
		{in: []string{"name==a=b"}, out: map[string]string{"name": "a=b"}},
		{in: []string{"nokey"}, out: map[string]string{}},
		{in: []string{"cluster=", "name=="}, out: map[string]string{"cluster": "", "name": ""}},
		{in: nil, out: map[string]string{}},
	}
	for _, tcase := range tcases {
		ctx := context.WithValue(context.Background(), "filters", tcase.in)
		if got, want := getUserFiltersFromContext(ctx), tcase.out; !reflect.DeepEqual(got, want) {
			t.Errorf("%v: got %+v, want %+v", tcase.in, got, want)
		}
	}
}

// sliceOfSlice batches identifiers for the APIs that cap how many you may ask
// about at once, such as ecs.DescribeClusters. The test used to live in
// aws/services next to a copy of the function, so it verified the copy rather
// than the implementation the fetchers actually call.
func TestSliceOfSlice(t *testing.T) {
	var empty [][]string
	tcases := []struct {
		in        []string
		maxlength int
		out       [][]string
	}{
		{in: []string{"1", "2", "3"}, maxlength: 2, out: [][]string{{"1", "2"}, {"3"}}},
		{in: []string{"1", "2", "3"}, maxlength: 1, out: [][]string{{"1"}, {"2"}, {"3"}}},
		{in: []string{"1", "2", "3"}, maxlength: 3, out: [][]string{{"1", "2", "3"}}},
		{in: []string{"1", "2", "3"}, maxlength: 5, out: [][]string{{"1", "2", "3"}}},
		{in: []string{"1", "2", "3"}, maxlength: 0, out: empty},
		{in: []string{}, maxlength: 2, out: empty},
		{in: []string{"1", "2", "3", "4"}, maxlength: 2, out: [][]string{{"1", "2"}, {"3", "4"}}},
	}
	for i, tcase := range tcases {
		if got, want := sliceOfSlice(tcase.in, tcase.maxlength), tcase.out; !reflect.DeepEqual(got, want) {
			t.Fatalf("%d: got %+v, want %+v", i+1, got, want)
		}
	}
}
