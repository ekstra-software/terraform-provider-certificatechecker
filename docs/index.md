---
page_title: "Certificate Checker Provider"
description: |-
  Manage the domains that Certificate Checker monitors for SSL/TLS certificate expiry.
---

# Certificate Checker Provider

Manage the domains that [Certificate Checker](https://certificate-checker.com) monitors for SSL/TLS certificate expiry. Works with both Terraform and OpenTofu.

Reading domains requires a plan with API access (Expert and up); managing them requires API write access (Business and up). Tokens are created in the dashboard under **Settings → API tokens**.

## Example Usage

```terraform
provider "certificatechecker" {
  # Or set CERTIFICATECHECKER_API_TOKEN in the environment.
  api_token = var.certificatechecker_token
}

resource "certificatechecker_domain" "site" {
  hostname = "example.com"
}
```

## Schema

### Optional

- `api_token` (String, Sensitive) Personal access token, created in the dashboard under Settings → API tokens. May also be provided via the `CERTIFICATECHECKER_API_TOKEN` environment variable.
- `endpoint` (String) Base URL of the Certificate Checker installation. Defaults to `https://certificate-checker.com`, or the `CERTIFICATECHECKER_ENDPOINT` environment variable when set.
