package awsfetch

import (
	"reflect"
	"testing"
)

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
