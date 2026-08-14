resource "certificatechecker_domain" "site" {
  hostname = "example.com"
}

resource "certificatechecker_domain" "mail" {
  hostname = "mail.example.com"

  # This host serves no website, so watch its certificate but do not ping it.
  uptime_checks = false
}
