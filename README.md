# Terraform Provider for Certificate Checker

Manage the domains that [Certificate Checker](https://certificate-checker.com) monitors for SSL/TLS certificate expiry — from Terraform or OpenTofu (the same provider binary serves both).

```hcl
terraform {
  required_providers {
    certificatechecker = {
      source = "ekstra-software/certificatechecker"
    }
  }
}

provider "certificatechecker" {
  # Or set CERTIFICATECHECKER_API_TOKEN in the environment.
  api_token = var.certificatechecker_token
}

resource "certificatechecker_domain" "site" {
  hostname = "example.com"
}

resource "certificatechecker_domain" "mail" {
  hostname = "mail.example.com"

  # This host serves no website, so watch its certificate but do not ping it.
  uptime_checks = false
}

data "certificatechecker_domain" "site" {
  hostname = "example.com"
}

output "days_until_expiration" {
  value = data.certificatechecker_domain.site.days_until_expiration
}

# The certificate itself, as seen by the latest successful check.
data "certificatechecker_certificate" "site" {
  domain_id = certificatechecker_domain.site.id
}

output "certificate_fingerprint" {
  value = data.certificatechecker_certificate.site.fingerprint_sha256
}

# The uptime summary: current state and the percentage over the last 30 days.
data "certificatechecker_uptime" "site" {
  domain_id = certificatechecker_domain.site.id
}

output "site_uptime_30d" {
  value = data.certificatechecker_uptime.site.uptime_30d
}
```

## Requirements

- An API token, created in the dashboard under **Settings → API tokens**.
- Reading domains requires a plan with API access (Expert and up); managing them requires API write access (Business and up).

The `certificatechecker_certificate` data source reads the certificate a domain currently serves — subject, issuer, validity period, covered names, fingerprints and key — so it can be compared or alerted on elsewhere. Until the domain's first successful check it answers with null values and a warning, not an error.

The `certificatechecker_uptime` data source reads the uptime summary of a domain — current state and since when, the uptime percentage over the last 30 and 90 days, the response times of the current period — the way the dashboard shows it. It always answers; before the first ping the state is `unknown` with null measurements and a warning. The values change with every ping, so use them in outputs and checks rather than as resource arguments.

Domains are immutable in Certificate Checker apart from their uptime setting: changing the `hostname` replaces the domain and starts a fresh check history — exactly Terraform's replace semantics — while `uptime_checks` is changed in place.

## Development

```sh
go build ./...
go test ./...
```

The unit tests run against a built-in fake of the API, so `go test ./...` works for everyone — Certificate Checker itself is closed source, and no local installation is needed or available.

Acceptance tests (`TestAcc*`) are for maintainers only: they run against a real Certificate Checker development installation with a test account on a plan with API write access — never production. When testing with OpenTofu, point the framework at the tofu binary and its registry host:

```sh
TF_ACC=1 \
TF_ACC_TERRAFORM_PATH=$(which tofu) \
TF_ACC_PROVIDER_HOST=registry.opentofu.org \
TF_ACC_PROVIDER_NAMESPACE=ekstra-software \
CERTIFICATECHECKER_ENDPOINT=http://certificate-checker.localhost \
CERTIFICATECHECKER_API_TOKEN=<token> go test ./... -v -run TestAcc
```

To try a local build in OpenTofu/Terraform, point a [dev override](https://opentofu.org/docs/cli/config/config-file/#development-overrides-for-provider-developers) at the build directory in `~/.terraformrc` / `~/.tofurc`:

```hcl
provider_installation {
  dev_overrides {
    "ekstra-software/certificatechecker" = "/path/to/terraform-provider-certificatechecker"
  }
  direct {}
}
```

## Releasing

Push a semver tag (`v0.1.0`): GitHub Actions builds all platform binaries with goreleaser and signs the checksums with the repository's GPG key, as required by the Terraform Registry.

## License

[MPL-2.0](LICENSE)
