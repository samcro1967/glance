# Downloads

[Glance README](../../../../../../README.md) · [Configuration](../../../../../configuration.md) · [Widgets](../../../../../widgets.md) · [Examples](../../../../../examples.md) · [Custom API Monitoring](../../README.md)

This integration retrieves monitoring information from SABnzbd and converts it to the shared [Custom API Monitoring](../../README.md) JSON contract.

The example keeps data collection separate from Glance. `fetch.py` produces a JSON file, and Glance consumes that file through a URL served by your environment.

![Downloads monitoring example](../../../../../images/examples/custom-api/monitoring/sabnzbd.png)

## Files

- [fetch.py](fetch.py) retrieves and normalizes the monitoring data.
- [example.json](example.json) provides deterministic representative output and is also used by Glance's visual test fixture.
- [template.yml](../../template.yml) provides the shared Custom API monitoring presentation.

## Requirements

- Python 3
- The `requests` Python package
- A SABnzbd API accessible from the system running the producer

## Configuration

The producer is configured with environment variables:

- `SABNZBD_URL` — SABnzbd base URL. Defaults to `http://localhost:8080`.
- `SABNZBD_API_KEY` — SABnzbd API key.
- `OUTPUT_FILE` — generated JSON path. Defaults to `monitoring.json`.

## Usage

Run the producer after supplying the configuration required by your environment:

```bash
python3 fetch.py
```

By default the example writes `monitoring.json` in the current directory. Set `OUTPUT_FILE` when another destination is required.

Serve the generated JSON over HTTP from a location reachable by Glance. Keep credentials outside the script and outside files committed to source control.

## Reported data

The producer reports SABnzbd status, queue depth, current speed, downloaded data for today and the current week, available disk space, and version.

SABnzbd warnings or a paused state produce an overall `warning` state. Downloading, fetching, idle, checking, repairing, extracting, and moving are healthy operational states and produce `ok`. An unknown status produces a warning.

SABnzbd warnings are included in the details section. When the queue contains downloads, the expandable section shows active queue items and available status, remaining-size, and category information. When the queue is empty, the expandable section instead shows recent download history.

## Glance configuration

```yaml
- type: custom-api
  title: Downloads
  url: https://example.com/sabnzbd_metrics.json
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

[Glance README](../../../../../../README.md) · [Configuration](../../../../../configuration.md) · [Widgets](../../../../../widgets.md) · [Examples](../../../../../examples.md) · [Custom API Monitoring](../../README.md) · [Back to top](#downloads)
