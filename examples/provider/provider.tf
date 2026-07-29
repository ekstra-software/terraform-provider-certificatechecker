provider "certificatechecker" {
  # Personal access token from the dashboard (Settings → API tokens).
  # May also be set via the CERTIFICATECHECKER_API_TOKEN environment variable.
  api_token = var.certificatechecker_token
}
