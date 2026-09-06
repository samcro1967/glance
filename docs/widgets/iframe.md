# iframe

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Embed an iframe as a widget.

Example:
## Quick start


```yaml
- type: iframe
  source: <url>
  height: 400
```

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| source | string | yes | |
| height | integer | no | 300 |

### `source`
The source of the iframe.

### `height`
The height of the iframe. The minimum allowed height is 50.


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#iframe)
