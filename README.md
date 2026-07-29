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

data "certificatechecker_domain" "site" {
  hostname = "example.com"
}

output "days_until_expiration" {
  value = data.certificatechecker_domain.site.days_until_expiration
}
```

## Requirements

- An API token, created in the dashboard under **Settings → API tokens**.
- Reading domains requires a plan with API access (Expert and up); managing them requires API write access (Business and up).

Domains are immutable in Certificate Checker: changing the `hostname` replaces the domain and starts a fresh check history — exactly Terraform's replace semantics.

## Development

```sh
go build ./...
go test ./...
```

Acceptance tests run against a real Certificate Checker installation — use a local development environment with a test account on a plan with API write access, never production:

```sh
CERTIFICATECHECKER_ENDPOINT=http://certificate-checker.localhost \
CERTIFICATECHECKER_API_TOKEN=<token> TF_ACC=1 go test ./... -v
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
