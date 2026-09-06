# Status Bar

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display compact information from supported widgets in a full-width ticker or wrapping status bar.

A status bar can only be placed directly in a page's `head-widgets` or `bottom-widgets`. It cannot be placed in a column or nested inside another widget such as a `group`, `stack`, `split-column`, or another `status-bar`.

A page can contain one or multiple status bars in `head-widgets`, one or multiple status bars in `bottom-widgets`, or status bars in both sections.

## Preview

![Full-width Status Bar showing compact weather, market, RSS, and Custom API information](../images/status-bar-preview.png)

## Supported widgets

- [Weather](weather.md)
- [Markets](markets.md)
- [RSS](rss.md)
- [Custom API](custom-api.md)

Weather, Markets, and RSS children are configured through the `widgets` property using their normal widget configuration. They retain their existing provider fetching, caching, refresh, recovery, error handling, limits, sorting, and link behavior, but are displayed using a compact status-bar presentation rather than their normal full widget layout.

A `custom-api` child uses a dedicated compact mode. It retains the normal Custom API request, HTTP, caching, refresh, stale-content, recovery, and link behavior, but does not accept `template`, `subrequests`, `options`, or `skip-json-validation` inside a Status Bar. Its response must instead conform to the locked Status Bar Custom API contract described below.

## Quick start

```yaml
pages:
  - name: Home
    head-widgets:
      - type: status-bar
        mode: ticker
        widgets:
          - type: weather
            location: London, United Kingdom
            units: metric

          - type: markets
            markets:
              - symbol: SPY
                name: S&P 500
              - symbol: BTC-USD
                name: Bitcoin

          - type: rss
            limit: 3
            feeds:
              - url: https://example.com/feed.xml
                title: News

          - type: custom-api
            url: https://example.com/status.json
            cache: 1m

    columns:
      - size: full
        widgets:
          - type: hacker-news
```

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `mode` | string | no | `ticker` |
| `speed` | string | no | `normal` |
| `widgets` | array | yes | — |

### `mode`

Controls how the compact items are laid out.

* `ticker` continuously scrolls the items horizontally. Scrolling pauses while the status bar is hovered or focused. When reduced motion is requested by the browser or operating system, the items are displayed statically instead.
* `wrap` displays the items statically and allows them to wrap onto additional lines when necessary.

Possible values are `ticker` and `wrap`.

### `speed`

Controls the horizontal scrolling speed when `mode` is `ticker`. The speed is normalized to the rendered width of the ticker content so Status Bars with different amounts of content move at a consistent visual rate.

Possible values are `slow`, `normal`, and `fast`. The default is `normal`.

### `widgets`


An array of supported Glance widgets to display in compact form.

A Custom API child must return this exact response envelope:

```json
{
  "items": [
    {
      "icon1": "https://example.com/away.png",
      "url": "https://example.com/game",
      "line1": "AWAY 2 · HOME 4",
      "line2": "Top 7th",
      "icon2": "https://example.com/home.png"
    }
  ]
}
```

The root object must contain exactly one field, `items`, and `items` must be an array.

Each item permits exactly five fields:

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `icon1` | string | no | Image displayed before the text |
| `url` | string | no | Link applied to the complete compact item |
| `line1` | string | yes | Primary text; must not be empty or whitespace-only |
| `line2` | string | no | Secondary text |
| `icon2` | string | no | Image displayed after the text |

Unknown root or item fields are rejected. Every field that is present must be a JSON string; `null`, numeric, boolean, object, and array values are not accepted. Item order is preserved. An empty data set is represented by:

```json
{"items":[]}
```

The optional `url` applies to the complete compact item and honors the normal link destination behavior, including `new-tab` where configured. Omitting `url` renders a non-link item. Either icon may be omitted independently.

Inside a Status Bar, Custom API is deliberately restricted to this contract. Do not configure `template`, `subrequests`, `options`, or `skip-json-validation` on the child. This keeps the Status Bar presentation deterministic and prevents arbitrary Custom API templates from becoming a second Status Bar rendering system.

The status bar intentionally provides an alternate presentation of existing widgets rather than a separate data-source system. Weather uses the configured location and units, Markets preserves configured symbols, names, sorting and links, and RSS preserves its configured feeds, limits, ordering and article links.

Where equivalent underlying resources are requested by multiple configured widgets, Glance shares the applicable fetched resource work while keeping each widget's configuration and presentation independent.

Presentation-specific options of a child widget that apply only to its normal full layout do not change the status bar's compact renderer.



---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#status-bar)
