# Analog Clock

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display an analog clock showing the current time. Optionally, display the date, AM/PM indicator, numerical dial markers, and clocks for additional timezones.

## Quick start

```yaml
- type: analog-clock
  hide-am-pm-indicator: false
  hide-date: false
  dial-markers: NumericalFull
  timezones:
    - timezone: Europe/Paris
      label: Paris
    - timezone: America/New_York
      label: New York
    - timezone: Asia/Tokyo
      label: Tokyo
```

## Preview

![Analog Clock widget with date and numerical dial markers](../images/analog-clock-preview.png)

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `hide-am-pm-indicator` | boolean | no | `false` |
| `hide-date` | boolean | no | `false` |
| `dial-markers` | string | no | `NumericalFull` |
| `timezones` | array | no | — |

### `hide-am-pm-indicator`
Whether to hide the AM/PM indicator from the clock face.

### `hide-date`
Whether to hide the date from the clock face.

### `dial-markers`
Controls the numerical markers displayed around the clock face. Possible values are `NumericalFull`, `NumericalMinimal`, and `None`.

`NumericalFull` displays all twelve hour numbers, `NumericalMinimal` displays only 12, 3, 6, and 9, and `None` hides the numerical markers.

## Timezone configuration

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `timezone` | string | yes | — |
| `label` | string | no | — |

### `timezone`
A timezone identifier such as `Europe/London`, `America/New_York`, etc.

### `label`
Optionally, override the display value for the timezone with a more meaningful label such as "Home" or "Work".


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#analog-clock)
