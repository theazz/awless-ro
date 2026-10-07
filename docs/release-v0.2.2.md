# awless-ro v0.2.2

`tail` comes out of hiding, and `tail scaling-activities` says when there is nothing to
show.

## `tail` is listed

`tail` shows recent CloudFormation stack events or autoscaling activities, or follows
them. It had been hidden from `--help` and completion since it first appeared upstream,
as an experiment — though it works, and the README already documented it. It is now
listed, with help and examples:

```sh
awless-ro tail stack-events my-stack                 # the last 10 events of a stack
awless-ro tail stack-events my-stack --follow        # follow a deployment until it completes
awless-ro tail scaling-activities -n 20              # the last 20 autoscaling activities
awless-ro tail scaling-activities --follow           # wait for new ones
```

## `tail scaling-activities` with nothing to show

Two defects inherited from upstream:

- **No activity printed nothing and exited 0**, which reads the same as a command that
  failed silently. Autoscaling keeps six weeks of history, so in a quiet account this
  is the usual answer. It now says so — on stderr, so stdout stays the events alone.
- **`--follow` returned at once when there was no activity yet**, which is exactly when
  one waits for the first. It now polls from that moment on. An invalid `--frequency`
  is refused before any AWS call.

## README

The README now opens with a short recorded demo, and its "What it is good for" section
says plainly how the tool gets its data: `list`, `search`, `whoami` and `tail` ask the
AWS API every time; `show` and `inspect` work on a synced graph of the account, which
is where relations come from; `list`, `show` and `inspect` take `--local` to answer from
that copy without calling AWS.

No change to any command's output, flag or exit code beyond the above, hence a patch
release. Full detail in
[CHANGELOG.md](https://github.com/theazz/awless-ro/blob/master/CHANGELOG.md).

## Verified

Against a live account: `tail stack-events` prints the header and events, and fails
with exit 1 on a stack that does not exist; `tail scaling-activities` with no activity
prints the notice and exits 0; `--follow` keeps waiting.
