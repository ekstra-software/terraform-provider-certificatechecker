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

## Account-level errors

Two situations close the whole API, regardless of plan, and answer `403` on every request with a stable `error` code next to the human-readable message:

- `payment_required` — an invoice has been overdue long enough to suspend the account. Paying it (dashboard → **Settings → Invoices**) restores access right away.
- `legal_acceptance_required` — the terms have changed and the account owner has to accept them in the dashboard before the API answers again.

The provider recognizes both codes and explains them in its error output; neither is a problem with your configuration or state.

## Schema

### Optional

- `api_token` (String, Sensitive) Personal access token, created in the dashboard under Settings → API tokens. May also be provided via the `CERTIFICATECHECKER_API_TOKEN` environment variable.
- `endpoint` (String) Base URL of the Certificate Checker installation. Defaults to `https://certificate-checker.com`, or the `CERTIFICATECHECKER_ENDPOINT` environment variable when set.
