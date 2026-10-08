# awless-ro v0.2.3

Credential and region resolution fixes for machines without `~/.aws`.

### Fixed

- Credentials exported in `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` are used when
  `~/.aws` is absent (CI, containers); they were ignored since v0.1.0. (#32)
- A first run with `--aws-profile` takes the region from the profile instead of
  prompting for one. (#32)
