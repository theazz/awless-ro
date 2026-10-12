# awless-ro v0.4.0

`ssh` is back, finds your key by fingerprint, and says which users it tries.

### Added

- `ssh` works again: directly, through a jump host with `--through`, offline with
  `--local`; `--print-cli` and `--print-config` print without connecting. (#47)
- `ssh` finds the private key by the key pair's fingerprint, so the local file no
  longer has to be named like the key pair. (#49)
- `ssh` announces each guessed login user on stderr (`trying user ubuntu...`) and
  ends with a hint when none is accepted. (#49)

### Security

- `ssh` never offers the `ssh-rsa` (SHA-1) or DSA host-key algorithms. (#47)
