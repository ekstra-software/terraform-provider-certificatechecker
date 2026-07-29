---
page_title: "certificatechecker_domain Resource"
description: |-
  A domain monitored by Certificate Checker.
---

# certificatechecker_domain (Resource)

A domain monitored by Certificate Checker. Domains are immutable: changing the hostname replaces the domain (and starts a fresh check history). Requires a plan with API write access.

## Example Usage

```terraform
resource "certificatechecker_domain" "site" {
  hostname = "example.com"
}
```

## Schema

### Required

- `hostname` (String) Bare hostname to monitor, e.g. `example.com` — no scheme or path. The certificate served on port 443 is checked. Changing it replaces the domain.

### Read-Only

- `id` (String) Identifier of the domain.
- `days_until_expiration` (Number) Days until the certificate expires, per the account's expiration strategy. Null until the first successful check.
- `level` (String) Expiration level zone: info, warning, error, critical or fatal. Null until the first successful check.
- `last_checked_at` (String) Time of the latest successful certificate check (ISO 8601, UTC). Null until the first successful check.
- `created_at` (String) Time the domain was registered (ISO 8601, UTC).

## Import

Import a domain by its id:

```shell
terraform import certificatechecker_domain.site 42
```
