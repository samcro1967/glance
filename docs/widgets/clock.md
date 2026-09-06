# Clock

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display a clock showing the current time and date. Optionally, also display the time in other timezones.

## Quick start

```yaml
- type: clock
  hour-format: 24h
  timezones:
    - timezone: Europe/Paris
      label: Paris
    - timezone: America/New_York
      label: New York
    - timezone: Asia/Tokyo
      label: Tokyo
```

## Preview

![Clock widget with local time and additional timezones](../images/clock-widget-preview.png)

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `hour-format` | string | no | `24h` |
| `timezones` | array | no | — |

### `hour-format`
Whether to show the time in 12 or 24 hour format. Possible values are `12h` and `24h`.

## Timezone configuration

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `timezone` | string | yes | — |
| `label` | string | no | — |

### `timezone`
A timezone identifier such as `Europe/London`, `America/New_York`, etc. The full list of available identifiers can be found [here](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones).

### `label`
Optionally, override the display value for the timezone to something more meaningful such as "Home", "Work" or anything else.


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#clock)
