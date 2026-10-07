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
