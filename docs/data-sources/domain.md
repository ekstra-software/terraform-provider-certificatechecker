---
page_title: "certificatechecker_domain Data Source"
description: |-
  Look up a monitored domain to read its certificate expiry status.
---

# certificatechecker_domain (Data Source)

Look up a monitored domain — by id or by hostname — to read its certificate expiry status. Works on every plan with API access (read-only is enough).

## Example Usage

```terraform
data "certificatechecker_domain" "site" {
  hostname = "example.com"
}

output "days_until_expiration" {
  value = data.certificatechecker_domain.site.days_until_expiration
}
```

## Schema

### Optional

- `id` (String) Identifier of the domain. Provide either `id` or `hostname`.
- `hostname` (String) Hostname of the domain. Provide either `id` or `hostname`.

### Read-Only

- `uptime_checks` (Boolean) Whether this domain is pinged for uptime, on top of its certificate check.
- `days_until_expiration` (Number) Days until the certificate expires, per the account's expiration strategy. Null until the first successful check.
- `level` (String) Expiration level zone: info, warning, error, critical or fatal. Null until the first successful check.
- `last_checked_at` (String) Time of the latest successful certificate check (ISO 8601, UTC).
- `created_at` (String) Time the domain was registered (ISO 8601, UTC).
