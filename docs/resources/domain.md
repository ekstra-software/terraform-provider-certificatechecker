---
page_title: "certificatechecker_domain Resource"
description: |-
  A domain monitored by Certificate Checker.
---

# certificatechecker_domain (Resource)

A domain monitored by Certificate Checker. Domains are immutable apart from their uptime setting: changing the hostname replaces the domain (and starts a fresh check history). Requires a plan with API write access.

## Example Usage

```terraform
resource "certificatechecker_domain" "site" {
  hostname = "example.com"
}

resource "certificatechecker_domain" "mail" {
  hostname = "mail.example.com"

  # This host serves no website, so watch its certificate but do not ping it.
  uptime_checks = false
}
```

## Schema

### Required

- `hostname` (String) Bare hostname to monitor, e.g. `example.com` — no scheme or path. The certificate served on port 443 is checked. Changing it replaces the domain.

### Optional

- `uptime_checks` (Boolean) Whether this domain is pinged for uptime, on top of its certificate check. Defaults to `true`; set it to `false` for a hostname that serves no website, such as a mail host. Leaving it out adopts whatever the account has set, so existing configurations keep working unchanged. Changing it is an in-place update.

### Read-Only

- `id` (String) Identifier of the domain.
- `days_until_expiration` (Number) Days until the certificate expires, per the account's expiration strategy. Null until the first successful check.
- `level` (String) Expiration level zone: info, warning, error, critical or fatal. Null until the first successful check.
- `last_checked_at` (String) Time of the latest successful certificate check (ISO 8601, UTC). Null until the first successful check.
- `created_at` (String) Time the domain was registered (ISO 8601, UTC).

## Import

Import a domain by its id (shown by `GET /api/v1/domains`):

```shell
terraform import certificatechecker_domain.site 0198aaaa-0000-7000-8000-000000000001
```
