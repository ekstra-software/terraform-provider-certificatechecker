resource "certificatechecker_domain" "site" {
  hostname = "example.com"
}

data "certificatechecker_uptime" "site" {
  domain_id = certificatechecker_domain.site.id
}

output "site_status" {
  value = data.certificatechecker_uptime.site.status
}

output "site_uptime_30d" {
  value = data.certificatechecker_uptime.site.uptime_30d
}
