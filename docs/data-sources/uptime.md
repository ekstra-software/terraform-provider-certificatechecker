---
page_title: "certificatechecker_uptime Data Source"
description: |-
  Read the uptime summary of a monitored domain.
---

# certificatechecker_uptime (Data Source)

Read the uptime summary of a monitored domain, as the Uptime section of its dashboard page shows it: the current state and since when it holds, when it was last pinged, the uptime percentage over the last 30 and 90 days, and the response times of the current period. Works on every plan with API access (read-only is enough).

The read always succeeds. Before the first ping the state is `unknown` and the measurements are null, and the read carries a warning saying why: the domain has not been pinged yet, its uptime checks are switched off, or the pings are paused because the plan allows fewer domains than the account has. None of those is an error in the configuration.

Every value here changes as pings come in. That is what a data source is for — it is read fresh on every plan — but it also means these attributes belong in an output or a check, not in the arguments of a resource, where each run would show a change.

The percentages count only observed time: moments without a ping, such as a monitoring gap, count for neither side. A domain that was up during every ping reads `100`.

## Example Usage

```terraform
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
```

To read the uptime of a domain that is not managed by Terraform, look it up by hostname first:

```terraform
data "certificatechecker_domain" "site" {
  hostname = "example.com"
}

data "certificatechecker_uptime" "site" {
  domain_id = data.certificatechecker_domain.site.id
}
```

A check block turns the percentage into a gate for a pipeline:

```terraform
check "site_uptime" {
  data "certificatechecker_uptime" "site" {
    domain_id = certificatechecker_domain.site.id
  }

  assert {
    condition     = coalesce(data.certificatechecker_uptime.site.uptime_30d, 100) >= 99.9
    error_message = "example.com was up less than 99.9% of the last 30 days."
  }
}
```

## Schema

### Required

- `domain_id` (String) Identifier of the domain, from the `certificatechecker_domain` resource or data source.

### Read-Only

- `enabled` (Boolean) Whether this domain is pinged for uptime at all; the `uptime_checks` setting of the domain.
- `paused` (Boolean) Whether the pings are paused because the plan allows fewer domains than the account has. The state is then the last one seen before the pause.
- `status` (String) The current state: `up`, `down`, or `unknown` while uptime checks are off or none has run yet.
- `since` (String) When the current state began (ISO 8601, UTC). Null until the first ping.
- `last_checked_at` (String) When the domain was last pinged (ISO 8601, UTC). Null until the first ping.
- `uptime_30d` (Number) Share of the observed time over the last 30 days the domain was up, in percent. Moments without a ping count for neither side. Null while nothing was observed in the window.
- `uptime_90d` (Number) The same over the last 90 days.
- `response_time_average_ms` (Number) Average response time in the current period, in milliseconds. Null while no ping in this period got a response.
- `response_time_min_ms` (Number) Fastest response in the current period, in milliseconds. Null while no ping in this period got a response.
- `response_time_max_ms` (Number) Slowest response in the current period, in milliseconds. Null while no ping in this period got a response.
