package match

import (
	"strings"

	"github.com/theazz/awless-ro/cloud"
)

// Filter is one parsed `--filter` argument: which column to look at, what to
// compare against it, and whether the comparison is anchored.
type Filter struct {
	Key   string
	Value string
	Exact bool
}

// ParseFilter reads the `--filter` grammar. `key=value` matches a substring,
// `key==value` matches the whole value; both are case insensitive.
//
// Upstream had only the substring form, which meant a filter value could never
// exclude a longer value containing it: `--filter state=active` also returned
// Inactive access keys (wallix/awless#252) and `--filter type=A` also returned
// CNAME and SOA records (wallix/awless#296), because case-insensitively
// "inactive" contains "active" and "cname" contains "a". No anchored syntax was
// reachable from the CLI at all. `=` keeps its substring meaning, which the
// README documents and examples such as `--filter type=t3` rely on, so the
// anchored form is spelled `==`.
//
// The operator is found by looking at the first `=` and the byte after it, and
// the value is then taken verbatim. Splitting on the first `=` alone — which is
// what every caller used to do — turns `state==Active` into the key `state` and
// the value `=Active`, which matches nothing. Nothing is re-split afterwards, so
// a value may itself contain `=`: `tag==a=b` is an exact match on `a=b`.
//
// The second result is false when the argument carries no `=` at all, which is
// how callers have always silently ignored such an argument.
func ParseFilter(arg string) (Filter, bool) {
	i := strings.Index(arg, "=")
	if i < 0 {
		return Filter{}, false
	}
	f := Filter{Key: arg[:i]}
	if rest := arg[i+1:]; strings.HasPrefix(rest, "=") {
		f.Exact, f.Value = true, rest[1:]
	} else {
		f.Value = rest
	}
	return f, true
}

// PropertyFilter builds the predicate for one parsed filter. Both forms compare
// the stored value as a string and ignore case, so they differ only in
// anchoring: exact drops the Contains() step, which is what makes `==` able to
// exclude a longer value.
func PropertyFilter(name, val string, exact bool) cloud.Matcher {
	m := Property(name, val).IgnoreCase().MatchString()
	if exact {
		return m
	}
	return m.Contains()
}
