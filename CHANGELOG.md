# Changelog

## v0.3.0

**Re-run `sync` after upgrading:** Route 53 records get new ids in the local graph. (#41)

### Added

- `--filter key==value` matches the whole value; `key=value` still matches a substring. (#31)
- `--max-width N` on `list` and `show`; `0` means no limit. (#37)

### Changed

- Piped or redirected tables are no longer wrapped: one row per line. On a terminal,
  tables use its full width. (#37)
- `--format json` prints netmasks and route destinations as CIDR strings
  (`"10.0.0.0/16"`) instead of base64. (#35)
- An unreadable local graph file is an error (exit 1, naming the file) instead of an
  empty result. (#39)

### Fixed

- `--sort` on a boolean column, such as `list subnets --sort public`, no longer panics. (#33)
- `show` works for a name shared by two resource types, e.g. a keypair and a load
  balancer both named `prod`. (#41)
- Route 53 records with the same name and type (weighted, geolocation, or in different
  zones) are listed separately instead of merged. (#41)
- Wildcard DNS names show as `*.example.com` instead of `\052.example.com`. (#41)
- `--tag` matches IAM users and roles. (#41)
- A `--tag` value split at a comma is refused with a quoting hint instead of returning
  nothing. (#38)
- Graph files with very long lines, such as large IAM policies, load. (#39)

## v0.2.3

### Fixed

- Credentials exported in `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` are used when
  `~/.aws` is absent (CI, containers); they were ignored since v0.1.0. (#32)
- A first run with `--aws-profile` takes the region from the profile instead of
  prompting for one. (#32)

## v0.2.2

### Fixed

- **`tail` is listed in `--help` and offered by completion.** It had been hidden since
  it first appeared upstream, as an experiment, though it works and the README
  documents it. Its help now says what it does, with examples.
- **`tail scaling-activities` with no activity said nothing and exited 0**, which reads
  the same as a silent failure. It now says there were no scaling activities in the
  last six weeks (as long as autoscaling keeps history), on stderr so stdout stays the
  events alone. (Inherited.)
- **`tail scaling-activities --follow` returned at once when there was no activity
  yet** — exactly when one wants to wait for the first. It now polls from that moment
  on. An invalid `--frequency` is refused before any AWS call. (Inherited.)

### Documentation

- The README opens with a recorded demo, and says plainly what the tool is for and how
  it gets its data: some commands ask the AWS API every time, `show` and `inspect` work
  on a synced graph, and `--local` answers from that copy without calling AWS.

## v0.2.1

### Fixed

- **`list buckets` could fail on its first run with "no such host"**, and succeed on
  the second ([#10](https://github.com/theazz/awless-ro/issues/10), inherited from
  upstream). To keep only the current region's buckets it called `GetBucketLocation`
  for every bucket in the account, all at once — each to the bucket's own hostname,
  so as many simultaneous DNS lookups of different names. A cold resolver, typically
  behind a VPN, answered some of them "no such host", the SDK does not retry that, and
  the first failure ended the whole listing. S3 now filters by region itself
  (`ListBuckets` with `BucketRegion`): one call instead of one per bucket, and
  `GetBucketLocation` is no longer among the operations the tool can perform (61 now).
  This applies to everything that reads buckets: `list buckets`, `list s3objects`,
  `show`, `sync` and the bucket inspectors.
- **Per-item API calls are bounded.** Fetchers that make one call per bucket, task
  definition revision, IAM user, load balancer, queue, hosted zone or ECS cluster
  started all of them at once, which in a large account meant hundreds or thousands of
  requests in the same instant and throttling the SDK's retry budget could not absorb.
  At most eight are now in flight per fetcher. When one fails the rest are stopped
  instead of being left blocked forever, and the access key fetcher no longer races on
  its error flag.

### Performance

- **A first `sync` takes about half as long** ([#12](https://github.com/theazz/awless-ro/issues/12)).
  IAM users, groups, roles and managed policies come from one
  `GetAccountAuthorizationDetails` pagination, and its pages were fetched one after
  another; IAM caps a page by size, so an account with ~500 roles and ~560 policies
  took 25 sequential pages and most of the sync. Each entity type is now its own
  pagination, run side by side. Measured from an empty home on the same account:
  `sync` 31–47s → 15.5s, `list policies` 19s → 11–13s. Same call, same permission,
  same result.

### Dependencies

- AWS SDK for Go v2 service modules updated (patch and minor releases).

## v0.2.0

### Features

- Shell completion for **fish** and **PowerShell**, alongside bash and zsh. The zsh
  script can now be installed as a completion file (it starts with `#compdef`), so
  package managers can set it up; the Homebrew formula will install all three.
- Completion is cobra's own rather than a hand-written generator, and what it offers
  is computed in Go, the same for every shell: resource ids and names for `show`,
  instances for `ssh` (keeping a `user@` prefix), stacks for `tail stack-events`,
  regions and profiles for `switch`, `-r` and `-p`, config keys with their help and
  their values for `config set`, inspectors for `inspect -i`, vendors for
  `search images`, formats for `list --format`.
- Completion answers from local state only. It never calls AWS, never syncs and,
  on a machine where awless-ro has not run yet, never starts first-run setup.
  Profile completion skips `sso-session` and `services` sections of `~/.aws/config`,
  which are not profiles.

## v0.1.1

### Fixed

- **The first command you ran synced the whole account before doing its own work.**
  `awless-ro whoami` needs a single `GetCallerIdentity`; on a machine with no
  `~/.awless-ro` it took about thirty-five seconds instead of two, because nine
  services and several thousand resources were fetched first. Writing the region for
  the first time went through the same path as changing it, and a region change
  schedules a sync — reasonably, since the local graph is per-region — but the first
  run is the initial write, not a change. Nothing needed the sync: commands fetch what
  they need when they are not given `--local`, and a local copy is what `sync` is for.
  Changing the region later still syncs, and `aws.autosync` still turns that off.
  ([#7](https://github.com/theazz/awless-ro/issues/7), inherited from upstream.)

  Measured on the same account, from an empty home: `whoami` 37s → 3s,
  `list instances` 30s → 1s, `list users` 36s → 1s,
  `search images canonical --latest-id` 41s → 1s.

- **`--local` said "No results found." when nothing had been synced yet**, which
  answers a question about the account in a situation where the account was never
  read. It now says so and names the setting to change. Reachable before, but rarely:
  the first run used to sync, so there was usually something there.

## v0.1.0 — first release of awless-ro

Fork of [wallix/awless](https://github.com/wallix/awless) at `44e892b4`, unmaintained
since 2018. The version number restarts: continuing upstream's `v0.1.11` would imply a
compatibility that does not exist.

Upstream's own changelog is kept below, unchanged, as the provenance of everything
this inherits.

Everything in this entry is a difference from `44e892b4`. Where a defect was inherited
rather than introduced here, it says so — the distinction matters, because an
inherited defect means the same bug is in the last released `awless` binary.

### Removed

The write half is gone. It was both the larger maintenance burden and the part least
safe to run unmaintained against a live account: six years of AWS API drift sits
between that code and reality, and its failure mode is not a wrong listing but a wrong
change.

- The templating language, the `.aws` template format, its lexer, parser, compiler and
  runner.
- Every `create`, `update`, `delete`, `attach`, `detach`, `start`, `stop`, `restart`,
  `check` and `copy` command, and the generated one-liner CLI behind them.
- `awless log` and `awless revert`, and the BoltDB tables backing them. The database
  remains for configuration.
- The scheduler integration, and with it the `github.com/wallix/awless-scheduler`
  dependency.
- `awless web` and the embedded HTTP server.
- `gopkg.in/src-d/go-git.v4`: the sync kept the local graph in a git repository and
  committed each sync. Nothing read the history.
- The `pricer` inspector. It sent an inventory of the account's EC2 instances to
  `ec2-price.com` over plain HTTP. That domain **was not registered** when this fork
  was made — `whois` returned no match — so anyone could have taken the name and
  started collecting. Removed rather than repointed.
- The `defaultsDefinitions` config store. Every entry in it — `instance.type`,
  `instance.count`, `securitygroup.protocol`, `volume.device` and the rest — supplied
  a default parameter to one of the removed commands, so nothing read them any more. A
  setting the user can change with no effect is worse than no setting. The store
  itself is kept, because older local databases may hold the legacy `region` and
  `sync.auto` keys.

`awless-ro switch` and `awless-ro config set` remain: they write to your
configuration, not to AWS.

Not yet working:

- **`awless-ro ssh`** is disabled. The resolution and connection logic is largely
  sound — against a live account it resolved an instance name to its private IP
  through the local graph and reached the host — but `--local` panics, `--print-cli`
  connects instead of printing, and the host key prompt loops when there is no
  terminal. The command explains this rather than running.

### Read-only is enforced, not intended

Every AWS call goes through a narrow per-service interface; nothing else in the tree
holds an SDK client. `TestEveryAWSOperationIsARead` reflects over those interfaces and
fails if any method name does not begin with `Describe`, `Get`, `List` or `Head`, or
if a field is a concrete client rather than an interface. Currently 62 operations
across 18 services. Reflecting over the struct rather than a hand-written list means a
service added later is covered without anyone remembering.

This constrains the tool, not your credentials. An administrative profile is still an
administrative profile.

### Platform

- Go modules instead of `dep`; no `vendor/` directory. Module path
  `github.com/theazz/awless-ro`.
- Go 1.26.
- **AWS SDK v2**, from v1.10.27 (2017). The migration is behind a test written first:
  SDK v2 changes the value semantics the reflective property extraction assumes —
  scalars stop being pointers, list elements stop being pointers, enums become named
  string types — and none of that is a compile error. Without it the binary would have
  built, run, and listed resources with silently empty columns.
- `go.etcd.io/bbolt` instead of the archived `github.com/boltdb/bolt`.
- `golang.org/x/crypto` at v0.57.0, from a 2018 revision carrying three CVEs
  reachable from this code: CVE-2020-9283, CVE-2021-43565 and CVE-2022-27191, all in
  the SSH client path.
- `github.com/wallix/triplestore` replaced by an in-tree `triplestore/` package. The
  dependency was unmaintained and most of it was unused; a quarter of it covered what
  this tool needs. The replacement is 1246 lines at 86% coverage, and writing it
  surfaced six defects in the original, listed below.
- Binary, module and state directory renamed to `awless-ro` / `~/.awless-ro`.
  Sharing `~/.awless` with an installed upstream `awless` would mean sharing its
  graph, database and key directory.

### Defects fixed

**Wrong answers about your account.** All nine of these were found by running against
a live account; none was visible to a unit test, because each turns on something a
test cannot know about AWS: what the vendors actually name their images, what an
account actually contains, what AWS itself considers the right answer.

- `search images <vendor> --latest-id` returned a **specialised build instead of the
  ordinary image** for three of the six vendors: `al2023-ami-minimal-`,
  `debian-13-backports-`, `suse-…-sp6-chost-byos-`. Vendors publish these alongside
  the ordinary image, under a name sharing its prefix and sometimes seconds newer, and
  the newest match won. The SUSE case is the damaging one: `byos` means bring your own
  subscription, so that instance comes up unregistered, and `chost` is a container host
  rather than a general-purpose server — none of it visible in the printed id. The EC2
  name filter cannot express the difference, because its only wildcard matches hyphens
  too, so each vendor now also declares an anchored expression applied after the
  fetch. (Inherited: upstream's patterns had the same shape.)
- **Image ordering turned on a one-second publication gap.** Amazon Linux publishes
  kernel 6.1, 6.12 and 6.18 builds of one release within seconds, and the 6.1 build
  happened to land last, so it was reported as the latest while AWS's own
  `al2023-ami-kernel-default` pointer named 6.18. Images of one release are now ranked
  among themselves by name, and a release is dated by its newest image.
- **Every failure exited 0.** `main` discarded the error from the root command. For
  output built to be read by a program — `id=$(awless-ro search images canonical
  --latest-id)` — a failed lookup was indistinguishable from success with an empty id.
  (Inherited.)
- **The flag list printed over the error message.** cobra prints usage whenever a
  command returns an error, which is right for a mistyped flag and wrong for "no image
  matched": the message a person needs scrolled off the top. Usage is now silenced
  once the command line has parsed. (Inherited.)
- **`show <sns-topic>` exited 1 with an internal message.** SNS subscriptions were
  keyed by their endpoint, and for the `lambda` and `sqs` protocols an endpoint *is*
  another resource's ARN — so the subscription and the function became one graph node
  carrying two `rdf:type` triples, and everything holding only an id failed.
  `list subscriptions` kept working because it resolves by type first, which is how
  this survived to a live account. The same key also merged distinct subscriptions:
  one queue subscribed to two topics is two subscriptions and was one node.
  (Inherited.)
- **A disabled resource type was reported as `0`.** `-> dns: 2 zones, 0 record`
  against an account holding 84 records. Two types are off by default because they
  cost an API call per parent; zero is an answer about the account, and nothing had
  been looked at. Now `records: off (aws.dns.record.sync)`. The help text for those
  keys also said `(when empty: true)` while sitting beside the value `false`.
  (Inherited.)
- **`list infra --format porcelain` panicked.** Columns are chosen per resource type,
  a whole-service listing has no single type, so rows were built zero cells wide while
  the sorter ordered by column 0. (Inherited.)
- **`--columns id` produced a blank column** for whole-service listings. A name
  matching no declared column is read as a property name — the only way to reach
  properties no listing declares a column for, such as an instance's `Architecture` —
  and the name was guessed with `strings.Title`, so `id` became `Id` while the
  property is `ID`. Names now resolve through the generated property registry.
  (Inherited.)
- **A relation pointing outside the local graph stopped the traversal.** A node named
  by a relation but never fetched has no type, which is ordinary: relations cross
  services, and a service may be off, refused for want of permissions, or not synced.
  One security group applying to something unsynced was enough to end
  `inspect -i port_scanner` with "resource type not found" and no output at all. The
  direct listing methods already answered with `NotFoundResource`, so the same graph
  was fine through one door and fatal through another. (Inherited.)

**Data integrity.**

- **Composite ids merged distinct resources.** The id for a Route53 record or a
  CloudWatch metric is a hash of its fields, and the fields were concatenated without
  a separator and hashed with adler32. `("AWS/EC2", "CPUUtilization")` and
  `("AWS/EC2C", "PUUtilization")` produced the same id. Now NUL-separated and hashed
  with truncated SHA-256. (Inherited.)
- Six defects in the triple store, found while replacing it: N-Triples escaping
  handled only `\n` and `\r`, so a literal backslash-n in a value was written as a
  real newline and read back changed, and a language tag could truncate the value;
  the object of a triple was parsed from the left, which mis-split values containing
  the delimiter; `clone()` dropped a blank node's sub-node flag; `Resource()`
  reported blank nodes as resources; `Equal()` did not distinguish blank nodes;
  `TraverseDFS` swallowed the error from its visitor. The on-disk format was pinned by
  a golden-file test before the replacement, and both the pre-existing and current
  files are checked against it. (Inherited.)
- `inspect -i bucket_sizer` read properties with unchecked type assertions, so an
  object carrying no `Bucket`, or a `Size` that did not arrive as an `int`, took the
  process down. Its output was also ordered by map iteration, so the same data printed
  differently on every run. (Inherited.)

**Security.**

- The SSH `ProxyCommand` path wrote an executable shell script into the shared `/tmp`
  on every connection, at a predictable path, and then ran it — a symlink and a race
  away from executing someone else's code, and the script interpolated a hostname into
  a shell command line. Removed; the workaround it implemented was for an OpenSSH CVE
  fixed in 2016. Verified against `ssh -G` and `sh -c` that quoting now behaves.
- `awless-ro whoami`'s public-IP lookup went to an HTTP endpoint; now HTTPS.
- Private key handling: `x509.DecryptPEMBlock`, deprecated and not
  authenticated, replaced with `ssh.ParsePrivateKeyWithPassphrase`. Key files with
  permissions other than 0600 are refused rather than used. A keypair name containing
  `..` can no longer escape the keys directory.
- Directory creation errors were discarded, turning a permissions problem into a
  confusing failure to open a file in a directory that was never created. Modes are
  now explicit: 0700 for directories holding keys or an account's synced picture, 0600
  for the files.
- `show` on an instance no longer prints its `UserData`. Bootstrap scripts routinely
  carry tokens, and it was displayed in full, in a table, by default.

**Correctness of the credentials path.** (The prompt and cache are new here; upstream
resolved credentials differently.)

- The first run did not reach credential resolution at all when `~/.aws` was absent —
  exactly the case the prompt exists for — because a profile not being defined was
  treated as an error.
- The prompt echoed the secret access key as it was typed.
- With no terminal it looped forever instead of failing with a message.
- An existing profile section was appended to rather than left alone, producing a
  duplicate section that breaks the file for every reader.
- `errors.As` against `*SharedConfigProfileNotExistError` never matched, because the
  SDK declares `Error()` on the value and returns a value. The first version of the
  fix above looked right and silently did nothing.

### Behaviour changes worth knowing

- `--ids` prints ids only. It used to print each resource's name as well, on its own
  line between the ids, which a script cannot tell apart from an id. Shell completion,
  which wants both, now asks for both explicitly.
- `--columns` is honoured for whole-service listings, where it was accepted and
  ignored.
- `search images` is no longer a hidden command, and its deprecated `--id-only` flag
  is gone in favour of `--latest-id`.
- AMI search pins the publishing account with the `Owners` request parameter rather
  than an `owner-id` filter, and refuses a search that names no account
  (`ErrNoOwner`) before sending a request. Both are controls against the
  [whoAMI name confusion attack](https://securitylabs.datadoghq.com/articles/whoami-a-cloud-image-name-confusion-attack/),
  disclosed in 2025, which turned exactly this into code execution for tooling that
  fed the result to `run-instances`. This tool launches nothing, but it prints an id
  that a person will paste somewhere.
- AMI matching happens on the AWS side. Upstream fetched every public image an owner
  had ever published and filtered in Go with `strings.HasPrefix`; Canonical alone has
  tens of thousands.
- AMI vendors `coreos` and `centos` removed. CoreOS Container Linux reached end of
  life in 2020. CentOS Linux 7 did so in June 2024, and the account publishing CentOS
  Stream images could not be confirmed from a source belonging to the project or to
  AWS — the Marketplace listings under that name are third-party rebuilds. Shipping an
  unverified account id is the whoAMI mistake. Both are named in the error, with the
  `--owner`/`--name` escape hatch.
- `search images` accepts `arm64`. Upstream accepted only `i386` and `x86_64`;
  Graviton did not exist when that list was written.
- Vendor account ids and name patterns were re-verified, and two had drifted: Debian
  now publishes from `136693071363`, not `379101102735`, and Canonical's image names
  gained a storage-class segment (`hvm-ssd` became `hvm-ssd-gp3`).
- Region defaults, sync toggles and profile handling are unchanged, but settings that
  only affected removed commands are gone from `config list`.

### Supply chain and tooling

- CI pins every action to a commit SHA, declares least-privilege `permissions`, runs
  `go mod verify` and `govulncheck`, and gates on generated files being current. Each
  gate was checked by mutation — a deliberately broken tree must fail it.
- Releases publish `SHA256SUMS`, in a format `sha256sum -c` and `shasum -a 256 -c`
  both accept. Release artefacts are built for darwin and linux on amd64 and arm64,
  and windows on amd64; 386 was dropped.
- The release tool used to exit 0 having produced a partial release when a build
  failed, and replaced the environment wholesale so the module cache could not be
  found.
- [CONTRIBUTING.md](CONTRIBUTING.md) records the supply-chain rules for adding a
  dependency.
- Code generation is idempotent and verified as such in CI.

### Tests

`go test ./... -race` is green and no test requires AWS credentials, which is checked
by running with an empty `HOME` and no `AWS_*` variables.

Coverage where it matters: `aws/image` 98%, `cloud/match` 96%, `sync` 92%,
`triplestore` 86%, `aws/credentials` 80%, `aws/conv` 80%. The `commands` package had
no tests at all and now has some.

Every fix above is pinned by a test that was checked by mutation: restoring the old
behaviour must fail it. Several of these fixes exist *because* writing the test
surfaced the defect — the composite id collision and the N-Triples escaping both came
out that way.

---

# Upstream changelog (wallix/awless)

Everything below is upstream's changelog as of `44e892b4`, kept unchanged.

## v0.1.11 [2018-06-21]

**Check out our new article** on [Simplified Multi-Factor Authentication](https://medium.com/@awlessCLI/simplified-multi-factor-authentication-for-aws-d703e8d9f332) with `awless`

### Features

- [#71](https://github.com/wallix/awless/issues/71): Add support for Classic load-balancers:

```
    $ awless list classicloadbalancers
    $ awless create classicloadbalancer name=my-loadb subnets=[sub-123,sub-456] listeners=HTTP:80:HTTP:8080 healthcheck-path=/health/ping  securitygroups=sg-54321 tags=Env:Test,Created:Awless
    $ awless update classicloadbalancer name=my-loadb health-interval=10 health-target=HTTP:80/weather/ health-timeout=300 healthy-threshold=10  unhealthy-threshold=5
    $ awless attach classicloadbalancer name=my-loadb instance=@redis-prod-1
    $ awless delete classicloadbalancer name=my-loadb
```

- [#214](https://github.com/wallix/awless/issues/214): `AWS_PROFILE` env variable now loaded in `awless` in addition to the deprecated `AWS_DEFAULT_PROFILE` thanks to @alewando
- Better completion for `attach mfadevice` and `attach user` commands
- [#219](https://github.com/wallix/awless/issues/219): Validate access key and secret key before writing into `~/.aws/credentials` file

### Fixes

- [#220](https://github.com/wallix/awless/issues/220): Add double quotes to CSV output if needed thanks to @lllama
- Fix compilation error in templates with concatenation and reference (c.f. for example in [this template](https://gist.githubusercontent.com/fxaguessy/ef9511bf5ed8f3312904cccb96b818e8/raw/75c0f808220665441055b589be133cf711c64f37/ManageOwnMFA.aws))
- Parse integer beginning with '0' as string (preventing the deletion of the initial '0' for example in `... account.id=0123456789`)

## v0.1.10 [2018-04-13]

### Features

- Much better performance when synchronising all access data (IAM, etc.)
- Create instances now supports distro prompting for CentOS, Amazon Linux 2, CoreOS
   
      $ awless create instance name=myinst distro=amazonlinux:amzn2
      $ awless create instance distro=coreos
      $ awless create instance distro=centos name=myinst

- Avoiding extra throttling: Listing flag `--filter` now passes on the user wanted filtering down to the AWS API when possible so that _less unneeded resources are fetched_, _bandwidth is reduced_ and _some throttling avoided_.
  
  For example:
  
      $ awless ls s3objects --filter bucket=website
      $ awless ls records --filter name=io
      $ awless ls containertasks --filter name=my-task-definition-name

- Support for region embedded in an AWS profile (i.e. shared config files ~/.aws/{credentials,config}). See #181 in Fixes for more details 
      
- [#191](https://github.com/wallix/awless/issues/191) Attach a certificate to a listener with: `awless listener attach id=... certificate=...` (see awless attach listener -h for more)


### Fixes

- [#200](https://github.com/wallix/awless/issues/200): Now paging is supported for s3 objects when listing
- [#196](https://github.com/wallix/awless/issues/196): Regression fix SIGSEV when having AWS config with role assuming
- [#182](https://github.com/wallix/awless/issues/182): Region embedded in profile taken into account and given correct precedence
- [#144](https://github.com/wallix/awless/issues/144): Filtering done on AWS side when listing records for a given zone name
- [#172](https://github.com/wallix/awless/issues/172): Filtering done on AWS side when listing containertasks for a given task definition name

## v0.1.9 [2018-01-16]

**In this release, the local data model has been updated to support multi-account and stale data is removed when upgrading. Local data (ex: used for completion, etc...) will progressively be synced again through your usage of awless. Although, to get all your data now under the new model, you can manually run `'awless sync'`**	

### Features

- Support and seamless sync across multi-account (i.e. multiple profiles) and regions
- Enriched params prompting with optional/skippable but very common params. Can be disabled with `--prompt-only-required` or forced with `--prompt-all` to leverage smart completion for all params
- Automatically complete the username when deleting an access key by its ID, if it is contained in the local graph model:
    * `awless delete accesskey id=ACCESSKEYID`
-  For `awless update stack` param `stackfile` can now slurp yml and json params files. Thanks to @Trane9991 ([#167](https://github.com/wallix/awless/pull/167), [#145](https://github.com/wallix/awless/issues/145))
- Better completion for template parameters independently of their display name
- Aliases can now be resolved to properties other than IDs. For example, they are resolved to ARN in attach/detach/update/delete policy: `awless attach policy arn=@my-policy-name`
- Running only `awless switch` now returns your current region and profile, allowing a quick and short region/profile lookup
- Better completion of slice properties 

### AWS Services

- Listing of Route53 records now contains a new column for aliases [#181](https://github.com/wallix/awless/issues/181)
- Create an image from an existing instance. See `awless create image -h`
    * `awless create image instance=@my-instance-name name=redis-image  description='redis prod image'`
    * `awless create image instance=i-0ee436a45561c04df name=redis-image reboot=true`
    * List your images with `awless ls images --sort created`
    * Delete images with an `awless revert ...` or with `awless delete image id=@redis-image`
- [#169](https://github.com/wallix/awless/issues/169): Start/Stop a RDS database:
    * `awless start database id=my-db-id`
    * `awless stop database id=@my-db-name`
    * `awless restart database id=@my-db-name`
- Restart an EC2 instance
  * `awless restart instance id=id-1234`
  * `awless restart instance ids=@redis-prod-1,@redis-prod-2`
- [#176](https://github.com/wallix/awless/issues/176): Delete a DNS record only by its awless ID (see `awless ls records`) or by its name:
    * `awless delete record id=awls-39ec0618`
    * `awless delete record id=@my.sub.domain.com`

### Fixes

- Fix regression error: errors in dry run showed but where ignored hence user could wrongly confirm to run the template
- Delete a DNS record only by its awless ID

## v0.1.8 [2017-11-29]

### Features

- Better prompting of template parameters
- Overall better logging output of template execution

### AWS Services

- Create a database replica with: `awless create database replica=...`

## v0.1.7 [2017-11-24]

### Features

- Better prompt completion for template parameters
- Create instance/launchconfiguration from community distro names (`awless create instance distro=debian`). In default config value, deprecation of `instance.image` in favor of `instance.distro` (migration should be seamless).
    * `awless create instance distro=redhat:rhel:7.2`
    * `awless create launchconfiguration distro=canonical:ubuntu`
    * `awless create instance distro=debian`
- Quick way to switch to profiles and regions. Ex: `awless switch eu-west-1`, `awless switch mfa us-west-1`
- Create a public subnet in only one command with: `awless create subnet public=true...`
- Save directly your newly created access key in `~/.aws/credentials` with : `awless create accesskey save=true`
- Overall better logging output of template execution

### AWS Services

- Update Cloudfront distribution with: `awless update distribution...`

## v0.1.6 [2017-11-16]

**Overall re-design of AWS commands with full acceptance testing allowing for easier external contribution, greater flexibility and scalability moving forward**

### Features

- [#154](https://github.com/wallix/awless/issues/154): `awless ssh` allow specifying both `--port` and `--through-port`
- [#151](https://github.com/wallix/awless/issues/151): `awless ssh` using ip addresses. Ex: `awless ssh 172.31.68.49 --through 172.31.11.249`
- `awless attach mfadevice` now propose to automatically add the MFA device configuration to `~/.aws/config`
- [#158](https://github.com/wallix/awless/pull/158), [#159](https://github.com/wallix/awless/pull/159): Added bash/zsh completion to regions and profiles. Thanks to @padilo.

## v0.1.5 [2017-10-05]

### Features

- Complete flow to enable MFA for a user, including QRCode generation
- Much better output for `awless log`; default message (or user specified message) stored now in logs
- [#143](https://github.com/wallix/awless/issues/143): Follow CloudFormation stack events: `awless tail stack-events my-stack-name --follow`. Thanks to @Trane9991.
- Support concatenation between `{holes}` and `"quoted strings"` in template with `+` operator: `policy = create policy ... resource="arn:aws:iam::" + {account.id} + ":mfa/${aws:username}"`

### AWS Services

- Manage and listing of MFA devices: `awless create/delete/attach/detach mfadevice`, `awless list mfadevices`
- Support [Network Load Balancers](http://docs.aws.amazon.com/elasticloadbalancing/latest/network/introduction.html): `awless create loadbalancer .... type=network ...`
- Add conditions in policies and support multiple resources `awless create policy ... conditions=\"aws:MultiFactorAuthPresent==true\" resource=arn:aws:iam::0123456789:mfa/test,arn:aws:iam::0123456789:user/test`
- Add conditions in role creation `awless create role name=awless-mfa-role principal-account=0123456789 conditions=\"aws:MultiFactorAuthPresent==true\"`
- List the access keys of all users with `awless list accesskeys` (previously, only current user)
- Fetch role trust policy document: `awless show my-role`

### Fixes

- Exit code is now non zero on template run with KO states

## v0.1.4 [2017-09-21]

### Features

- Local storage of cloud data (RDF store) now done using the NTriples text format instead of a binary format (transition completely transparent for the user). New format allows more friendly git revisioning of data compared to a binary format.
- [#87](https://github.com/wallix/awless/issues/87): Customize columns displayed in `awless list` with `--columns`: `awless ls instances --sort name --columns name,vpc,state,privateip`
- Global `--no-sync` flag to not run any sync on command
- `awless show policy-name/policy-id` now displays the current policy Document (in JSON).

### AWS Services

- Update IAM policies, to add statements with `awless update policy`
- Add ACM certificates in infra:
    - `awless list certificates`
    - `awless create/delete/check certificate domains=my.firstdomain.com,my.seconddomain.com validation-domains=firstdomain.com,seconddomain.com`
- [#123](https://github.com/wallix/awless/issues/123): Listing route tables display the association IDs.

### Fixes
- `awless ssh --through`: no reusing same conn to avoid EOF. Bug: only first user (amazonlinux) was successful (usually ec2-user) !!
- `awless ssh --through`: on new proxy client catching error that where shadowed

## v0.1.3 [2017-09-06]

### Features

- `awless show` command 'not found' error now suggests if resource with same reference exists in other locally synced regions
- `awless` template language now supports lists, for example: `create loadbalancer subnets=[$subnet1, $subnet2]`
- Variables in `awless` template language now support references, holes and lists, for example: `mysecgroups = [$secgroup1, {my.secgroup},sg-123456]`
- `awless` template language now supports *holes* in strings, for example: `create instance name={prefix}database{version}`
- `awless update securitygroup` can now authorize/revoke access from another security group: `update securitygroup id=sg-12345 inbound=authorize portrange=any protocol=tcp securitygroup=sg-23456`
- Template CLI prompt: better TAB completion of resources and their properties
- Man CLI examples for all one liners command. For example, `awless create instance -h` will display relevant CLI examples
- Add `Type` (AWS/Customer managed) and `Attached` (true/false) columns in `awless list policies`
- [#129](https://github.com/wallix/awless/issues/129): flag `--color=always/never` to force enabling/disabling of colored output.

### AWS Services

- List network interfaces with `awless list networkinterfaces`

### Fixes

- Fix regression: listing a resource returned no results when this resource was disabled for sync. Listing should always fetch the resources and display what is on your cloud.
- [#130](https://github.com/wallix/awless/issues/130): Better exit status code in `awless show` command
- Port ranges starting from *0* to *n* are no longer processed as from *n* to *n*.
- `awless ssh --through`: works without an SSH agent running; correct StrictHostkeyChecking; correct display for `--print-config`

## v0.1.2 [2017-08-17]

### Features

- Sync overall speed up and massive reducing in memory consumption
- SSH `--through`: `awless ssh my-priv-inst --through my-pub-inst` allow you to connect to a private instance by going through a public one in ths same VPC. You need to have the same keypair (SSH key) on both instances. 
- Flag `--profile-sync` on `awless sync` to enable live profiling. Will dump `mem` and `cpu` Go profiling files for later inspection
- [#109](https://github.com/wallix/awless/issues/109): Support caching of STS credentials for Multi-Factor Authentication.
- [#126](https://github.com/wallix/awless/issues/126): Flag `--no-alias` in `awless show` force the display of IDs in relations.
- [#126](https://github.com/wallix/awless/issues/126): Reverse sorting when listing resources with flag `--reverse`
- [#120](https://github.com/wallix/awless/issues/120): Profile info is now included in execution logs and appended when suggesting revert action
- [#82](https://github.com/wallix/awless/issues/82): Better template TAB completion (e.g. complete list of parameters)


### AWS Services

- Instance Profiles: List them; attach them to an instance. Ex: `attach instanceprofile name=...`, `awless ls instanceprofiles`
- Replace in one command an InstanceProfile on a given instance with the `replace=true` param. Ex: `attach instanceprofile .... replace=true`
- Update Route53 records with `awless update record`

### Fixes

- [#116](https://github.com/wallix/awless/issues/116) No more sync Out Of Memory

## v0.1.1 [2017-07-06]

### Features

- Detach/Attach rapidly AWS policies to user, group or role with: `attach policy service=ec2 access=readonly group=sysadmin`. More info with `awless attach policy -h`
- Better template TAB completion: suggest on properties, suggest nothing if not relevant
- Create access keys: prompt user to potentially store them locally under a specific profile
- Conveniently prompting and storing locally (~/.aws/credentials) for AWS profile credentials when access keys not found
- `awless ssh`: support SSH agent thanks to @justone
- New `--port` flag for `awless ssh`: specifying non-standard SSH port thanks to @justone
- Use `--no-headers` flag in `awless list` to display the results without headers
- New flag `--values-for` in `awless show` to output machine readable values for resource properties. Ex: `awless show my_instance --values-for name,publicip`
- Sync works on best effort now. Meaning it does not bail out when an error happens (most often it can be an access right issues on some AWS services)
- `awless ls policies` now returns: your managed policies + all policies attached to any users, role or group
- Table display now use full terminal width when possible
- Much friendlier first install

### New AWS Services

- Support of EC2 NAT Gateways: `awless list natgateways` / `awless create/delete natgateway`
- Support [ECR](https://aws.amazon.com/ecr/) repositories and registry: `awless list repositories` / `awless create/delete repository` / `awless authenticate registry`
- Support [ECS](https://aws.amazon.com/ecs/) clusters, services, containerinstances and containers: `awless list containerclusters/containertasks/containerinstances` `awless attach/detach/delete/start/stop containertask`
- Create/Delete [ApplicationAutoScaling](http://docs.aws.amazon.com/ApplicationAutoScaling/latest/APIReference/Welcome.html) scalable target and policies: `awless create/delete appscalingtarget/appscalingpolicy`

### Bugfixes
- Template TAB completion: do not display non relevant id/name listing for each prompt
- Parse successfully template parameters starting with a digit

## v0.1.0 [2017-05-31]

## Features

- Add documentation for all template parameters (`awless create instance -h`, `awless update s3object -h`...)
- Listing with filter invalid keys: return error and help
- `awless whoami` now has flags to return specific account properties only: `--account-only`, `--id-only`, `--name-only`, `--resource-only`, `--type-only`
- Rename template parameters for standardization:
    - `delete keypair id=...` -> `delete keypair name=...`
    - `create listener target=...` -> `create listener targetgroup=...`
    - `delete database skipsnapshot=... snapshotid=...` -> `delete database skip-snapshot=... snapshot=...`
    - `delete dbsubnetgroup id=...` -> `delete dbsubnetgroup name=...`
    - `create queue maxMsgSize=... retentionPeriod=... msgWait=... redrivePolicy=... visibilityTimeout=...` -> `create queue max-msg-size=... retention-period=... msg-wait=... redrive-policy=... visibility-timeout=...`

## v0.0.25 [2017-05-26]

## Features

- [#98](https://github.com/wallix/awless/issues/98): `awless ssh` searches SSH keys in both `~/.awless/keys` and `~/.ssh` folders.
- When `awless ssh` in an instance, you can now specify only `-i keyname`, if the key is stored in `~/.awless/keys` or `~/.ssh`.
- [#99](https://github.com/wallix/awless/issues/99): Suggesting the right command when typing `awless create instance ID` or `awless create ID` rather than `awless create instance id=ID`
- Use a s3 bucket as a public website with `awless update bucket name=my-bucket-name public-website=true`
- Set/update buckets or s3objects predefined ACL (private / public-read / public-read-write / bucket-owner-read...): `awless update s3object acl=public-read`
- List CloudFront distributions: `awless list distributions`
- Create/Update/Check/Delete a CloudFront distribution: `awless create/update/check/delete distribution`
- List CloudFormation stacks: `awless list stacks`
- Create/Update/Delete a CloudFormation stack: `awless create/delete stack`
- `awless log --raw-json` shows the full info stored on template execution (context, fillers used, region, ...). Typically this contextual info can be reused for replay and updates of templates

## v0.0.24 [2017-05-22]

### Features

- Template author is now persisted in awless log using the caller identity
- [#93](https://github.com/wallix/awless/issues/93): Supporting EC2 tags: syncing locally; filtering in `awless list` with --tag, --tag-value, --tag-key
- [#84](https://github.com/wallix/awless/issues/84): Create AMI by importing VM image from S3: `awless import image bucket=my-bucket s3object=my-object`. Add template to create AMI from local VM file (OVA, VMDK ...): `awless run repo:upload_image`.
- Listing pending import image tasks with `awless list importimagetasks`
- Deleting images and optionally its related snapshots `awless delete image delete-snapshots=true`
- Create/Update/Delete login profiles (AWS Console credentials): `awless create/update/delete loginprofile username=...`
- Autowrapping results in tables when too long for `awless list`. No longer truncate results in `--format csv/tsv/json`
- Adjust the width of table columns to the terminal width in `awless show`
- Using local EC2 metadata to set region when installing awless on an EC2 instance
- [#94](https://github.com/wallix/awless/issues/94): Add short flags for `--aws-profile`: `-p` and `--aws-region`: `-r`

### Bugfixes

- Listing in CSV: remove extra spaces; proper listing in TSV (only 1 tab separator)
- Avoid double sync on first install due to pre defined default region value us-east-1
- [#92](https://github.com/wallix/awless/issues/92): Impossible to set a region in config when `aws.region` was empty
- [#89](https://github.com/wallix/awless/issues/89): Fix `awless whoami` when using STS credentials.

## v0.0.23 [2017-05-05]

### Features

- Create and attach role to a user or resource (instance, ...). See an [example](https://github.com/wallix/awless-templates#role-for-resource)
- Get my IP as seen by AWS: `awless whoami --ip-only`. Example: `awless create securitygroup ... cidr=$(awless whoami --ip-only)/32 ...`
- [#86](https://github.com/wallix/awless/issues/86): SSH using private IP with `--private` flag. Thanks @padilo.
- `awless ssh` now checks the remote host public key before connecting. Check can be disabled with the (insecure) `--disable-strict-host-keychecking` flag.
- [#74](https://github.com/wallix/awless/issues/74): support of encrypted SSH keys for generation `awless create keypair encrypted=true` and in `awless ssh`.
- Better documentation of [awless-templates](https://github.com/wallix/awless-templates); listing remote templates in awless with `awless run --list`.
- Friendlier (using units: B, K, M, G) display for storage size (s3objects, volumes, lambda functions)
- Better help for template parameters (ex: `awless create loadbalancer -h`)
- Create/delete and list Lambda functions: `awless list functions` / `awless create/delete function`
- Create/delete/attach/detach and list elastic IPs: `awless list elasticips` / `awless create/delete/attach/detach elasticip`
- Create/delete and list volume snapshots: `awless list snapshots` / `awless create/delete snapshot`
- Create/delete and list autoscaling launch configurations, scaling policies and scaling groups: `awless create/delete launchconfiguration/scalingpolicy/scalinggroup`. See an [example](https://github.com/wallix/awless-templates/#group-of-instances-scaling-with-cpu-consumption)
- Create/delete/start/stop/attach/detach and list cloudwatch alarms. List cloudwatch metrics: `awless list alarms/metrics`
- List EC2 images (AMIs) of which you are the owner: `awless list images`
- Copy an EC2 image from a given region to the current region: `awless copy image name=... source-id=... source-region=...`
- List your IAM access keys: `awless list accesskeys`

### Bugfixes

- Update SSH library to fix [CVE-2017-3204](http://www.cve.mitre.org/cgi-bin/cvename.cgi?name=2017-3204).
- Take the file name rather than full path as default name when uploading a s3object
- Correctly create repo on first install on machine with git not installed

## v0.0.22 [2017-04-13]

### Features

- Amazon [**userdata**](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/user-data.html) support. Give the data as local file or remote http file resource. Ex: `awless create instance userdata=/tmp/mydata.sh ...` or `awless create instance userdata=https://gist.github.com/jsmith/5f58272fa5406`.
- Global rename of `storageobject` to `s3object` for shorter typing in CLI.
- awless model/storing is now full RDF ;). Allow exploration of all your infra in RDF tools and ontology editor (Ex: [Protege](http://protege.stanford.edu/))
- Faster, better and simpler RDF & triples management now done through the nifty library [triplestore](https://github.com/wallix/triplestore)
- Ability to use strings with spaces and special characters in template parameters by surrounding them with single or double quotes.
- Loggers are now sent to the stderr file descriptor which makes easier piping and redirecting output.
- Warn when creating an instance without access key.
- ssh: print SSH configuration (`~/.ssh/config`) or the CLI one-liner to connect with SSH using `--print-config` or `--print-cli` flags.
- ssh: better handle when several instances have the same name (e.g., with a running and a terminated instance)
- ssh: more warning; provide help and context on failing connections
- Manage properly secgroups on instances with `awless attach/detach secgroup id=... instance=@my-instance`
- Logging more info when running templates

### Bugfixes

- `awless whoami` now supports displaying info for `root` user and user with org path
- Use `securitygroup` rather than `group` in templates, when appropriate.
- Use `keypair` rather than `key` in templates, when appropriate.
- Fix the fact you could not attach multiple security groups to an instance
- Reverting the creation of a load balancer now waits the deletion of its network interfaces

## v0.0.21 [2017-03-23]

### Features

- `awless whoami` now returns your identity, your attached (i.e. managed), inlined and group policies
- Rudimentary security groups port scanner inspector via `awless inspect -i port_scanner`
- Template: compile time check of undefined or unused references
- Run official remote templates without specifying full url: `awless run repo:create_vpc`
- [#78](https://github.com/wallix/awless/issues/78): Show progress when uploadgin object to storage
- [#81](https://github.com/wallix/awless/issues/81): Global force flag `--force` to bypass confirm prompt

### Bugfixes

- Fix regression: run templates/one-liners failed on `storageobject`, `subscription` entities
- Filtering in `awless list --filter` now works with column types other than string
- Users, groups and policies are now independent of the region
- [#83](https://github.com/wallix/awless/issues/83): Syncing while offline does not clear local cloud infra

## v0.0.20 [2017-03-20]

### Features

- Auto completion of id/name to help fill in easily any missing info before template execution
- Better error messaging on parsing template errors
- Infra: basic support of RDS: listing, creation and deletion of databases and database subnets:  `awless list databases/dbsubnetgroups`; `awless create/delete database/dbsubnetgroup`
- Infra: attach/detach an `instance` to a `targetgroup`
- Infra: delete tag: `awless delete tag`
- Access: create an AWS access key for a user
- DNS: allow to revert creation/deletion of records
- [#80](https://github.com/wallix/awless/issues/80) DNS: return the ChangeInfo id when creating/deleting a record

### Bugfixes

- [#79](https://github.com/wallix/awless/issues/79): `awless list records` do not add new lines between records.
- Better compute table columns width to adjust the number of columns to display exactly to the terminal width.

## v0.0.19 [2017-03-16]

### Features

- [#76](https://github.com/wallix/awless/issues/76): Show private IP and availability zones when listing instances.
- Run remote template when path prefixed with `http`. Ex: `awless run http://github.com/wallix/awless-templates/...`
- Fetch more instances properties when showing instances (ex: network interfaces, public and private DNS, Root device type and name...)
- DNS: listing Route53 zones and records `awless list zones/records`
- DNS: basic creation/deletion of Route53 zones and records `awless create/delete zone/record`
- Infra: detach EBS volumes `awless detach volume`
- Config: enable/disable the syncing of Route53 service `awless config set aws.dns.sync`
- All listing with default format are now Markdown table compatible. 
- Better display of `awless show`. Added `--siblings` flag to display exhaustively all siblings
- Reverse the sorting order when listing instances sorted by "up since"

### Bugfixes

- Fix `awless show` to properly show relations between groups and users

## 0.0.18 [2017-03-13]

### Features

- infra: support the creation/deletion of ELBv2 loadbalancers, listeners and target groups: `awless create loadbalancer/listener/targetgroup`
- infra: add tag `Name` to subnets.
- Format `tsv` supported when listing: `awless list subnets --format tsv`
- Pricer inspector now resolves prices for any regions: `awless inspect -i pricer`

### Bugfixes

- Fix alias, required and extra params parsing in template runs

## 0.0.17 [2017-03-09]

If you have any data or config issues, you can run `rm -Rf ~/.awless/` to start with a fresh install.

### Features

- [#65](https://github.com/wallix/awless/issues/65): `awless ssh`: use existing SSH client if available, otherwise fallback on builtin SSH.
- `awless show` resolves automatically on id, name or arn without any prefixing (previously it was '@')
- [#47](https://github.com/wallix/awless/issues/47): Enable/disable sync per services or resources through config. Ex: `awless config set aws.notification.sync false`, `awless config set  aws.storage.storageobject.sync true`.
- [#55](https://github.com/wallix/awless/issues/55): Dynamically change AWS region/profile with global flags `--aws-region us-west-1` or `--aws-profile myprofile`.
- [#73](https://github.com/wallix/awless/issues/73): `AWS_DEFAULT_REGION` env variable now loaded in `awless`. It takes precedence over `aws.region`.
- [#73](https://github.com/wallix/awless/issues/73): `AWS_DEFAULT_PROFILE` env variable now loaded in `awless`. It takes precedence over `aws.profile`.
- Better output of `awless config list` (doc per variable, etc.).
- Global default menu with clearer one-liner display.
- Simplification of the templating engine using decoupled compile passes.
- Config setters now provide dialogs (ex: `awless config set instance.type` or `awless config set aws.region`).
- [#54](https://github.com/wallix/awless/issues/54): `awless ssh`: specify the keyfile to use with `-i /path/toward/key` flag.
- [#64](https://github.com/wallix/awless/issues/64): `awless ssh`: columns and lines automatically adapt to terminal with/height.

- Attach/detach policy to user/group (see [wiki examples](https://github.com/wallix/awless/wiki/Examples))
- Attach/detach user to group (see [wiki examples](https://github.com/wallix/awless/wiki/Examples))
- List AWS load balancers, target groups and listeners with `awless list loadbalancers/targetgroups/listeners`. Show their relations with, e.g. `awless show LOAD_BALANCER`.

### Bugfixes

- [#12](https://github.com/wallix/awless/issues/12): Support AWS pagination when fetching resources in AWS IAM.
- Template parsing: allow digits in refs; allow regular chars in alias declaration
- Template: all aliases now resolves correctly from file or CLI. Ex: `awless create instance subnet=@my-subnet`

## 0.0.16 [2017-03-01]

### Features

- Allow simple fuzzy search for listing filters. Ex: `awless list instances --filter state=run`
- Revert: waiting instance termination when deleting a vpc/subnet/instance hierarchy.

### Bugfixes

- Fix regression: timeout too low for HTTP requests with AWS.

## 0.0.15 [2017-02-28]

As model/relations for resources may evolve, if you have any issues with models related commands, you can run `rm -Rf ~/.awless/aws/rdf` to start a fresh RDF model.

### Features

- [#6](https://github.com/wallix/awless/issues/6): Create Linux installer shell script: `curl https://raw.githubusercontent.com/wallix/awless/master/getawless.sh | bash`
- [#42](https://github.com/wallix/awless/issues/42), [#60](https://github.com/wallix/awless/issues/60), [#66](https://github.com/wallix/awless/issues/66): Better load AWS credentials (support profile credentials, MFA and crossaccount profile access)
- [#32](https://github.com/wallix/awless/issues/32): Basic support of [SNS](https://aws.amazon.com/sns/) (CRUD for topics and subscriptions)
- [#32](https://github.com/wallix/awless/issues/32): Basic support of [SQS](https://aws.amazon.com/sqs/) (CRUD for queues)
- [#53](https://github.com/wallix/awless/issues/53): Filter results in listings. Ex: `awless ls instances --filter state=running,"Access Key"=my-key` or the equivalent `awless list instances --filter state=running --filter "Access Key"=my-key`
- Better help menus by splitting one-liner template commands from general commands
- Run template: better dialog and remove noisy info
- Template validation: notify on unexpected params; check names unicity against local graph
- Log contextual error instead of hard failure when user has no rights to sync a service

### Bugfixes

- [#57](https://github.com/wallix/awless/issues/57): Properly fetch buckets when they are in the `us-east-1` region.
- [#12](https://github.com/wallix/awless/issues/12): Support AWS pagination when fetching resources in AWS SNS and EC2.

## 0.0.14 [2017-02-21]

As model/relations for resources may evolve, if you have any issues with models related commands, you can run `rm -Rf ~/.awless/aws/rdf` to start a fresh RDF model.

### Features

- [#39](https://github.com/wallix/awless/issues/38), [#38](https://github.com/wallix/awless/issues/33): Remove data collection & sending
- [#33](https://github.com/wallix/awless/issues/33): Ability to set AWS profile using `aws.profile` config key
- Better output for `awless sync`
- `awless ls` now an alias for `awless list`

### Bugfixes

- [#44](https://github.com/wallix/awless/issues/44): Fetch only the S3 buckets and related objects of the current region.
- [#52](https://github.com/wallix/awless/issues/52), [#34](https://github.com/wallix/awless/issues/34): Properly fetch route tables, even if a route contains several destinations.
- [#37](https://github.com/wallix/awless/issues/37): Load the region from database when initializing cloud services rather than `awless` environment.
- [#56](https://github.com/wallix/awless/issues/56): Do not require a VPC as parent of security groups nor route table.
