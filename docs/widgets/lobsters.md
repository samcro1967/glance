# Lobsters

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display posts from [Lobsters](https://lobste.rs) or another compatible Lobsters instance.

## Quick start

```yaml
- type: lobsters
  sort-by: hot
  tags:
    - go
    - security
    - linux
```

![Lobsters widget displaying a list of posts](../images/lobsters-widget-preview.png)

## Configuration

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `instance-url` | string | no | `https://lobste.rs/` |
| `custom-url` | string | no | — |
| `limit` | integer | no | `15` |
| `collapse-after` | integer | no | `5` |
| `sort-by` | string | no | `hot` |
| `tags` | array | no | — |

All widgets also support the [shared widget properties](../widgets.md#shared-properties).

### `instance-url`

Sets the base URL for a Lobsters-compatible instance. When omitted, Glance uses `https://lobste.rs/`.

```yaml
instance-url: https://www.journalduhacker.net/
```

### `custom-url`

Fetches posts directly from a custom feed URL. When configured, `instance-url`, `sort-by`, and `tags` are not used to construct the request URL.

### `limit`

Sets the maximum number of posts displayed.

### `collapse-after`

Sets how many posts are visible before the **SHOW MORE** control appears. Set it to `-1` to disable collapsing.

### `sort-by`

Selects the post ordering. Supported values are `hot` and `new`.

### `tags`

Filters the feed to posts matching the configured tags. Tag-filtered requests use the instance tag feed rather than a separately selected sort endpoint.

---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#lobsters)
