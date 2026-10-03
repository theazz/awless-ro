# awless-ro v0.2.1

A first `sync` in about half the time, and `list buckets` no longer falls over on a
flaky DNS lookup.

## `sync` from scratch: 31–47s → 15.5s

Almost all of a first sync was IAM. Users, groups, roles and managed policies come from
one `GetAccountAuthorizationDetails` pagination, and its pages were fetched one after
another. IAM caps a page by size — policy documents make them heavy — so an account
with ~500 roles and ~560 policies took 25 sequential pages. Asking for bigger pages
does not help; IAM truncates by size regardless.

Each entity type now has its own pagination, and they run side by side. Measured on
the same account, from an empty home:

| | v0.2.0 | v0.2.1 |
|---|---|---|
| `sync` | 31.4 / 47.0 / 37.7s | 15.6 / 15.5 / 15.7s |
| `list policies` | 18.9 / 18.9s | 11.1 / 12.7s |

Same API call, same permission, same data.
([#12](https://github.com/theazz/awless-ro/issues/12))

## `list buckets` failing with "no such host"

The first `list buckets` could fail like this, and the second succeed:

```
[error]   operation error S3: GetBucketLocation, ... dial tcp:
          lookup <bucket>.s3.<region>.amazonaws.com: no such host
```

To keep only the current region's buckets, the tool asked for the location of every
bucket in the account, all at once — each request to the bucket's own hostname, so
as many simultaneous DNS lookups of different names. A resolver with a cold cache,
typically behind a VPN, answered some of them "no such host"; the AWS SDK does not
retry that, and the first failure ended the listing.

S3 can filter by region itself now, so this is one request instead of one per bucket,
and `GetBucketLocation` is no longer among the operations awless-ro can perform —
61, all reads. ([#10](https://github.com/theazz/awless-ro/issues/10), inherited from
upstream)

## Per-item requests are bounded

The same pattern — one request per item, all at once, the first failure ending
everything — was in every place that makes a call per bucket, task definition, IAM
user, load balancer, queue, hosted zone or ECS cluster. In a large account that is
hundreds or thousands of requests in the same instant, and AWS throttles beyond what
the SDK's retries absorb. At most eight are now in flight at a time, and a failure
stops the rest cleanly instead of leaving them hanging.

## Also

- AWS SDK for Go v2 modules updated (patch and minor releases).
- Installing with Homebrew picks this release up through `brew upgrade awless-ro`.

No change to any command, flag, output format or exit code, hence a patch release.
Full detail in
[CHANGELOG.md](https://github.com/theazz/awless-ro/blob/master/CHANGELOG.md).

## Verified

Against a live account (~500 roles, ~560 policies, 77 buckets): `list` of buckets,
access keys, task definitions, containers, listeners, queues, zones and load
balancers returns exactly what v0.2.0 returned, and the synced IAM graph has the same
resources and properties.
