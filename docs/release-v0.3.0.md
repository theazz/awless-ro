# awless-ro v0.3.0

Exact-match filters, tables that behave in pipes, CIDR in JSON, and fixes to the local graph.

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
