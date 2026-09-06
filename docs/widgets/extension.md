# Extension

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display content supplied by an external Glance extension endpoint. The widget supports request parameters, headers, authentication, TLS controls, and extension-provided presentation metadata.

## Quick start


Display a widget provided by an external source (third party). For extension development, see the [Extensions guide](../extensions.md).

```yaml
- type: extension
  url: https://domain.com/widget/display-a-message
  allow-potentially-dangerous-html: true
  parameters:
    message: Hello, world!
```


## Configuration
| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `url` | string | yes | — |
| `fallback-content-type` | string | no | — |
| `allow-potentially-dangerous-html` | boolean | no | `false` |
| `timeout` | duration | no | inherited |
| `allow-insecure` | boolean | no | `false` |
| `headers` | map | no | — |
| `basic-auth` | object | no | — |
| `parameters` | map | no | — |

All widgets also support the [shared widget properties](../widgets.md#shared-properties). The Extension widget uses a default `cache` duration of `30m`.

## Response metadata

Extension endpoints can control widget presentation with response headers including `Widget-Title`, `Widget-Title-URL`, `Widget-Content-Type`, and `Widget-Content-Frameless`. See the [extension development guide](../extensions.md) for the complete producer-side contract.

### `url`
The URL of the extension. **Note that the query gets stripped from this URL and the one defined by `parameters` gets used instead.**

### `fallback-content-type`
Optionally specify the fallback content type of the extension if the URL does not return a valid `Widget-Content-Type` header. Currently the only supported value for this property is `html`.

### `timeout`
The maximum time to wait for the extension request.

### `allow-insecure`
Whether to allow invalid/self-signed certificates when requesting the extension.

### `headers`
Optionally specify the headers that will be sent with the request. Example:

```yaml
headers:
  x-api-key: ${SECRET_KEY}
```

### `basic-auth`
Optional HTTP Basic Authentication credentials for the extension request.

### `allow-potentially-dangerous-html`
Whether to allow the extension to display HTML.

> [!WARNING]
>
> There's a reason this property is scary-sounding. It's intended to be used by developers who are comfortable with developing and using their own extensions. Do not enable it if you have no idea what it means or if you're not **absolutely sure** that the extension URL you're using is safe.

### `parameters`
A list of keys and values that will be sent to the extension as query parameters.


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#extension)
