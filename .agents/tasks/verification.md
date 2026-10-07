# Verification — fix/console-sort-panic

Fixes two reproduced console-layer defects:

- Upstream wallix/awless#248 — `list subnets --sort public` panicked with
  `can not compare values of type bool` (`console/displayer.go`,
  `valueLowerOrEqual`).
- N5 (`.agents/tasks/upstream-bug-validation.md`, §5) — `TestNoHeadersDisplay`
  (`console/displayer_test.go`) overwrote the package-level
  `DefaultsColumnDefinitions` map without restoring it.

A second, same-shaped leak was found while proving N5 is gone (see "Shuffle
proof" below): `TestMaxWidth` mutates the package-level `autowrapMaxSize` /
`tableColWidth` vars and never restores them either. Fixed the same way
(`t.Cleanup`), since it blocked the very shuffle run the task asked for as
proof.

## Code changes

- `console/displayer.go`:
  - Added `case bool:` to `valueLowerOrEqual`'s type switch, `false` sorting
    before `true`.
  - Replaced both `panic(...)` calls in `valueLowerOrEqual` (the type-mismatch
    branch and the `default:` branch of the switch) with a fallback that
    compares `fmt.Sprint(a) <= fmt.Sprint(b)`. Rationale: this is a read-only
    inspection CLI; a column holding a type the sorter doesn't recognise should
    yield an arbitrary-but-stable order, not abort the process. No CLI-flag
    input can panic the sorter after this change.
- `console/displayer_test.go`:
  - `TestNoHeadersDisplay`: save/restore `DefaultsColumnDefinitions` via
    `t.Cleanup`.
  - `TestMaxWidth`: save/restore `autowrapMaxSize` / `tableColWidth` via
    `t.Cleanup` (same pollution pattern, found via the shuffle proof below).
- `graph/resourcetest/resourcetest.go`: added `Database` and `Volume`
  builders, needed by the new regression tests (`Database`/`Volume` are
  listed resource types with a boolean `Public`/`Encrypted` default column
  that reach the same sorter path).
- `console/sorter_test.go` (new): table-driven regression tests —
  `TestSortByBooleanColumn` (two different bools ascending/descending, upper
  case `--sort PUBLIC`, one nil/one bool, both nil, equal bools),
  `TestSortByBooleanColumnOtherResourceTypes` (subnet/image/database/
  launchconfiguration Public, vpc Default, volume Encrypted), and
  `TestValueLowerOrEqualUnsortableTypeDoesNotPanic` (unknown type and
  mismatched-type fallback, both non-panicking and stable).

## Commands run and results

```sh
go build ./...
# exit 0, no output

go vet ./...
# exit 0, no output

go test ./... -race
# all 28 packages: ok (or "no test files"), 0 FAIL
```

```
ok  	github.com/theazz/awless-ro
ok  	github.com/theazz/awless-ro/aws/config
ok  	github.com/theazz/awless-ro/aws/conv
ok  	github.com/theazz/awless-ro/aws/credentials
ok  	github.com/theazz/awless-ro/aws/fetch
ok  	github.com/theazz/awless-ro/aws/image
ok  	github.com/theazz/awless-ro/aws/services
?   	github.com/theazz/awless-ro/aws/tailers [no test files]
ok  	github.com/theazz/awless-ro/cloud
ok  	github.com/theazz/awless-ro/cloud/match
?   	github.com/theazz/awless-ro/cloud/properties [no test files]
ok  	github.com/theazz/awless-ro/cloud/rdf
ok  	github.com/theazz/awless-ro/commands
ok  	github.com/theazz/awless-ro/config
ok  	github.com/theazz/awless-ro/console
ok  	github.com/theazz/awless-ro/database
ok  	github.com/theazz/awless-ro/fetch
?   	github.com/theazz/awless-ro/gen/aws [no test files]
?   	github.com/theazz/awless-ro/gen/aws/generators [no test files]
ok  	github.com/theazz/awless-ro/graph
?   	github.com/theazz/awless-ro/graph/resourcetest [no test files]
?   	github.com/theazz/awless-ro/inspect [no test files]
ok  	github.com/theazz/awless-ro/inspect/inspectors
?   	github.com/theazz/awless-ro/logger [no test files]
ok  	github.com/theazz/awless-ro/ssh
ok  	github.com/theazz/awless-ro/sync
?   	github.com/theazz/awless-ro/sync/repo [no test files]
ok  	github.com/theazz/awless-ro/triplestore
```

```sh
make generate && git diff --exit-code
```
`git diff --stat` after `make generate` shows only the three hand-edited files
(`console/displayer.go`, `console/displayer_test.go`,
`graph/resourcetest/resourcetest.go`) plus the new `console/sorter_test.go` —
nothing under `aws/fetch`, `aws/services`, `cloud/properties`, `cloud/rdf`
changed. Generated output is unperturbed.

## Shuffle proof (N5, and the TestMaxWidth leak it surfaced)

Before any fix (stash of the fix applied, i.e. production + test code at the
pre-fix state), a shuffle run with a recorded seed reproduced order-dependent
pollution — but from `TestMaxWidth`, not `TestNoHeadersDisplay` (`console/
displayer_test.go`'s only global-map mutator; `TestMaxWidth` mutates two other
package vars with the identical missing-restore pattern):

```
$ go test ./console/ -shuffle=on -count=1 -timeout 60s -v
-test.shuffle 1791380108951968000
--- FAIL: TestTabularDisplays (0.00s)
    displayer_test.go:195: got
        | ID ▲  | NAME  | STATE | TYPE  | PUBLIC |
        ...
        want
        |  ID ▲  |  NAME  |  STATE  |   TYPE    | PUBLIC IP |
        ...
FAIL	github.com/theazz/awless-ro/console	0.462s
```

(`TestMaxWidth` left `autowrapMaxSize`/`tableColWidth` at the narrow values it
sets mid-test, 4/4, and `TestTabularDisplays` ran after it in that shuffle
order and inherited them.) Confirmed pre-existing on the pristine worktree
(before any change in this PR, including the bool fix) with the same seed —
it is not something this PR introduced.

Fix: `t.Cleanup` restore in both `TestNoHeadersDisplay` and `TestMaxWidth`.

Re-running the exact failing seed after the fix:

```
$ go test ./console/ -shuffle=1791380108951968000 -count=1 -timeout 60s -v
ok  	github.com/theazz/awless-ro/console	0.492s
```

Four additional fresh shuffle runs after the fix, all passing (seeds
recorded):

```
-test.shuffle 1791380239671347000   ok   0.428s
-test.shuffle 1791380240477142000   ok   0.290s
-test.shuffle 1791380249281680000   ok   0.212s
-test.shuffle 1791380250285388000   ok   0.208s
```

Two more after restoring from a git stash round-trip (sanity that nothing
regressed in the restore):

```
-test.shuffle 1791380999351764000   ok   1.241s
-test.shuffle 1791381005237982000   ok   0.218s
```

## Binary-level verification

Fixture at `$H/.awless-ro/aws/rdf/default/eu-west-1/infra.nt` (synthetic
data, no AWS account involved):

```
<sub-pub> <cloud:id> "sub-pub" .
<sub-pub> <cloud:name> "public-subnet" .
<sub-pub> <net:cidr> "10.0.1.0/24" .
<sub-pub> <cloud:public> "true"^^<xsd:boolean> .
<sub-pub> <rdf:type> <cloud-owl:Subnet> .

<sub-priv> <cloud:id> "sub-priv" .
<sub-priv> <cloud:name> "private-subnet" .
<sub-priv> <net:cidr> "10.0.2.0/24" .
<sub-priv> <cloud:public> "false"^^<xsd:boolean> .
<sub-priv> <rdf:type> <cloud-owl:Subnet> .
```

Harness:

```sh
env -i HOME=$H PATH=/usr/bin:/bin AWS_REGION=eu-west-1 AWS_DEFAULT_REGION=eu-west-1 \
    AWS_EC2_METADATA_DISABLED=true ./awless-ro list subnets --local --sort public
```

### Before (binary built from the pre-fix worktree state, via a git stash of
    the fix)

```sh
$ env -i HOME=$H ... ./awro-before list subnets --local --sort public
EXIT=2
```
```
panic: can not compare values of type bool

goroutine 1 [running]:
github.com/theazz/awless-ro/console.valueLowerOrEqual(...)
	console/displayer.go:1024 ...
github.com/theazz/awless-ro/console.(*defaultSorter).sort.func2(...)
	console/displayer.go:963 ...
github.com/theazz/awless-ro/console.(*tableDisplayer).Print(...)
	console/displayer.go:553 ...
github.com/theazz/awless-ro/commands.printResources(...)
	commands/list.go:189 ...
```
Matches the stack trace in the evidence file
(`.agents/tasks/upstream-bug-validation.md`) exactly.

### After (binary built from the fixed worktree)

```sh
$ env -i HOME=$H ... ./awro-fixed list subnets --local --sort public
EXIT=0
```
```
|    ID    |      NAME      |    CIDR     | ZONE | DEFAULT | VPC | PUBLIC ▲ | STATE |
|----------|----------------|-------------|------|---------|-----|----------|-------|
| sub-priv | private-subnet | 10.0.2.0/24 |      |         |     | false    |       |
| sub-pub  | public-subnet  | 10.0.1.0/24 |      |         |     | true     |       |
```

```sh
$ env -i HOME=$H ... ./awro-fixed list subnets --local --sort PUBLIC   # upper case, as reported
EXIT=0
```
Same table, same order — the upper-case spelling from the original report no
longer panics either.

---

# Iteration 2 — strict weak ordering for the sort fallback

Review finding (confirmed): the non-panicking fallback in
`console/displayer.go` compared `fmt.Sprint(a) <= fmt.Sprint(b)`, so values
that print alike but are not equal (`int(1)` / `string("1")`) were each
"lower" than the other, leaving ascending and `--reverse` order undefined.

Commit: `efbba7fa fix(console): make the sort fallback a strict weak ordering`
(second commit on `fix/console-sort-panic`, on top of `fae4ad27`).

## Analysis

1. How the result is used. `defaultSorter.sort` (`console/displayer.go`)
   builds `compare(i, j)`: for each sort column it `continue`s when the two
   cells are `reflect.DeepEqual`, otherwise returns `valueLowerOrEqual(a, b)`;
   it returns `false` when every column is equal. `--reverse` is
   `compare = func(i, j) { return asc(j, i) }`: operands swapped, not the
   result negated. So `valueLowerOrEqual` is only ever reached by the sorter
   on non-DeepEqual pairs, and "or equal" is never observable there; what
   matters is that for those pairs it is never true in both directions and is
   transitive. A strict `<` for distinct values cannot flip into a non-strict
   order under `--reverse`, because reverse is `less(b, a)`, not
   `!less(a, b)`.
   Typed cases on distinct values: `int`, `string` `<=` are strict on
   distinct values; `bool` `!a || b` is true only for (false, true);
   `time.Time` `a.After(b)` is strict (same instant in different
   representations: equivalent, transitively). Two typed cases were not
   strict weak orderings:
   - `[]string` / `[]int` used `fmt.Sprint <=` too: `[]string(nil)` vs
     `[]string{}` and `[]string{"a b"}` vs `[]string{"a", "b"}` print alike
     and were "lower" both ways (same defect as the finding). The in-progress
     change left this case on `<=`.
   - `float64` NaN: `NaN <= x` is false both ways, so NaN was equivalent to
     every number, and `1.5 ~ NaN ~ 2` with `1.5 < 2` breaks transitivity of
     incomparability (sort.Slice's requirement).
2. Type-key collisions. `typeOrderKey` (package path + name, or
   `reflect.Type.String()` for unnamed types) is not unique: same-named types
   declared inside different functions of one package share path and name,
   and unnamed types render package names, not paths. So the escape hatch is
   reachable. The fallback now breaks the tie on the printed form and then on
   the Go-syntax `%#v` form, which separates e.g. local `type collide int` (1)
   from local `type collide string` ("1"). The naturally ordered types (`int`,
   `float64`, `string`, `bool`, `time.Time`) have keys no other type can
   produce, so a tie never mixes a natural order with the fallback. Values
   identical on all three keys (a twin local `type collide int` holding 1) are
   equivalent, which is still a strict weak ordering; they print identically.
3. nil and equal values. The short-circuits at the top of `valueLowerOrEqual`
   are unchanged: `nil, nil` → true, comparable `a == b` → true, `nil` first.
   `TestCompareInterface` (`valueLowerOrEqual(1, 1) == true`) and the nil /
   equal-bool sorter cases still pass. The default branch answers
   `reflect.DeepEqual(a, b) || fallbackLess(a, b)`, so equal non-comparable
   values (e.g. two equal `[]string`) keep the "or equal" answer for direct
   callers.

## Code changes (this iteration)

- `console/displayer.go`
  - New `fallbackLess(a, b)`: lexicographic, strict, on (type key, printed
    form, `%#v` form). Used by the mismatched-type branch and by `default:`.
  - `case []string, []int:` removed; those slices go through `default:`
    (`DeepEqual || fallbackLess`). Order is unchanged wherever printed forms
    differ.
  - `case float64:` sorts NaN first; NaN is never lower than another NaN.
  - `typeOrderKey` comment corrected (keys are not unique; ties are broken by
    `fallbackLess`). Doc comment on `valueLowerOrEqual` states the contract.
- `console/sorter_test.go`
  - `TestSortFallbackOnPrintedKeyCollision` is now table-driven over four
    colliding pairs: `1`/`"1"`, two distinct local types sharing a type key,
    `[]string(nil)`/`[]string{}`, `[]string{"a b"}`/`[]string{"a", "b"}`. Each:
    exactly one direction is lower; ascending and `--reverse` through
    `defaultSorter` are mirror images; both repeated 20 times with the input
    rows in both orders.
  - New `TestSortLessIsStrictWeakOrdering`: over 32 mixed values (nil, ints,
    floats incl. two NaNs and -Inf, strings, bools, times, nil/empty/colliding
    slices, an unknown struct type, three same-key local types) checks
    irreflexivity, asymmetry, transitivity and transitive incomparability of
    the sorter's less, ascending and descending.

No user-visible change for any column whose values print differently or have
a natural order; the change only fixes the order of rows that previously had
none (undefined sort.Slice result).

## Red check: the new tests against the previous comparator

The two new tests copied with the package into a throw-away directory, with
`console/displayer.go` taken from `HEAD` before this commit (`fae4ad27`) plus
a copy of `typeOrderKey` so the test compiles. Directory removed afterwards.

```
$ go test ./zz_redcheck/ -count=1 -run 'TestSortFallbackOnPrintedKeyCollision|TestSortLessIsStrictWeakOrdering'
--- FAIL: TestSortFallbackOnPrintedKeyCollision (0.00s)
    --- FAIL: TestSortFallbackOnPrintedKeyCollision/int_and_string_printing_as_1 (0.00s)
    --- FAIL: TestSortFallbackOnPrintedKeyCollision/distinct_types_sharing_a_type_key (0.00s)
    --- FAIL: TestSortFallbackOnPrintedKeyCollision/nil_and_empty_[]string (0.00s)
    --- FAIL: TestSortFallbackOnPrintedKeyCollision/[]string{"a_b"}_and_[]string{"a",_"b"} (0.00s)
--- FAIL: TestSortLessIsStrictWeakOrdering (0.03s)
    --- FAIL: TestSortLessIsStrictWeakOrdering/ascending (0.01s)
    --- FAIL: TestSortLessIsStrictWeakOrdering/descending (0.01s)
FAIL
FAIL	github.com/theazz/awless-ro/zz_redcheck	0.255s
FAIL
exit=1
```

Counts and samples of the failure lines (269 in total):

```
== not asymmetric: 72
        sorter_test.go:346: not asymmetric: 1 and "1" are both lower than each other
        sorter_test.go:346: not asymmetric: 1 and 1 are both lower than each other
== not transitive: 192
        sorter_test.go:350: not transitive: 1 < "1" < 1 but not 1 < 1
        sorter_test.go:350: not transitive: 1 < 1 < 1 but not 1 < 1
== incomparability not transitive: 24
        sorter_test.go:353: incomparability not transitive: 1.5 ~ NaN ~ 2 but not 1.5 ~ 2
        sorter_test.go:353: incomparability not transitive: 1.5 ~ NaN ~ -Inf but not 1.5 ~ -Inf
== not irreflexive: 0
== not antisymmetric: 4
        sorter_test.go:249: comparator is not antisymmetric: 1 and "1" are both lower than each other
        sorter_test.go:249: comparator is not antisymmetric: 1 and "1" are both lower than each other
== must compare as equivalent: 1
```

Slice collisions in the same run:

```
        sorter_test.go:249: comparator is not antisymmetric: []string(nil) and []string{} are both lower than each other
        sorter_test.go:249: comparator is not antisymmetric: []string{"a b"} and []string{"a", "b"} are both lower than each other
        sorter_test.go:346: not asymmetric: []string(nil) and []int(nil) are both lower than each other
        sorter_test.go:346: not asymmetric: []string(nil) and []string{} are both lower than each other
        sorter_test.go:346: not asymmetric: []string{"a b"} and []string{"a", "b"} are both lower than each other
        sorter_test.go:346: not asymmetric: []string{"a", "b"} and []string{"a b"} are both lower than each other
        sorter_test.go:346: not asymmetric: []string{} and []int(nil) are both lower than each other
        sorter_test.go:346: not asymmetric: []string{} and []string(nil) are both lower than each other
```

(The "1 and 1" lines are `int(1)` against the local `collide` types, which
print as `1`.)

## Full gate (working tree = this commit's content)

Toolchain note: the local toolchain is go1.27.1; `go.mod` targets `go 1.26.0`.

```
$ go version
go version go1.27.1 darwin/arm64
$ go build ./...
exit=0
$ go vet ./...
exit=0
$ gofmt -s -l .
exit=0 (empty output above = clean)
```

```
$ go test ./... -race -count=1
ok  	github.com/theazz/awless-ro	25.104s
ok  	github.com/theazz/awless-ro/aws/config	4.965s
ok  	github.com/theazz/awless-ro/aws/conv	6.428s
ok  	github.com/theazz/awless-ro/aws/credentials	4.671s
ok  	github.com/theazz/awless-ro/aws/fetch	3.643s
ok  	github.com/theazz/awless-ro/aws/image	2.693s
ok  	github.com/theazz/awless-ro/aws/services	5.470s
?   	github.com/theazz/awless-ro/aws/tailers	[no test files]
ok  	github.com/theazz/awless-ro/cloud	4.021s
ok  	github.com/theazz/awless-ro/cloud/match	5.961s
?   	github.com/theazz/awless-ro/cloud/properties	[no test files]
ok  	github.com/theazz/awless-ro/cloud/rdf	3.099s
ok  	github.com/theazz/awless-ro/commands	7.921s
ok  	github.com/theazz/awless-ro/config	7.117s
ok  	github.com/theazz/awless-ro/console	8.814s
ok  	github.com/theazz/awless-ro/database	8.342s
ok  	github.com/theazz/awless-ro/fetch	6.492s
?   	github.com/theazz/awless-ro/gen/aws	[no test files]
?   	github.com/theazz/awless-ro/gen/aws/generators	[no test files]
ok  	github.com/theazz/awless-ro/graph	6.573s
?   	github.com/theazz/awless-ro/graph/resourcetest	[no test files]
?   	github.com/theazz/awless-ro/inspect	[no test files]
ok  	github.com/theazz/awless-ro/inspect/inspectors	6.394s
?   	github.com/theazz/awless-ro/logger	[no test files]
ok  	github.com/theazz/awless-ro/ssh	8.786s
ok  	github.com/theazz/awless-ro/sync	6.299s
?   	github.com/theazz/awless-ro/sync/repo	[no test files]
ok  	github.com/theazz/awless-ro/triplestore	6.383s
exit=0
```

Generated files, before the commit (hand-edited files excluded from the diff since they were not yet committed):

```
$ git status --short (before generate)
 M console/displayer.go
 M console/sorter_test.go
?? .agents/tasks/review.json
?? .agents/tasks/review.md
?? .agents/tmp/
$ make generate && git diff --exit-code
Generating commands code: runtime, doc, etc.
[+] generated aws/fetch/gen_apis.go
[+] generated aws/fetch/gen_fetchers.go
[+] generated aws/services/gen_services.go
[+] generated aws/services/gen_mocks_test.go
[+] generated cloud/properties/gen_properties.go
[+] generated cloud/rdf/gen_rdf.go
exit=0
$ git status --short (after generate)
 M console/displayer.go
 M console/sorter_test.go
?? .agents/tasks/review.json
?? .agents/tasks/review.md
?? .agents/tmp/
```

Generated files, after the commit (literal command, whole tree):

```
$ git rev-parse --short HEAD
efbba7fa
$ make generate && git diff --exit-code
Generating commands code: runtime, doc, etc.
[+] generated aws/fetch/gen_apis.go
[+] generated aws/fetch/gen_fetchers.go
[+] generated aws/services/gen_services.go
[+] generated aws/services/gen_mocks_test.go
[+] generated cloud/properties/gen_properties.go
[+] generated cloud/rdf/gen_rdf.go
exit=0
$ git diff --stat master...HEAD
 .agents/tasks/verification.md      | 220 ++++++++++++++++++++++
 console/displayer.go               |  80 +++++++-
 console/displayer_test.go          |  14 ++
 console/sorter_test.go             | 366 +++++++++++++++++++++++++++++++++++++
 graph/resourcetest/resourcetest.go |   8 +
 5 files changed, 684 insertions(+), 4 deletions(-)
```

(`.agents/tasks/verification.md` in the stat above was committed by `fae4ad27`; this update to it is left uncommitted.)

## Shuffle runs (six fresh seeds, plus the seed that failed before the N5/TestMaxWidth fix)

```
$ go test ./console/ -shuffle=on -count=1 -v  (run 1; -shuffle line and result shown)
-test.shuffle 1791386094650223000
PASS
ok  	github.com/theazz/awless-ro/console	0.216s
$ go test ./console/ -shuffle=on -count=1 -v  (run 2; -shuffle line and result shown)
-test.shuffle 1791386095317774000
PASS
ok  	github.com/theazz/awless-ro/console	0.216s
$ go test ./console/ -shuffle=on -count=1 -v  (run 3; -shuffle line and result shown)
-test.shuffle 1791386095965650000
PASS
ok  	github.com/theazz/awless-ro/console	0.227s
$ go test ./console/ -shuffle=on -count=1 -v  (run 4; -shuffle line and result shown)
-test.shuffle 1791386096604157000
PASS
ok  	github.com/theazz/awless-ro/console	0.209s
$ go test ./console/ -shuffle=on -count=1 -v  (run 5; -shuffle line and result shown)
-test.shuffle 1791386097245141000
PASS
ok  	github.com/theazz/awless-ro/console	0.219s
$ go test ./console/ -shuffle=on -count=1 -v  (run 6; -shuffle line and result shown)
-test.shuffle 1791386097896419000
PASS
ok  	github.com/theazz/awless-ro/console	0.217s
$ go test ./console/ -shuffle=1791380108951968000 -count=1 -v
-test.shuffle 1791380108951968000
PASS
ok  	github.com/theazz/awless-ro/console	0.211s
$ go test ./console/ -shuffle=1791380108951968000 -count=1
ok  	github.com/theazz/awless-ro/console	0.217s
exit=0
```

## Binary before/after

Both binaries built with `go build -trimpath`: master from `git archive master` extracted into a scratch directory (master untouched), the branch from this worktree. Synthetic fixture at `$H/.awless-ro/aws/rdf/default/eu-west-1/infra.nt`, `$H` a fresh scratch directory (no AWS data, no credentials):

```
<sub-pub> <cloud:id> "sub-pub" .
<sub-pub> <cloud:name> "public-subnet" .
<sub-pub> <net:cidr> "10.0.1.0/24" .
<sub-pub> <cloud:public> "true"^^<xsd:boolean> .
<sub-pub> <rdf:type> <cloud-owl:Subnet> .
<sub-priv> <cloud:id> "sub-priv" .
<sub-priv> <cloud:name> "private-subnet" .
<sub-priv> <net:cidr> "10.0.2.0/24" .
<sub-priv> <cloud:public> "false"^^<xsd:boolean> .
<sub-priv> <rdf:type> <cloud-owl:Subnet> .
```

```
$ [master] env -i HOME=$H PATH=/usr/bin:/bin AWS_REGION=eu-west-1 AWS_DEFAULT_REGION=eu-west-1 AWS_EC2_METADATA_DISABLED=true ./awless-ro list subnets --local --sort public

 █████╗  ██╗    ██╗ ██╗     ██████  ██████╗ ██████╗     
██╔══██╗ ██║    ██║ ██║     ██╔══╝  ██╔═══╝ ██╔═══╝    
███████║ ██║ █╗ ██║ ██║     ████╗   ██████  ██████   
██╔══██║ ██║███╗██║ ██║     ██╔═╝       ██╗     ██╗   
██║  ██║ ╚███╔███╔╝ ██████╗ ██████╗ ██████║ ██████║   
╚═╝  ╚═╝  ╚══╝╚══╝  ╚═════╝ ╚═════╝ ╚═════╝ ╚═════╝
                                        read-only
Welcome! Resolving environment data...

Found existing AWS region 'eu-west-1'. Setting it as your default region.

All done. Enjoy!
You can review and configure awless-ro with `awless-ro config`

Now running: `awless-ro list subnets`
panic: can not compare values of type bool

goroutine 1 [running]:
github.com/theazz/awless-ro/console.valueLowerOrEqual({0x10704e968?, 0x105399ee8?}, {0x10704e968, 0x105399ee0})
	github.com/theazz/awless-ro/console/displayer.go:1024 +0x338
github.com/theazz/awless-ro/console.(*defaultSorter).sort.func2(0x1, 0x0)
	github.com/theazz/awless-ro/console/displayer.go:963 +0xc4
sort.insertionSort_func({0x513d2fb9f5b0?, 0x513d2f8fb710?}, 0x0, 0x2)
	sort/zsortfunc.go:12 +0xac
sort.pdqsort_func({0x513d2fb9f5b0?, 0x513d2f8fb710?}, 0x18?, 0x1070616b8?, 0x513d2f941a28?)
	sort/zsortfunc.go:73 +0x258
sort.Slice({0x107061820?, 0x513d2f9e60d8?}, 0x513d2fb9f5b0)
	sort/slice.go:29 +0xc0
github.com/theazz/awless-ro/console.(*defaultSorter).sort(0x10?, {0x513d2f8fb6e0?, 0x1052e2eb9?, 0x1?})
	github.com/theazz/awless-ro/console/displayer.go:973 +0x98
github.com/theazz/awless-ro/console.(*tableDisplayer).Print(0x513d2f580b40, {0x1073a2298, 0x513d2f592018})
	github.com/theazz/awless-ro/console/displayer.go:553 +0x388
github.com/theazz/awless-ro/commands.printResources({0x1073ec598, 0x513d2f42d750}, {0x1052e39bc?, 0x7?})
	github.com/theazz/awless-ro/commands/list.go:189 +0x2b4
github.com/theazz/awless-ro/commands.init.func4.1(0x513d2fa94f08?, {0x513d2f42e960?, 0x0?, 0x3?})
	github.com/theazz/awless-ro/commands/list.go:129 +0x434
github.com/spf13/cobra.(*Command).execute(0x513d2fa94f08, {0x513d2f42e900, 0x3, 0x3})
	github.com/spf13/cobra@v1.10.2/command.go:1019 +0x810
github.com/spf13/cobra.(*Command).ExecuteC(0x10748a138)
	github.com/spf13/cobra@v1.10.2/command.go:1148 +0x340
github.com/spf13/cobra.(*Command).Execute(0x1073f9888?)
	github.com/spf13/cobra@v1.10.2/command.go:1071 +0x1c
main.main()
	github.com/theazz/awless-ro/main.go:31 +0x24
EXIT=2

$ [master] env -i HOME=$H PATH=/usr/bin:/bin AWS_REGION=eu-west-1 AWS_DEFAULT_REGION=eu-west-1 AWS_EC2_METADATA_DISABLED=true ./awless-ro list subnets --local --sort PUBLIC
panic: can not compare values of type bool

goroutine 1 [running]:
github.com/theazz/awless-ro/console.valueLowerOrEqual({0x106c42968?, 0x104f8dee8?}, {0x106c42968, 0x104f8dee0})
	github.com/theazz/awless-ro/console/displayer.go:1024 +0x338
github.com/theazz/awless-ro/console.(*defaultSorter).sort.func2(0x1, 0x0)
	github.com/theazz/awless-ro/console/displayer.go:963 +0xc4
sort.insertionSort_func({0x4c2719a1f5b0?, 0x4c2719a88390?}, 0x0, 0x2)
	sort/zsortfunc.go:12 +0xac
sort.pdqsort_func({0x4c2719a1f5b0?, 0x4c2719a88390?}, 0x4c2719aa0000?, 0x0?, 0x4c2719a9e188?)
	sort/zsortfunc.go:73 +0x258
sort.Slice({0x106c55820?, 0x4c2719aa0000?}, 0x4c2719a1f5b0)
	sort/slice.go:29 +0xc0
github.com/theazz/awless-ro/console.(*defaultSorter).sort(0x10?, {0x4c2719a88360?, 0x104ed6eb9?, 0x1?})
	github.com/theazz/awless-ro/console/displayer.go:973 +0x98
github.com/theazz/awless-ro/console.(*tableDisplayer).Print(0x4c27191abb00, {0x106f96298, 0x4c271913a040})
	github.com/theazz/awless-ro/console/displayer.go:553 +0x388
github.com/theazz/awless-ro/commands.printResources({0x106fe0598, 0x4c271988aa10}, {0x104ed79bc?, 0x7?})
	github.com/theazz/awless-ro/commands/list.go:189 +0x2b4
github.com/theazz/awless-ro/commands.init.func4.1(0x4c2719882f08?, {0x4c271986afc0?, 0x0?, 0x3?})
	github.com/theazz/awless-ro/commands/list.go:129 +0x434
github.com/spf13/cobra.(*Command).execute(0x4c2719882f08, {0x4c271986af60, 0x3, 0x3})
	github.com/spf13/cobra@v1.10.2/command.go:1019 +0x810
github.com/spf13/cobra.(*Command).ExecuteC(0x10707e138)
	github.com/spf13/cobra@v1.10.2/command.go:1148 +0x340
github.com/spf13/cobra.(*Command).Execute(0x106fed888?)
	github.com/spf13/cobra@v1.10.2/command.go:1071 +0x1c
main.main()
	github.com/theazz/awless-ro/main.go:31 +0x24
EXIT=2

$ [master] env -i HOME=$H PATH=/usr/bin:/bin AWS_REGION=eu-west-1 AWS_DEFAULT_REGION=eu-west-1 AWS_EC2_METADATA_DISABLED=true ./awless-ro list subnets --local --sort public --reverse
panic: can not compare values of type bool

goroutine 1 [running]:
github.com/theazz/awless-ro/console.valueLowerOrEqual({0x106f16968?, 0x105261ee8?}, {0x106f16968, 0x105261ee0})
	github.com/theazz/awless-ro/console/displayer.go:1024 +0x338
github.com/theazz/awless-ro/console.(*defaultSorter).sort.func2(0x0, 0x1)
	github.com/theazz/awless-ro/console/displayer.go:963 +0xc4
github.com/theazz/awless-ro/console.(*defaultSorter).sort.func3(0x302351b61488?, 0x104fbc5e8?)
	github.com/theazz/awless-ro/console/displayer.go:970 +0x30
sort.insertionSort_func({0x302351b615e8?, 0x302351bbc450?}, 0x0, 0x2)
	sort/zsortfunc.go:12 +0xac
sort.pdqsort_func({0x302351b615e8?, 0x302351bbc450?}, 0x18?, 0x106f296b8?, 0x302351bbe268?)
	sort/zsortfunc.go:73 +0x258
sort.Slice({0x106f29820?, 0x302351a87350?}, 0x302351b615e8)
	sort/slice.go:29 +0xc0
github.com/theazz/awless-ro/console.(*defaultSorter).sort(0x10?, {0x302351bbc420?, 0x1051aaeb9?, 0x1?})
	github.com/theazz/awless-ro/console/displayer.go:973 +0x98
github.com/theazz/awless-ro/console.(*tableDisplayer).Print(0x30235149da40, {0x10726a298, 0x3023513b8040})
	github.com/theazz/awless-ro/console/displayer.go:553 +0x388
github.com/theazz/awless-ro/commands.printResources({0x1072b4598, 0x302351b76a10}, {0x1051ab9bc?, 0x7?})
	github.com/theazz/awless-ro/commands/list.go:189 +0x2b4
github.com/theazz/awless-ro/commands.init.func4.1(0x302351b6ef08?, {0x302351a39280?, 0x0?, 0x4?})
	github.com/theazz/awless-ro/commands/list.go:129 +0x434
github.com/spf13/cobra.(*Command).execute(0x302351b6ef08, {0x302351a39240, 0x4, 0x4})
	github.com/spf13/cobra@v1.10.2/command.go:1019 +0x810
github.com/spf13/cobra.(*Command).ExecuteC(0x107352138)
	github.com/spf13/cobra@v1.10.2/command.go:1148 +0x340
github.com/spf13/cobra.(*Command).Execute(0x1072c1888?)
	github.com/spf13/cobra@v1.10.2/command.go:1071 +0x1c
main.main()
	github.com/theazz/awless-ro/main.go:31 +0x24
EXIT=2

$ [branch] env -i HOME=$H PATH=/usr/bin:/bin AWS_REGION=eu-west-1 AWS_DEFAULT_REGION=eu-west-1 AWS_EC2_METADATA_DISABLED=true ./awless-ro list subnets --local --sort public
|    ID    |      NAME      |    CIDR     | ZONE | DEFAULT | VPC | PUBLIC ▲ | STATE |
|----------|----------------|-------------|------|---------|-----|----------|-------|
| sub-priv | private-subnet | 10.0.2.0/24 |      |         |     | false    |       |
| sub-pub  | public-subnet  | 10.0.1.0/24 |      |         |     | true     |       |
EXIT=0

$ [branch] env -i HOME=$H PATH=/usr/bin:/bin AWS_REGION=eu-west-1 AWS_DEFAULT_REGION=eu-west-1 AWS_EC2_METADATA_DISABLED=true ./awless-ro list subnets --local --sort PUBLIC
|    ID    |      NAME      |    CIDR     | ZONE | DEFAULT | VPC | PUBLIC ▲ | STATE |
|----------|----------------|-------------|------|---------|-----|----------|-------|
| sub-priv | private-subnet | 10.0.2.0/24 |      |         |     | false    |       |
| sub-pub  | public-subnet  | 10.0.1.0/24 |      |         |     | true     |       |
EXIT=0

$ [branch] env -i HOME=$H PATH=/usr/bin:/bin AWS_REGION=eu-west-1 AWS_DEFAULT_REGION=eu-west-1 AWS_EC2_METADATA_DISABLED=true ./awless-ro list subnets --local --sort public --reverse
|    ID    |      NAME      |    CIDR     | ZONE | DEFAULT | VPC | PUBLIC ▼ | STATE |
|----------|----------------|-------------|------|---------|-----|----------|-------|
| sub-pub  | public-subnet  | 10.0.1.0/24 |      |         |     | true     |       |
| sub-priv | private-subnet | 10.0.2.0/24 |      |         |     | false    |       |
EXIT=0

```
