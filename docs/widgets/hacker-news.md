# Hacker News

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display stories from [Hacker News](https://news.ycombinator.com/) with configurable sorting and comment links.

## Quick start

```yaml
- type: hacker-news
  limit: 15
  collapse-after: 5
```

![Hacker News widget displaying a list of stories](../images/hacker-news-widget-preview.png)

## Configuration

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `limit` | integer | no | `15` |
| `collapse-after` | integer | no | `5` |
| `comments-url-template` | string | no | Hacker News comments URL |
| `sort-by` | string | no | `top` |
| `extra-sort-by` | string | no | — |

All widgets also support the [shared widget properties](../widgets.md#shared-properties).

### `limit`

Sets the maximum number of stories displayed.

### `collapse-after`

Sets how many stories are visible before the **SHOW MORE** control appears. Set it to `-1` to disable collapsing.

### `comments-url-template`

Overrides the destination for story comments, which is useful with an alternative Hacker News front end.

```yaml
comments-url-template: https://www.hckrnws.com/stories/{POST-ID}
```

The `{POST-ID}` placeholder is replaced with the Hacker News story ID.

### `sort-by`

Selects the Hacker News story feed. Supported values are `top`, `new`, and `best`.

### `extra-sort-by`

Applies an additional local sort after stories are fetched. The supported value is `engagement`, which favors stories with more points and comments while also prioritizing newer stories.

---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#hacker-news)
