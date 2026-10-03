# awless-ro v0.1.0

A CLI for **looking at** an AWS account: readable tables instead of JSON, resources
by name instead of by id, how they relate to each other, and output you can pipe.
Optionally, a local copy of the account to explore offline.

It cannot change anything — and that is enforced, not promised. Every AWS operation
the binary is capable of calling is a `Describe`, `Get`, `List` or `Head`, and a test
fails the build if that stops being true.

This is a fork of [wallix/awless](https://github.com/wallix/awless), unmaintained
since 2018 and no longer buildable. The read half aged well and has no close
equivalent; the write half — templates, hundreds of `create`/`delete` commands,
`revert` — was both the larger maintenance burden and the part least safe to run
unmaintained against a live account. So the read half was kept and modernised, and
read-only became a property of the tool.

## Try it

```sh
brew install theazz/tap/awless-ro
awless-ro list instances
awless-ro show my-database
```

No setup step: `list` asks AWS directly, and `show` fetches what it needs. Existing
`~/.aws` profiles are picked up; anything missing is prompted for on first run. State goes to `~/.awless-ro`, so an installed upstream `awless` is untouched.

## What works

**`list`** — 49 resource types across EC2, IAM, S3, RDS, AutoScaling, SNS, SQS,
Route53, CloudWatch, CloudFormation, Lambda, ECS, ECR, ELB (classic and v2),
CloudFront and ACM.

```sh
awless-ro list instances --filter state=running --sort uptime
awless-ro list instances --columns name,state,architecture,lifecycle
awless-ro list volumes --tag-key Dept --format tsv
awless-ro list users --format json
awless-ro list vpcs --ids
```

`--columns` reaches any property a resource carries, not only the default columns.
Formats: `table`, `csv`, `tsv`, `json`, `porcelain`. `--ids` is ids only, one per
line.

**`show`** — one resource by id, by name, or by `@name`, with its properties, its
lineage of parents and children, what it applies to, what depends on it, and its
siblings. Names are not unique in AWS, so an ambiguous one lists the candidates.

**`search images`** — official AMIs by vendor, pinned to the publishing account and
resolving to the vendor's ordinary image rather than a specialised build.

```sh
awless-ro search images canonical --latest-id
awless-ro search images redhat::9
awless-ro search images --owner 123456789012 --name 'my-base-*'
```

**`inspect`** and **`tail`** — `port_scanner` (security groups opening ports, and to
what they are attached), `open_buckets`, `bucket_sizer`; CloudFormation stack events
and autoscaling activities.

**`whoami`**, **`switch`** (region and profile), **`config`**, **`completion`**,
**`version`**.

**`sync`** — optional. Fetches all nine services in parallel into a local graph
under `~/.awless-ro`, after which any command with `--local` answers from it without
calling AWS. A service you lack permission for is reported and skipped; the rest still
land. Two resource types are off by default because they cost an API call per parent,
and the sync says so rather than reporting zero of them.

## What does not work yet

**`awless-ro ssh`** is disabled in this release. The resolution and connection logic
is largely sound — against a live account it resolved an instance name to its private
IP through the local graph and reached the host — but `--local` panics, `--print-cli`
connects instead of printing, and the host key prompt loops when there is no
terminal. The command explains this instead of running. Until then, `show` tells you
what to connect to and `ssh` does the rest.

The write half is gone for good: templates, `create`/`delete`/`attach`/…, `log`,
`revert`, the scheduler, `awless web`.

## On read-only, precisely

Every AWS call goes through a narrow per-service interface. Nothing else in the tree
holds an SDK client, so the methods on those interfaces are the complete list of
operations the binary can perform — currently 62 across 18 services.
`TestEveryAWSOperationIsARead` reflects over them and fails the build if a name is not
a read, or if a field is a concrete client rather than an interface.

The rest of the security posture, and what is checked automatically, is in the
[README](https://github.com/theazz/awless-ro#security-checks).

## Notable if you used awless

- Failures exit non-zero. Upstream discarded the error from the root command, so
  everything exited 0.
- `--ids` prints ids only; it used to interleave names, which a script cannot tell
  apart from an id.
- AMI vendors `coreos` and `centos` are gone, the first end-of-life since 2020 and
  the second unverifiable — shipping an unverified account id is the mistake the
  whoAMI attack relies on.
- The `pricer` inspector is gone. It posted an inventory of your EC2 instances to a
  third-party host over plain HTTP, and that domain **was not registered** when this
  fork was made.
- Version restarts at v0.1.0; continuing upstream's v0.1.11 would imply a
  compatibility that does not exist.

Full accounting, including the nine defects found by running against a live account
and the inherited ones among them, is in
[CHANGELOG.md](https://github.com/theazz/awless-ro/blob/master/CHANGELOG.md).

## Install

```sh
go install github.com/theazz/awless-ro@latest
```

Or download a binary below and verify it:

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing
```

Builds for macOS and Linux on amd64 and arm64, and Windows on amd64. Requires Go 1.26
if building from source.

## Status

`go test ./... -race` is green and no test needs AWS credentials. The tool is also
checked against live accounts, which is where the defects that matter turn up: nine
were found that way, and none of them was visible to a unit test.

The Windows and Linux binaries here are cross-compiled and have not been exercised on
those platforms.

Feature requests and pull requests are welcome, and bugs get fixed as time allows. If
you point this at your account and something looks wrong, that finding is useful —
please open an [issue](https://github.com/theazz/awless-ro/issues).
