---
page_title: "certificatechecker_certificate Data Source"
description: |-
  Read the certificate a monitored domain currently serves.
---

# certificatechecker_certificate (Data Source)

Read the certificate a monitored domain currently serves: the one behind its `days_until_expiration`, as seen by the most recent successful check. When a hostname serves a different certificate per server, this is the one of those that expires first. Works on every plan with API access (read-only is enough).

Until a domain has had one successful check, every attribute is null and the read carries a warning rather than an error. That is normal right after the domain was added: the first check is queued when a domain is registered, so a configuration that creates the domain and reads its certificate in the same apply sees null values the first time around and the certificate on the next run. If the attributes stay null, the checks are failing — the dashboard shows why.

The fingerprints and the serial number are lowercase hex without separators, which makes them easy to compare with what `openssl` or another tool reports.

## Example Usage

```terraform
resource "certificatechecker_domain" "site" {
  hostname = "example.com"
}

data "certificatechecker_certificate" "site" {
  domain_id = certificatechecker_domain.site.id
}

output "certificate_expires" {
  value = data.certificatechecker_certificate.site.valid_to
}

output "certificate_fingerprint" {
  value = data.certificatechecker_certificate.site.fingerprint_sha256
}
```

To read the certificate of a domain that is not managed by Terraform, look it up by hostname first:

```terraform
data "certificatechecker_domain" "site" {
  hostname = "example.com"
}

data "certificatechecker_certificate" "site" {
  domain_id = data.certificatechecker_domain.site.id
}
```

## Schema

### Required

- `domain_id` (String) Identifier of the domain, from the `certificatechecker_domain` resource or data source.

### Read-Only

- `subject` (String) Common name the certificate was issued for.
- `issuer` (String) Name of the issuing certificate authority, such as `R11`.
- `issuer_organization` (String) Organization behind the issuer, such as `Let's Encrypt`.
- `serial_number` (String) Serial number, lowercase hex without separators.
- `valid_from` (String) Start of the validity period (ISO 8601, UTC).
- `valid_to` (String) End of the validity period (ISO 8601, UTC): the expiry date the day counts are based on.
- `names` (List of String) Every hostname the certificate covers: the subject alternative names plus the common name, without duplicates.
- `fingerprint_sha256` (String) SHA-256 fingerprint, lowercase hex without separators.
- `fingerprint_sha1` (String) SHA-1 fingerprint, lowercase hex without separators.
- `signature_algorithm` (String) Signature algorithm, such as `RSA-SHA256` or `ecdsa-with-SHA384`.
- `key_type` (String) Type of the public key: `RSA`, `EC`, `DSA` or `DH`. Null when the check did not record it.
- `key_bits` (Number) Size of the public key in bits. Null when the check did not record it.
- `remote_address` (String) IP address and port that presented this certificate, as `ip:port`.
