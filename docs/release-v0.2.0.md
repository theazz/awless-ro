# awless-ro v0.2.0

Shell completion, redone: **fish** is new, **zsh** now works when installed by a package
manager, and what Tab offers is the same in every shell.

## What changed

Completion used to be the scheme inherited from upstream: a hand-written generator for
bash, a zsh script that emulated bash, and lookups written as bash functions. There was
no fish, and the zsh script could only be sourced — installed as a completion file, it
was silently ignored, which is why the Homebrew formula shipped bash only.

It is now cobra's own completion:

```sh
awless-ro completion bash|zsh|fish|powershell
```

and Tab completes, in every shell:

- `show` — resource ids and names from your local copy; `ssh` — instances, keeping a
  `user@` you typed; `tail stack-events` — stacks
- `switch`, `-r`, `-p` — regions and profiles
- `config get|set|unset` — keys with their description, then values for `set`
- `inspect -i`, `search images`, `list --format`

Commands that take no argument no longer offer file names.

## Installing it

**With Homebrew** the files are installed for bash, zsh and fish. fish picks them up on
its own; zsh and bash do once the shell is set up for Homebrew's completions — if Tab
already completes `brew`, it is. Otherwise see
[Homebrew's guide](https://docs.brew.sh/Shell-Completion).

**Otherwise:**

```sh
echo 'eval "$(awless-ro completion bash)"' >> ~/.bashrc          # bash, with bash-completion
awless-ro completion zsh > "${fpath[1]}/_awless-ro"                # zsh
awless-ro completion fish > ~/.config/fish/completions/awless-ro.fish
```

## Safe to press Tab

Completion answers from local state only — the synced graph, the config, `~/.aws`. It
never calls AWS, never syncs, and on a machine where awless-ro has not run yet it never
starts first-run setup: it offers nothing and writes nothing. A test runs the binary in
an empty home directory to hold it to that.

Profile completion also stops offering the `sso-session` and `services` sections of
`~/.aws/config`, which are not profiles.

## Compatibility

A new capability with nothing removed, hence a minor version. `awless-ro completion bash`
and `awless-ro completion zsh` still exist; if you load them with `source <(…)` or
`eval`, that keeps working.

`ssh` is still disabled — [#1](https://github.com/theazz/awless-ro/issues/1).

Full detail in
[CHANGELOG.md](https://github.com/theazz/awless-ro/blob/master/CHANGELOG.md).

## Verified

Unit tests for every completion and end-to-end tests on the built binary. By hand: fish
against a synced test graph; zsh registering the installed `_awless-ro` file through
`compinit`; bash loading the script (interactive use needs the bash-completion package,
as before).
