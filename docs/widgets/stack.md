# Stack

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Stack multiple widgets vertically and treat them as one compositional unit. Stack is especially useful inside a Group when a single tab should display several widgets at the same time.


Child widgets are configured with the `widgets` property.

## Quick start

```yaml
- type: group
  widgets:
    - type: stack
      title: News
      widgets:
        - type: hacker-news
          collapse-after: 5

        - type: lobsters
          collapse-after: 5

    - type: stack
      title: Social
      widgets:
        - type: reddit
          subreddit: selfhosted

        - type: reddit
          subreddit: homelab
```

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `widgets` | array | yes | — |

### `widgets`

Child widgets rendered vertically in their configured order.

A Stack cannot directly contain another `stack`, a `group`, or a `split-column`. Other widget types can be used as child widgets.

## How it works

In this example, the group has two tabs: `News` and `Social`. Selecting `News` displays both the Hacker News and Lobsters widgets vertically, while selecting `Social` displays both Reddit widgets vertically.

### Preview

![Group tabs containing vertically stacked child widgets](../images/stack-widget-preview.png)


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#stack)
