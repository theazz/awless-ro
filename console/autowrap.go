package console

import (
	"strings"
	"unicode/utf8"

	"github.com/olekukonko/tablewriter"
)

// wrapingChars are the characters a token too long for its column may be broken
// after. A break never inserts or removes anything inside a token, so the lines of a
// broken ARN concatenate back to the ARN and it can be copied off the screen.
const wrapingChars = " \n.,?;:/+=*-_°)(\"!@#"

// wrapCell lays s out in lines no wider than width display columns, and returns it
// unchanged when width <= 0 (no limit) or when it already fits.
//
// Words are filled greedily, the space at a line break being dropped. A word wider
// than the column is cut after the last wrapingChars character that keeps the piece
// within the width, or exactly at the width when it has none. Existing line breaks
// are kept. Width is display width: an ANSI escape sequence counts for nothing and is
// never split, a wide rune counts for two.
//
// It replaces a wrapper that inserted spaces into long tokens and left the breaking
// to tablewriter, which then rejoined the pieces with a visible space and measured
// bytes rather than columns.
func wrapCell(s string, width int) string {
	if width <= 0 || cellWidth(s) <= width {
		return s
	}

	var lines []string
	for _, para := range strings.Split(s, "\n") {
		lines = append(lines, wrapParagraph(para, width)...)
	}
	return strings.Join(lines, "\n")
}

func wrapParagraph(para string, width int) []string {
	if textWidth(para) <= width {
		return []string{para}
	}

	var lines []string
	var cur string
	var curW int
	started := false
	for _, word := range strings.Split(para, " ") {
		ww := textWidth(word)
		if started && curW+1+ww <= width {
			cur, curW = cur+" "+word, curW+1+ww
			continue
		}
		if started {
			lines = append(lines, cur)
		}
		if ww <= width {
			cur, curW, started = word, ww, true
			continue
		}
		chunks := breakWord(word, width)
		lines = append(lines, chunks[:len(chunks)-1]...)
		cur = chunks[len(chunks)-1]
		curW, started = textWidth(cur), true
	}
	return append(lines, cur)
}

// breakWord cuts a word wider than width into pieces of at most width columns.
func breakWord(word string, width int) []string {
	units := splitUnits(word)
	var chunks []string
	for len(units) > 0 {
		// The longest prefix that fits, and the last break character inside it.
		w, n, lastBreak := 0, 0, 0
		for n < len(units) && w+units[n].width <= width {
			w += units[n].width
			if units[n].isBreak {
				lastBreak = n + 1
			}
			n++
		}
		if n == len(units) {
			chunks = append(chunks, joinUnits(units))
			break
		}
		switch {
		case n == 0:
			n = 1 // a rune wider than the whole column gets a line of its own
		case lastBreak > 0:
			n = lastBreak
		}
		chunks = append(chunks, joinUnits(units[:n]))
		units = units[n:]
	}
	return chunks
}

type unit struct {
	text    string
	width   int
	isBreak bool
}

// splitUnits splits s into runes and whole ANSI escape sequences (ESC [ … final
// byte), the latter being zero-width.
func splitUnits(s string) []unit {
	var units []unit
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			units = append(units, unit{text: s[i:j]})
			i = j
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		units = append(units, unit{
			text:    s[i : i+n],
			width:   tablewriter.DisplayWidth(string(r)),
			isBreak: strings.ContainsRune(wrapingChars, r),
		})
		i += n
	}
	return units
}

func joinUnits(units []unit) string {
	var b strings.Builder
	for _, u := range units {
		b.WriteString(u.text)
	}
	return b.String()
}

func textWidth(s string) int {
	var w int
	for _, u := range splitUnits(s) {
		w += u.width
	}
	return w
}
