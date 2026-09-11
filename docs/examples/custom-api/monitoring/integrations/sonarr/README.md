# TV

[Glance README](../../../../../../README.md) · [Configuration](../../../../../configuration.md) · [Widgets](../../../../../widgets.md) · [Examples](../../../../../examples.md) · [Custom API Monitoring](../../README.md)

This integration retrieves monitoring information from Sonarr and converts it to the shared [Custom API Monitoring](../../README.md) JSON contract.

The example keeps data collection separate from Glance. `fetch.py` produces a JSON file, and Glance consumes that file through a URL served by your environment.

![TV monitoring example](../../../../../images/examples/custom-api/monitoring/sonarr.png)

## Files

- [fetch.py](fetch.py) retrieves and normalizes the monitoring data.
- [example.json](example.json) provides deterministic representative output and is also used by Glance's visual test fixture.
- [template.yml](../../template.yml) provides the shared Custom API monitoring presentation.

## Requirements

- Python 3
- The `requests` Python package
- A Sonarr API accessible from the system running the producer

## Configuration

The producer is configured with environment variables:

- `SONARR_URL` — Sonarr base URL. Defaults to `http://localhost:8989`.
- `SONARR_API_KEY` — Sonarr API key.
- `OUTPUT_FILE` — generated JSON path. Defaults to `monitoring.json`.

## Usage

Run the producer after supplying the configuration required by your environment:

```bash
python3 fetch.py
```

By default the example writes `monitoring.json` in the current directory. Set `OUTPUT_FILE` when another destination is required.

Serve the generated JSON over HTTP from a location reachable by Glance. Keep credentials outside the script and outside files committed to source control.

## Reported data

The producer reports the total number of Sonarr series, monitored series, series and episodes with missing content, downloaded episodes, queue size, queued data size, and Sonarr version.

A successful Sonarr collection produces an overall `ok` state even when monitored series have missing episodes. Missing content is monitoring information rather than an application-health failure, so the Missing Series and Missing Episodes metrics and individual affected series are highlighted as warnings without degrading the overall widget state.

The expandable Missing Series section lists up to 20 monitored series with missing episodes, ordered by missing episode count.

## Glance configuration

```yaml
- type: custom-api
  title: TV
  url: https://example.com/sonarr_metrics.json
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

[Glance README](../../../../../../README.md) · [Configuration](../../../../../configuration.md) · [Widgets](../../../../../widgets.md) · [Examples](../../../../../examples.md) · [Custom API Monitoring](../../README.md) · [Back to top](#tv)
