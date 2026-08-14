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
```

## Requirements

- An API token, created in the dashboard under **Settings → API tokens**.
- Reading domains requires a plan with API access (Expert and up); managing them requires API write access (Business and up).

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
