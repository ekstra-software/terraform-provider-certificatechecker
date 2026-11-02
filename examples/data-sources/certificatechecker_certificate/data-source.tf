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
