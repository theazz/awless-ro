package console

import (
	"reflect"
	"testing"
)

func TestColumnWidths(t *testing.T) {
	cases := []struct {
		name     string
		natural  []int
		maxwidth int
		drop     bool
		want     []int
		overflow bool // the table cannot fit even at its floor widths
	}{
		{name: "no limit", natural: []int{10, 200}, maxwidth: 0, drop: true, want: []int{10, 200}},
		{name: "negative is no limit", natural: []int{10, 200}, maxwidth: -5, drop: true, want: []int{10, 200}},
		{name: "fits", natural: []int{10, 20}, maxwidth: 80, drop: true, want: []int{10, 20}},
		// 1 + (10+3) + (20+3)
		{name: "exactly fits", natural: []int{10, 20}, maxwidth: 37, drop: true, want: []int{10, 20}},
		{name: "one column short: only the wide column shrinks", natural: []int{10, 50}, maxwidth: 66, drop: true, want: []int{10, 49}},
		// room for columns: 100 - 1 - 9 = 90; 5 stays, 85 is split 43/42.
		{name: "narrow columns keep their width, wide ones share the rest", natural: []int{5, 100, 100}, maxwidth: 100, drop: true, want: []int{5, 43, 42}},
		// floors 30 each: two fit in 70 (67), three do not (100).
		{name: "floors do not fit: columns dropped from the right", natural: []int{40, 40, 40, 40}, maxwidth: 70, drop: true, want: []int{32, 31}},
		{name: "first column always kept", natural: []int{100, 10}, maxwidth: 20, drop: true, want: []int{30}, overflow: true},
		{name: "no dropping: floors overflow", natural: []int{40, 40, 40, 40}, maxwidth: 70, drop: false, want: []int{30, 30, 30, 30}, overflow: true},
		{name: "no dropping: shared out", natural: []int{8, 300}, maxwidth: 80, drop: false, want: []int{8, 65}},
		{name: "columns narrower than the floor are never wrapped", natural: []int{12, 25, 25, 25}, maxwidth: 60, drop: true, want: []int{12, 25}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := columnWidths(tc.natural, tc.maxwidth, tc.drop)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("columnWidths(%v, %d, %t) = %v, want %v", tc.natural, tc.maxwidth, tc.drop, got, tc.want)
			}
			if !tc.drop && len(got) != len(tc.natural) {
				t.Errorf("columns dropped although dropping is off: %v", got)
			}
			for i, w := range got {
				if floor := min(tc.natural[i], minColumnWidth); w < floor || w > tc.natural[i] {
					t.Errorf("column %d is %d wide, want between its floor %d and natural %d", i, w, floor, tc.natural[i])
				}
			}
			if tc.maxwidth > 0 && !tc.overflow && renderedWidth(got) > tc.maxwidth {
				t.Errorf("rendered width %d exceeds %d", renderedWidth(got), tc.maxwidth)
			}
		})
	}

	t.Run("the input is not modified", func(t *testing.T) {
		natural := []int{40, 40, 40}
		columnWidths(natural, 50, true)[0] = -1
		if !reflect.DeepEqual(natural, []int{40, 40, 40}) {
			t.Fatalf("natural widths changed to %v", natural)
		}
	})
}
