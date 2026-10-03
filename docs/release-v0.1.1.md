# awless-ro v0.1.1

A speed fix, and it is a large one: the first command you run no longer reads the
whole account before doing its own work.

## The first run was slow for no reason

`awless-ro whoami` needs a single `GetCallerIdentity`. On a machine with no
`~/.awless-ro` it took about thirty-five seconds, because nine services and several
thousand resources were synced first.

Writing the region for the first time went through the same code path as changing it,
and a region change does schedule a sync — reasonably, since the local graph is
per-region, so the one already on disk describes somewhere else. But the first run is
the initial write, not a change, and nothing distinguished the two.

Nothing needed that sync. `list`, `search`, `whoami` and `tail` ask AWS directly, and
`show` and `inspect` fetch what they need on their own; `sync` is for working offline.
The README said as much, so the first run was contradicting the documentation.

Measured against the same account, from an empty home:

| command | before | after |
|---|---|---|
| `whoami` | 37s | 3s |
| `list instances` | 30s | 1s |
| `list vpcs` | 32s | 1s |
| `list users` | 36s | 1s |
| `search images canonical --latest-id` | 41s | 1s |
| `ssh …` (refused, as in v0.1.0) | 35s | 0s |

Changing the region later still syncs, and `aws.autosync false` still turns that off.

## `--local` now says when there is nothing to read

"No results found." answers a question about the account. Asked before anything has
been synced, the account was never read, and those are not the same thing. It now
says which profile and region have nothing, and what to do:

```
$ awless-ro list instances --local
[info]    nothing has been synced for profile 'default' in region 'eu-west-1' yet, so
          --local has nothing to read. Run `awless-ro sync`, or drop --local to ask
          AWS directly.
```

This was reachable in v0.1.0 too, but rarely: the first run used to sync, so there was
usually something there. Removing that sync makes an empty local copy the normal
starting state, so the message had to be right.

## Unchanged

Everything else, including `ssh`, which is still disabled —
[#1](https://github.com/theazz/awless-ro/issues/1) tracks the three defects that have
to be fixed first.

No change to any command, flag, output format or exit code, which is why this is a
patch release. Full detail in
[CHANGELOG.md](https://github.com/theazz/awless-ro/blob/master/CHANGELOG.md).

## Verified

Every command was run on a fresh home against a live account, with the v0.1.0 binary
and this one side by side: identical exit statuses, identical output, no panics. The
only differences are the timings above.
