# Custom API Monitoring

[Glance README](../../../../README.md) · [Configuration](../../../configuration.md) · [Widgets](../../../widgets.md) · [Examples](../../../examples.md) · [Custom API](../../../widgets/custom-api.md)

This example provides a reusable presentation template for monitoring data displayed through the Custom API widget. Different services can produce the same small JSON contract and share one consistent Glance presentation.

The template uses native Glance presentation primitives for status, metrics, key/value details, and expandable content rather than defining service-specific CSS.

## Files

- [template.yml](template.yml) contains the shared Custom API presentation template.
- [example.yml](example.yml) shows the minimal widget configuration needed to consume a compatible JSON endpoint.
- [example.json](example.json) demonstrates the expected JSON contract.
- [integrations](#integrations) contains service-specific producers that normalize monitoring data into the shared contract.

## Widget configuration

A monitoring widget points to a JSON endpoint and includes the shared presentation template:

```yaml
- type: custom-api
  title: Service Monitor
  url: https://example.com/monitoring.json
  cache: 5m
  headers:
    Accept: application/json
  template: |
    $include: template.yml
```

Paths used by `$include` must be adjusted for the location of the template in your Glance configuration.

The Custom API `cache` controls how often Glance refreshes the endpoint. If an external producer creates the JSON file, its own execution schedule is separate and should normally be at least as frequent as the desired data freshness.

## JSON contract

A compatible response has this general shape:

```json
{
  "icon": "●",
  "updated": "2026-09-10 21:00",
  "state": "warning",
  "message": "1 service needs attention",
  "metrics": [
    {
      "value": "12",
      "label": "Services"
    }
  ],
  "details": [
    {
      "label": "Database",
      "value": "Unavailable",
      "state": "error"
    }
  ],
  "expandable_label": "Service Details",
  "expandable_items": [
    {
      "label": "database",
      "value": "Unavailable",
      "state": "error"
    }
  ]
}
```

The template reads `icon`, `updated`, `state`, `message`, `metrics`, and `details`. `metrics` and `details` may be empty arrays.

`expandable_items` is optional. When it exists and contains items, `expandable_label` is displayed as the expandable section label.

## States

The top-level `state` controls the widget status presentation:

- `ok` uses the positive status presentation.
- `warning` uses the warning status presentation.
- `error` uses the negative status presentation.
- Any other value uses the neutral status presentation.

Items in `details` and `expandable_items` may also provide a `state`. `ok` values use positive text, `error` values use negative text, and omitted or other states use the normal text presentation.

Metrics contain only `value` and `label`; their values are intentionally presented consistently regardless of the overall widget state.

## Integrations

Service-specific integrations should remain small. Their job is to retrieve the necessary source data, map it to this JSON contract, and make the resulting JSON available to Glance. Scheduling, alert delivery, extensive retry infrastructure, and other environment-specific operational concerns should remain outside the example unless they are necessary to demonstrate the integration.

Available integrations:

- [Argus](integrations/argus/README.md)
- [Beszel](integrations/beszel/README.md)
- [Coraza](integrations/coraza/README.md)
- [Docker Updates](integrations/docker-updates/README.md)
- [Drone](integrations/drone/README.md)
- [Gotify](integrations/gotify/README.md)
- [Healthchecks](integrations/healthchecks/README.md)
- [Internet Speed](integrations/internet-speed/README.md)
- [Portainer](integrations/portainer/README.md)
- [Radarr](integrations/radarr/README.md)
- [Restic](integrations/restic/README.md)
- [SABnzbd](integrations/sabnzbd/README.md)
- [Sonarr](integrations/sonarr/README.md)
- [Tdarr](integrations/tdarr/README.md)
- [Uptime Kuma](integrations/uptime-kuma/README.md)

---

[Glance README](../../../../README.md) · [Configuration](../../../configuration.md) · [Widgets](../../../widgets.md) · [Examples](../../../examples.md) · [Custom API](../../../widgets/custom-api.md) · [Back to top](#custom-api-monitoring)
