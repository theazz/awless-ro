# awless-ro v0.6.0

Eight more services to list: API Gateway v2, CloudTrail, DynamoDB, EFS, EKS, KMS, Secrets Manager and SSM Parameter Store.

### Added

- `list apis`: API Gateway v2 HTTP and WebSocket APIs. (#76, thanks @babyhuey)
- `list trails`: CloudTrail trails, each in its home region only. (#65, thanks @babyhuey)
- `list tables`: DynamoDB tables. (#71, thanks @babyhuey)
- `list filesystems`: EFS file systems. (#70, thanks @babyhuey)
- `list filesystems` shows the mount target count; `--columns zone` shows the zone of a
  One Zone file system. (#84, thanks @babyhuey)
- `list eksclusters`: EKS clusters. (#72, thanks @babyhuey)
- `list kmskeys`: KMS keys, off by default in `sync` (`aws.kms.sync`). (#73, thanks @babyhuey)
- `list secrets`: Secrets Manager secrets, metadata only, off by default in `sync`
  (`aws.secretsmanager.sync`). (#74, thanks @babyhuey)
- `list parameters`: SSM Parameter Store parameters, metadata only, off by default in
  `sync` (`aws.ssm.sync`). (#75, thanks @babyhuey)

### Changed

- `list loggroups` shows retention `never` instead of an empty cell for a group that keeps
  its events forever; JSON is unchanged. (#83, thanks @babyhuey)

### Fixed

- A `*.sync` setting missing from the config (e.g. after `config unset`) takes its
  documented default instead of on. (#79, thanks @babyhuey)
