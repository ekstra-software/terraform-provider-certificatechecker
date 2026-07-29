data "certificatechecker_domain" "site" {
  hostname = "example.com"
}

output "days_until_expiration" {
  value = data.certificatechecker_domain.site.days_until_expiration
}
