# awless-ro

A command line tool for **looking at** an AWS account. It syncs your cloud into a
local graph and lets you explore it — offline, by name instead of by id, with output
you can pipe.

It cannot change anything. Not by convention: every AWS operation it is capable of
calling is a `Describe`, `Get`, `List` or `Head`, and a test enforces that.

```sh
awless-ro sync
awless-ro list instances --local
awless-ro show my-database --local
awless-ro list volumes --filter state=available --format csv
awless-ro search images canonical --latest-id
```

## Why this exists

[`wallix/awless`](https://github.com/wallix/awless) was a genuinely good CLI, and it
has been unmaintained since 2018. It no longer builds: `dep` instead of modules, an
old Go, AWS SDK v1, and a vendor directory full of libraries that have since moved
on or been abandoned.

Its two halves aged very differently.

The **write** half — a templating language, hundreds of `create`/`delete`
one-liners, a log, `revert`, a scheduler — is the part you least want to run
unmaintained against a live account. Six years of AWS API drift sits between that
code and reality, and the failure mode is not a wrong listing, it is a wrong change.
It was also most of the maintenance burden.

The **read** half aged well, and still has no close equivalent. `aws ec2
describe-instances` gives you JSON about instances; it does not give you a graph
where an instance knows its subnet, its VPC, its security groups and what those
apply to, queryable offline. Nor does it let you say `show my-database` instead of
pasting an identifier.

So this fork keeps the read half, modernises it, and makes read-only a property of
the tool rather than a promise in a README.

That turns out to be the interesting part. A tool that provably cannot write is one
you can point at production, hand to someone new, or run in an account you are
nervous about — and the guarantee does not depend on how carefully you typed.

## Security checks

**In place now**

- *Read-only by construction.* Every AWS call goes through a narrow per-service
  interface and nothing else in the tree holds an SDK client, so the methods on those
  interfaces are the complete list of operations the binary can perform — currently 62
  across 18 services. `TestEveryAWSOperationIsARead` reflects over them and fails the
  build if any name is not a `Describe`, `Get`, `List` or `Head`, or if a field is a
  concrete client rather than an interface. Reflecting over the struct rather than a
  hand-written list means a service added later is covered without anyone remembering
  to.
- *No third-party egress.* The binary talks to AWS and nothing else. The upstream
  `pricer` inspector, which posted an EC2 inventory to an external host, is gone; the
  public-IP lookup in `whoami` uses HTTPS.
- *Secrets stay put.* `show` does not print instance `UserData`; secrets are read
  without echo and never logged; state under `~/.awless-ro` is `0700`, files in it
  `0600`, and nothing executable is written to a shared directory.
- *Audited SSH and key handling.* The shared-`/tmp` script behind `ProxyCommand` was
  removed, deprecated unauthenticated PEM decryption replaced, key files with loose
  permissions refused, and key names from AWS data can no longer escape the keys
  directory.
- *AMI search pinned to publishing accounts,* so a look-alike image name cannot be
  substituted ([whoAMI](https://securitylabs.datadoghq.com/articles/whoami-a-cloud-image-name-confusion-attack/)).
- *CI on every push and pull request:* `gofmt`, `go vet`, `go test -race`,
  `go mod verify` against `go.sum`, `govulncheck` for known vulnerabilities in
  reachable code, and a check that generated code matches its definitions.
- *Dependency review on pull requests,* blocking a new dependency that arrives with a
  known advisory or a copyleft licence. `govulncheck` answers a different question —
  whether vulnerable code is reachable — and misses a bad dependency nothing calls yet.
- *A weekly scan independent of any commit,* because an advisory published after the
  last push would otherwise go unnoticed for as long as the repository is quiet.
- *Releases built by GitHub Actions from the tag,* not on a workstation, re-running the
  test and vulnerability gates first — a tag can be pushed to a commit that never
  passed CI — and verifying the published checksums against the published files.
- *Supply chain:* GitHub Actions pinned by commit SHA, workflow token read-only,
  Dependabot updates for Go modules and actions, GitHub secret scanning with push
  protection, Dependabot security updates, and a `SHA256SUMS` file with every release.

**Planned, to run automatically**

- Static analysis (CodeQL and `gosec`) on every pull request.
- OpenSSF Scorecard for the repository's own security posture.
- Build provenance attestations and an SBOM attached to every release.

Found something exploitable? Please report it through
[Security → Report a vulnerability](https://github.com/theazz/awless-ro/security/advisories/new),
which is private between you and the maintainer, rather than in a public issue.

## Install

```sh
go install github.com/theazz/awless-ro@latest
```

Or download a binary from
[releases](https://github.com/theazz/awless-ro/releases) and verify it:

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing
```

Or build from source:

```sh
git clone https://github.com/theazz/awless-ro && cd awless-ro
go build -o awless-ro .
```

Requires Go 1.26. No configuration needed if you already use the AWS CLI: existing
`~/.aws/{credentials,config}` profiles are picked up, and anything missing is
prompted for on first run.

State lives in `~/.awless-ro` — deliberately not `~/.awless`, so an installed
upstream `awless` and this tool cannot overwrite each other's graph, database or
keys.

## What it does

**Sync into a local graph.** `awless-ro sync` fetches every supported service in
parallel and stores the result as N-Triples under `~/.awless-ro`. A service you lack
permission for is reported and skipped; the rest still land.

```
-> infra: 63 instances, 2 vpcs, 64 securitygroups, 179 networkinterfaces, ...
-> access: 8 users, 476 roles, 538 policies, 23 instanceprofiles, ...
-> storage: 76 buckets, s3objects: off (aws.storage.s3object.sync)
```

Two resource types are off by default because they cost an API call per parent — one
per bucket, one per hosted zone. The sync says so rather than reporting zero of them.

**List, offline.** 49 resource types across EC2, IAM, S3, RDS, AutoScaling, SNS, SQS,
Route53, CloudWatch, CloudFormation, Lambda, ECS, ECR, ELB, CloudFront and ACM.

```sh
awless-ro list instances --filter state=running --sort uptime --local
awless-ro list instances --columns name,state,architecture,lifecycle --local
awless-ro list volumes --tag-key Dept --format tsv --local
awless-ro list users --format json --local
```

`--columns` reaches any property a resource carries, not only the default columns.
`--format` covers `table`, `csv`, `tsv`, `json` and `porcelain`; `--ids` prints one
id per line and nothing else, for scripts.

**Show one resource, with its relations.** By id, by name, or by `@name`. Names are
not unique in AWS, so an ambiguous one lists the candidates.

```sh
awless-ro show i-0123456789abcdef0 --local
awless-ro show my-database --local
```

Output is the resource's properties, then its lineage (parents and children), then
what it applies to, depends on, and its siblings.

**Find official AMIs.** By vendor, pinned to the publishing account, resolving to
the vendor's ordinary image rather than a specialised build.

```sh
awless-ro search images canonical --latest-id
awless-ro search images redhat::9
awless-ro search images --owner 123456789012 --name 'my-base-*'
```

**Inspect and tail.**

```sh
awless-ro inspect -i port_scanner --local     # security groups opening ports, and to what
awless-ro inspect -i open_buckets --local
awless-ro tail stack-events my-stack
```

**Aliases.** Reference a resource by its `Name` tag anywhere an id is accepted.

**Switch region and profile.**

```sh
awless-ro switch eu-west-1
awless-ro switch my-profile eu-west-1
```

## What was removed

Gone, and not coming back:

- the templating language and `.aws` template files
- every `create`, `update`, `delete`, `attach`, `detach`, `start`, `stop` command
- `awless log` and `awless revert`
- the scheduler integration (`wallix/awless-scheduler`)
- `awless web` and the embedded HTTP server
- the `pricer` inspector, which posted an inventory of your EC2 instances to a
  third-party host whose domain **was not registered** when this fork was made

Not yet working:

- **`awless-ro ssh`**. The resolution and connection logic is there and mostly
  works, but `--local` panics, `--print-cli` connects instead of printing, and the
  host key prompt loops when there is no terminal. The command explains this instead
  of running. Tracked in [#1](https://github.com/theazz/awless-ro/issues/1).

## Differences you will notice if you used awless

- Binary and state directory are `awless-ro` / `~/.awless-ro`.
- Version restarts at `v0.1.0`. Continuing upstream's `v0.1.11` would imply a
  compatibility that does not exist.
- Failures exit non-zero. Upstream discarded the error from the root command, so
  everything exited 0.
- `--ids` prints ids only. It used to print each resource's name as well, on its own
  line, which a script cannot tell apart from an id.
- AMI vendor `coreos` and `centos` are gone: CoreOS Container Linux reached end of
  life in 2020, and the account publishing CentOS Stream images could not be verified
  from a source belonging to the project or to AWS. Shipping an unverified account id
  is the mistake the [whoAMI attack](https://securitylabs.datadoghq.com/articles/whoami-a-cloud-image-name-confusion-attack/)
  relies on.

The full accounting is in [CHANGELOG.md](CHANGELOG.md).

## Status and caveats

The tool is checked against live AWS accounts using
[docs/live-check.md](docs/live-check.md) — the checklist and what to look for. If you
point it at yours and something is wrong, the finding is useful: please open an issue.

`go test ./... -race` is green and no test needs AWS credentials.

## Versioning

Releases follow [Semantic Versioning 2.0.0](https://semver.org/) and are tagged
`vMAJOR.MINOR.PATCH`. The public API is the command line: commands, flags, output
formats (`csv`, `tsv`, `json`, `porcelain`, `--ids`) and exit codes.

- **PATCH** — bug fixes that do not change documented behaviour.
- **MINOR** — new commands, flags, resource types or services, backwards compatible.
- **MAJOR** — anything that breaks a documented command, flag or output format.

While the version is `0.y.z`, a breaking change bumps the **minor** version instead,
as SemVer allows. Every release has an entry in [CHANGELOG.md](CHANGELOG.md).

## Contributing

Feature requests and pull requests with improvements are very welcome — open an
[issue](https://github.com/theazz/awless-ro/issues) to suggest something or to
discuss a change before writing it. Bugs get fixed as time allows, on a best-effort
basis. The one thing that will not be accepted is anything that makes the tool
write: see the scope section in CONTRIBUTING.

[CONTRIBUTING.md](CONTRIBUTING.md) covers the layout, how code generation works, and
the supply-chain rules for dependencies.

## Licence and provenance

Apache 2.0, inherited from `wallix/awless` — copyright and licence retained, see
[NOTICE](NOTICE). This is a fork of
[wallix/awless](https://github.com/wallix/awless) at commit `44e892b4`, and its
history is preserved in full in this repository.
