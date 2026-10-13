# awless-ro v0.5.0

CloudWatch Logs log groups, and a `switch` that tells you when something overrides it.

**Breaking:** `switch` with `-p` or `-r` now exits 1 without saving anything. Pass the
profile and region as arguments instead: `awless-ro switch staging eu-west-1`. (#60)

### Added

- `list loggroups`: CloudWatch Logs log groups with class, retention, size and creation
  time. Synced with the rest of monitoring (`aws.monitoring.sync`). (#59, thanks @babyhuey)

### Fixed

- `switch` warns when `AWS_PROFILE`, `AWS_DEFAULT_PROFILE`, `AWS_DEFAULT_REGION` or the
  profile's own region overrides what it saved, and says how to make it apply.
  (#57, thanks @jay23606; #60)
