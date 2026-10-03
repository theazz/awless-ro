# Contributing to awless-ro

awless-ro is a read-only fork of [wallix/awless](https://github.com/wallix/awless),
which has been unmaintained since 2018. The fork exists to keep the good parts working
on a current AWS SDK, with everything that could change infrastructure removed.

## Getting set up

Go 1.26 and nothing else. There is no dependency manager to install and no `vendor/`
directory.

```sh
git clone https://github.com/theazz/awless-ro
cd awless-ro
make test      # go test ./... -race
make build     # generate, test, then compile ./awless-ro
```

`make generate` regenerates the generated files. Run it after changing anything under
`gen/`, and commit the result.

## What belongs here and what does not

The tool reads. It does not create, update or delete anything, and it does not run
templates. That is the product decision, not a stage on the way somewhere. Concretely,
a change is out of scope if it:

- calls an AWS API that mutates state;
- reintroduces the `.aws` templating language, `log`/`revert`, or the `history`
  command;
- sends anything to a host that is not AWS or the user's own. An earlier inspector
  posted the account's instance types to a third-party pricing site, and by the time
  we looked the domain was no longer registered — anyone could have claimed it and
  collected the inventory. Inspectors read the local graph.

## Before opening a pull request

CI runs these, so it is quicker to run them yourself:

```sh
gofmt -s -l .          # must print nothing
go vet ./...
go build ./...
go test ./... -race
make generate && git status --porcelain   # must print nothing
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

## Dependencies and the supply chain

This project inherited its dependency list from a repository abandoned in 2018. One
of those dependencies was itself abandoned, and one endpoint it talked to had expired.
Treat a new dependency as something you will have to own.

**Never work around checksum verification.** `go.sum` is committed, and every module is
checked against it and against `sum.golang.org` on the way in. That check is the only
thing standing between a build here and a dependency that was swapped out upstream. Do
not use, and do not suggest in an issue, any of:

| Setting | What it does |
|---|---|
| `GOSUMDB=off` | turns the checksum database off outright |
| `GONOSUMDB=*` / `GOPRIVATE=*` | exempts every module from it |
| `GOFLAGS=-mod=mod` | lets a build rewrite `go.mod` instead of failing on a mismatch |
| `GOPROXY=direct` with `GOSUMDB=off` | fetches straight from source with nothing checked |
| `GOINSECURE=*` | allows plain HTTP fetches. It does not disable the checksum database, but it is the same instinct |

If `go mod verify` fails or a hash does not match, that is the mechanism working. Find
out why the hash changed. `go mod verify` runs in CI for that reason: a rule that only
lives in this file is a rule nobody notices breaking.

Adding a dependency:

- prefer the standard library, then a module that is actually maintained;
- pin what you add; `go mod tidy` and commit both `go.mod` and `go.sum`;
- check the name character by character. A module whose path is one letter away from a
  popular one is the oldest trick there is;
- say in the pull request what it is for and what it pulls in transitively.

GitHub Actions are pinned by commit SHA, not by tag, because a tag can be moved after
someone has reviewed it. The readable version goes in a trailing comment, and
Dependabot keeps both in step. Do not replace a SHA with a tag.

## Generated code

`gen/aws/*_definitions.go` is the source of truth. Files named `gen_*.go` are output:
editing one is pointless, because the next `make generate` overwrites it, and CI fails
the build if a generated file does not match its definitions.

To change generated output, change the definitions or the templates in
`gen/aws/generators/`, then run `make generate`. The generator formats what it writes,
so there is no separate formatting step and no need for `goimports`.

## Language

English, everywhere a person reads: documentation, code comments, commit messages,
issues, pull requests, and the strings the tool prints. Not a preference about
English — a repository where some of the explanation is in a language a reader does
not have is worse than one with less explanation, because the reader can see that
something is being withheld and cannot tell whether it mattered.

The exception is test data. `graph/testdata/*.nt` and the literal tables in
`graph/ntformat_test.go` and `triplestore/ntriples_test.go` carry non-ASCII strings in
several scripts on purpose: they exist to prove that multi-byte UTF-8 survives being
written to N-Triples and read back. Changing which scripts appear there would weaken
the test, and `testdata/legacy.nt` cannot be changed at all — it is a captured
artifact of the old triple store, and nothing can regenerate it.

## Code style

`gofmt -s`, and the standard library's sense of naming. Two things this project cares
about more than most:

**Comments explain why, not what.** The code says what it does. A comment earns its
place by recording the reason, the alternative that was rejected, or the bug that a
piece of awkward-looking code exists to avoid. Several comments here describe a defect
in the implementation that was replaced; those are there so the defect does not come
back.

**A test must be able to fail.** When you add one, break the thing it covers and watch
it go red. The audit that produced much of the current test suite found an SSH quoting
bug that had survived for years behind a test file with no case for that code path,
and a serialiser that corrupted policy documents with nothing asserting the format at
all.

## Security

Report anything exploitable through
[**Security → Report a vulnerability**](https://github.com/theazz/awless-ro/security/advisories/new),
which is a private channel between you and the maintainer, rather than in an issue.
Private reporting is enabled on this repository for exactly this.

A reply may take a few days. If the finding is already public elsewhere, say so in the
report — that changes how urgent it is.

When touching code that handles credentials, keys or the synced graph, the existing
expectations are: state under `~/.awless-ro` is `0700`, files in it are `0600`,
secrets are read without echo and never logged, and nothing executable is written to a
shared directory. `sync/permissions_test.go` and the tests in `aws/credentials` pin
most of that.

## Versioning and releases

[Semantic Versioning 2.0.0](https://semver.org/), tags `vMAJOR.MINOR.PATCH`. The
public API is the command line — commands, flags, output formats and exit codes — so a
change that breaks any of those is a major bump (a minor one while the version is
`0.y.z`). A pull request that changes user-visible behaviour adds a line to
`CHANGELOG.md` under an `Unreleased` heading at the top, created if absent.

A release: set `Version` in `config/version.go`, move the changelog entries under the
new version, commit, tag `vX.Y.Z` on that commit, then build with `release.go`, which
refuses a dirty tree. The tag is what `go install …@latest` resolves, so never move or
reuse one: a mistake gets a new patch version.

## Licence

Apache 2.0, as upstream. See `LICENSE`, and `NOTICE` for the original copyright.
