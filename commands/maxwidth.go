package commands

import (
	"errors"
	"strconv"

	"github.com/theazz/awless-ro/console"
)

// maxWidthValue is the --max-width flag. It is a flag value of its own rather than an
// int flag so that a negative or malformed width is refused while the command line is
// parsed — before any hook runs, so before any setup or AWS call — and so that "not
// given" can be told apart from an explicit 0, which means no limit.
type maxWidthValue struct {
	n   int
	set bool
}

func (v *maxWidthValue) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return errors.New("must be a whole number of columns, 0 for no limit")
	}
	if n < 0 {
		return errors.New("must not be negative (0 means no limit)")
	}
	v.n, v.set = n, true
	return nil
}

// String is empty when the flag is not given, so the help shows no default: the
// default depends on where the output goes.
func (v *maxWidthValue) String() string {
	if !v.set {
		return ""
	}
	return strconv.Itoa(v.n)
}

func (v *maxWidthValue) Type() string { return "int" }

var maxWidthFlag maxWidthValue

const maxWidthUsage = "Fit table output to this many columns, 0 for no limit (default: the terminal width; no limit when output is not a terminal)"

// tableMaxWidth is the width tables are laid out in, 0 meaning no limit.
func tableMaxWidth() int {
	return console.TableWidth(maxWidthFlag.n, maxWidthFlag.set, console.StdoutTerminalWidth)
}
