package console

import (
	"strings"

	"github.com/olekukonko/tablewriter"
)

// minColumnWidth is the narrowest a column is squeezed to before columns start being
// dropped instead. 30 is the column width tables used to be wrapped at, so on a
// narrow terminal the same columns are dropped as before; a column naturally
// narrower than this is never wrapped at all.
const minColumnWidth = 30

// cellWidth is the display width of the widest line of s.
func cellWidth(s string) int {
	var widest int
	for _, line := range strings.Split(s, "\n") {
		widest = max(widest, tablewriter.DisplayWidth(line))
	}
	return widest
}

// renderedWidth is the width of a table whose columns have the given widths, as
// tablewriter draws them here (left and right borders on): one border, then per
// column two margins and a separator.
func renderedWidth(widths []int) int {
	total := 1
	for _, w := range widths {
		total += w + 3
	}
	return total
}

// columnWidths decides how wide each column of a table may be in maxwidth columns,
// given each column's natural width (its widest cell). maxwidth <= 0 means no limit.
//
// A table that fits keeps its natural widths. One that does not has the room shared
// out max-min fairly: columns narrower than their share keep their natural width and
// the remaining room is split evenly between the wide ones, so the whole width is
// used. No column goes below min(natural, minColumnWidth). When even that cannot fit
// and dropColumns is set, columns are dropped from the right — the result is then
// shorter than natural — but the first column is always kept. A table that cannot fit
// at all gets the floor widths and overflows rather than showing nothing.
func columnWidths(natural []int, maxwidth int, dropColumns bool) []int {
	if maxwidth <= 0 || renderedWidth(natural) <= maxwidth {
		return append([]int(nil), natural...)
	}

	floor := make([]int, len(natural))
	for i, n := range natural {
		floor[i] = min(n, minColumnWidth)
	}

	k := len(natural)
	if dropColumns && k > 0 {
		k = 1
		for k < len(natural) && renderedWidth(floor[:k+1]) <= maxwidth {
			k++
		}
	}
	natural, floor = natural[:k], floor[:k]

	avail := maxwidth - renderedWidth(make([]int, k))
	if sumWidths(natural) <= avail {
		return append([]int(nil), natural...)
	}

	capped := func(c int) []int {
		widths := make([]int, k)
		for i := range natural {
			widths[i] = max(floor[i], min(natural[i], c))
		}
		return widths
	}
	if sumWidths(floor) > avail {
		return floor
	}

	// The largest cap that fits, by bisection: the total only grows with the cap.
	lo, hi := 0, 0
	for _, n := range natural {
		hi = max(hi, n)
	}
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if sumWidths(capped(mid)) <= avail {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	widths := capped(lo)

	// What the cap leaves over goes, a column at a time, to those still short.
	for left := avail - sumWidths(widths); left > 0; {
		progress := false
		for i := range widths {
			if left > 0 && widths[i] < natural[i] {
				widths[i]++
				left--
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	return widths
}

func sumWidths(ns []int) int {
	var s int
	for _, n := range ns {
		s += n
	}
	return s
}
