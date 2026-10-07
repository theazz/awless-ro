# awless-ro v0.2.2

Two fixes to how a run works out which credentials and which region to use. Both are
the same mistake in opposite directions, and both bite hardest on a machine where
awless-ro has never run.

## Credentials in the environment were ignored

On a machine with no `~/.aws` — a CI job, a container, a fresh shell — every command
failed:

```
[error]   AWS credentials for profile "default" are configured but cannot be used:
          failed to get shared config profile, default
```

or walked past the keys entirely and tried to reach EC2 instance metadata. Which is
awkward, because the tool's own advice tells you to export exactly those variables:

```
no AWS credentials found for profile "default".
Set them up with `aws configure --profile default`, or point awless-ro at another
profile with --aws-profile, or export AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY
```

The default profile was always pinned on the AWS SDK. A pinned profile is exclusive:
the SDK resolves from that profile alone and stops consulting the environment, so with
no shared config file to read there was nothing left to try.

The profile is now pinned only when one was actually chosen:

- `--aws-profile` / `-p`
- `AWS_PROFILE` or `AWS_DEFAULT_PROFILE`
- a configured `aws.profile` naming something other than `default`

With nothing chosen, the SDK's documented order applies — environment, then
`~/.aws/{credentials,config}`, then container and instance roles.

A profile you *did* ask for and that does not exist is still reported by name. It does
not fall through to keys in the environment: the profile decides which account's
resources are read and where the local graph is kept, so answering a request for one
account with another account's credentials would be worse than failing.

## The first run asked for a region the profile already named

With a `~/.aws/config` like this and no `[default]` section:

```ini
[profile beta]
region = us-east-2
```

a first `awless-ro -p beta list instances` asked which region to use:

```
Please enter one region: (Ctrl+C to quit, Tab for completion)
>
```

and stored the answer as the default region, for a profile that had already said what
it meant. In a script, with no terminal to ask, the run could not get past it.

The region that a first run needs was resolved before the chosen profile was taken
into account. It is now resolved through that profile, and the selector appears only
when nothing — profile, environment, or instance metadata — names a region. A profile
name that does not exist is not an error here: the environment and the shared default
still get to answer.

## Also

- `awless-ro -p <profile>` keeps reporting where the region came from, unchanged:
  `region precedence: 'us-east-2' loaded through profile 'beta'`.

No change to any command, flag, output format or exit code, hence a patch release.
Full detail in
[CHANGELOG.md](https://github.com/theazz/awless-ro/blob/master/CHANGELOG.md).

## Verified

Offline tests cover each case: keys in the environment with no `~/.aws` and with a
shared config that has no `[default]` section; an explicitly chosen profile winning
over keys in the environment, through both `-p` and `AWS_PROFILE`; a chosen profile
that does not exist being named rather than skipped; and a first run with `-p` taking
the region from the profile without reaching the interactive selector. None of them
needs AWS credentials or network access.

Driven end to end against a built binary with an empty home and fake key material,
neither reported symptom appears. Checked against a live account too, read-only.
