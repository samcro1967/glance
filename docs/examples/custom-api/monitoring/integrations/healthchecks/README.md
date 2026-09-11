# Healthchecks

[Glance README](../../../../../../README.md) · [Configuration](../../../../../configuration.md) · [Widgets](../../../../../widgets.md) · [Examples](../../../../../examples.md) · [Custom API Monitoring](../../README.md)

This integration retrieves monitoring information from Healthchecks and converts it to the shared [Custom API Monitoring](../../README.md) JSON contract.

The example keeps data collection separate from Glance. `fetch.py` produces a JSON file, and Glance consumes that file through a URL served by your environment.

![Healthchecks monitoring example](../../../../../images/examples/custom-api/monitoring/healthchecks.png)

## Files

- [fetch.py](fetch.py) retrieves and normalizes the monitoring data.
- [example.json](example.json) provides deterministic representative output and is also used by Glance's visual test fixture.
- [template.yml](../../template.yml) provides the shared Custom API monitoring presentation.

## Requirements

- Python 3
- The `requests` Python package
- A Healthchecks API accessible from the system running the producer

## Configuration

The producer is configured with environment variables:

- `HEALTHCHECKS_URL` — Healthchecks base URL. Defaults to `http://localhost:8000`.
- `HEALTHCHECKS_API_KEY` — Healthchecks API key.
- `OUTPUT_FILE` — generated JSON path. Defaults to `monitoring.json`.

## Usage

Run the producer after supplying the configuration required by your environment:

```bash
python3 fetch.py
```

By default the example writes `monitoring.json` in the current directory. Set `OUTPUT_FILE` when another destination is required.

Serve the generated JSON over HTTP from a location reachable by Glance. Keep credentials outside the script and outside files committed to source control.

## Reported data

The producer reports the total number of Healthchecks checks and the number that are up, due, or down.

Checks in a down state produce an overall `error` state. Checks that are due or in the grace period produce `warning` when no checks are down. When all checks are up, the widget is `ok`.

Checks requiring attention are included in the expandable section so due and failed checks can be identified directly from the widget.

## Glance configuration

```yaml
- type: custom-api
  title: Healthchecks
  url: https://example.com/healthchecks_metrics.json
  cache: 5m
  headers:
    Accept: application/json
  template: |
    $include: path/to/monitoring/template.yml
```

Adjust the URL and template path for your environment.

## Scheduling

`fetch.py` is an external producer and is not executed by Glance. Run it periodically using cron, a systemd timer, a container scheduler, or another scheduler appropriate for your environment.

The producer schedule and the Custom API `cache` interval are independent. The example intentionally leaves alert delivery and environment-specific operational automation outside the producer.

---

[Glance README](../../../../../../README.md) · [Configuration](../../../../../configuration.md) · [Widgets](../../../../../widgets.md) · [Examples](../../../../../examples.md) · [Custom API Monitoring](../../README.md) · [Back to top](#healthchecks)
