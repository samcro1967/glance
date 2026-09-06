# Custom API

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)
Fetch data from an HTTP API and render it with a Go template. Custom API supports configurable requests, query parameters, authentication, subrequests, reusable template options, and stale-data fallback.

> [!NOTE]
>
> Custom API is intended for users comfortable with HTTP APIs, HTML, and Go templates. See the [Custom API guide](../custom-api.md) for template functions and more advanced examples.

## Quick start

```yaml
- type: custom-api
  title: Random Fact
  cache: 6h
  url: https://uselessfacts.jsph.pl/api/v2/facts/random
  template: |
    <p class="size-h4 color-paragraph">{{ .JSON.String "text" }}</p>
```

![Custom API widget rendering data from an HTTP API](../images/custom-api-preview-1.png)

## Configuration
| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `url` | string | no | — |
| `headers` | map | no | — |
| `method` | string | no | `GET` |
| `body-type` | string | no | `json` |
| `body` | any | no | — |
| `basic-auth` | object | no | — |
| `frameless` | boolean | no | `false` |
| `allow-insecure` | boolean | no | `false` |
| `skip-json-validation` | boolean | no | `false` |
| `template` | string | yes* | — |
| `options` | map | no | — |
| `parameters` | map | no | — |
| `subrequests` | map | no | — |
| `timeout` | duration | no | `5s` |

All widgets also support the [shared widget properties](../widgets.md#shared-properties).

* `template` is required for a normal Custom API widget. A Custom API embedded directly in a [Status Bar](status-bar.md) uses the Status Bar compact response contract instead and does not accept a template.

## Status Bar usage

When `custom-api` is a direct child of a [Status Bar](status-bar.md), it uses the Status Bar compact JSON contract instead of the normal template renderer. In this mode, `template`, `subrequests`, `options`, and `skip-json-validation` are not supported. See the Status Bar documentation for the exact response envelope and item fields.

## Stale fallback

After a `custom-api` widget has completed at least one successful refresh, Glance preserves that last successfully rendered content if a later refresh fails. The previous content remains visible and is marked with a `STALE` indicator showing how long ago the last successful refresh occurred.

The `cache` setting continues to control how often fresh data is requested. A failed refresh does not replace the last successful content or reset its age. Glance retries the update according to its normal retry schedule, and the stale indicator is automatically cleared when a later refresh succeeds.

If the initial request fails before any content has been successfully rendered, there is no previous content to fall back to and the normal widget error behavior is used.

A non-success HTTP response with an empty body is treated as a failed refresh. Non-success responses containing a body retain the existing Custom API behavior and remain available to templates through `.Response`, allowing templates to handle status codes explicitly.

### `url`
The URL to fetch the data from. It must be accessible from the server that Glance is running on.

### `headers`
Optionally specify the headers that will be sent with the request. Example:

```yaml
headers:
  x-api-key: your-api-key
  Accept: application/json
```

### `method`
The HTTP method to use when making the request. Possible values are `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` and `HEAD`.

### `body-type`
The type of the body that will be sent with the request. Possible values are `json`, and `string`.

### `body`
The body that will be sent with the request. It can be a string or a map. Example:

```yaml
body-type: json
body:
  key1: value1
  key2: value2
  multiple-items:
    - item1
    - item2
```

```yaml
body-type: string
body: |
  key1=value1&key2=value2
```

### `basic-auth`
Optionally specify credentials to be sent with the request using HTTP basic authentication. Example:

```yaml
basic-auth:
  username: your-username
  password: your-password
```

### `frameless`
When set to `true`, removes the border and padding around the widget.

### `allow-insecure`
Whether to ignore invalid/self-signed certificates.

### `skip-json-validation`
When set to `true`, skips the JSON validation step. This is useful when the API returns JSON Lines/newline-delimited JSON, which is a format that consists of several JSON objects separated by newlines.

### `template`
The template that will be used to display the data. It relies on Go's `html/template` package so it's recommended to go through [its documentation](https://pkg.go.dev/text/template) to understand how to do basic things such as conditionals, loops, etc. In addition, it also uses [tidwall's gjson](https://github.com/tidwall/gjson) package to parse the JSON data so it's worth going through its documentation if you want to use more advanced JSON selectors. You can view additional examples, explanations, and function definitions in the [Custom API guide](../custom-api.md).

### `options`
A map of options that will be passed to the template and can be used to modify the behavior of the widget.

<details>
<summary>View examples</summary>

<br>

Instead of defining options within the template and having to modify the template itself like such:

```yaml
- type: custom-api
  template: |
    {{ /* User configurable options */ }}
    {{ $collapseAfter := 5 }}
    {{ $showThumbnails := true }}
    {{ $showFlairs := false }}

     <ul class="list list-gap-10 collapsible-container" data-collapse-after="{{ $collapseAfter }}">
      {{ if $showThumbnails }}
        <li>
          <img src="{{ .JSON.String "thumbnail" }}" alt="thumbnail" />
        </li>
      {{ end }}
      {{ if $showFlairs }}
        <li>
          <span class="flair">{{ .JSON.String "flair" }}</span>
        </li>
      {{ end }}
     </ul>
```

You can use the `options` property to retrieve and define default values for these variables:

```yaml
- type: custom-api
  template: |
    <ul class="list list-gap-10 collapsible-container" data-collapse-after="{{ .Options.IntOr "collapse-after" 5 }}">
      {{ if (.Options.BoolOr "show-thumbnails" true) }}
        <li>
          <img src="{{ .JSON.String "thumbnail" }}" alt="thumbnail" />
        </li>
      {{ end }}
      {{ if (.Options.BoolOr "show-flairs" false) }}
        <li>
          <span class="flair">{{ .JSON.String "flair" }}</span>
        </li>
      {{ end }}
    </ul>
```

This way, you can optionally specify the `collapse-after`, `show-thumbnails` and `show-flairs` properties in the widget configuration:

```yaml
- type: custom-api
  options:
    collapse-after: 5
    show-thumbnails: true
    show-flairs: false
```

Which means you can reuse the same template for multiple widgets with different options:

```yaml
# Note that `custom-widgets` isn't a special property, it's just used to define the reusable "anchor", see https://support.atlassian.com/bitbucket-cloud/docs/yaml-anchors/
custom-widgets:
  - &example-widget
    type: custom-api
    template: |
      {{ .Options.StringOr "custom-option" "not defined" }}

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - <<: *example-widget
            options:
              custom-option: "Value 1"

          - <<: *example-widget
            options:
              custom-option: "Value 2"
```

The available methods on `.Options` are `StringOr`, `IntOr`, `FloatOr`, `BoolOr`, and `JSON`.

</details>

### `parameters`
A list of keys and values that will be sent to the custom-api as query parameters.

### `subrequests`
A map of additional requests that will be executed concurrently and then made available in the template via the `.Subrequest` property. Example:

```yaml
- type: custom-api
  cache: 2h
  subrequests:
    another-one:
      url: https://uselessfacts.jsph.pl/api/v2/facts/random
  title: Random Fact
  url: https://uselessfacts.jsph.pl/api/v2/facts/random
  template: |
    <p class="size-h4 color-paragraph">{{ .JSON.String "text" }}</p>
    <p class="size-h4 color-paragraph margin-top-15">{{ (.Subrequest "another-one").JSON.String "text" }}</p>
```

The subrequests support all the same properties as the main request, except for `subrequests` itself, so you can use `headers`, `parameters`, etc.

`(.Subrequest "key")` can be a little cumbersome to write, so you can define a variable to make it easier:

```yaml
  template: |
    {{ $anotherOne := .Subrequest "another-one" }}
    <p>{{ $anotherOne.JSON.String "text" }}</p>
```

You can also access the `.Response` property of a subrequest as you would with the main request:

```yaml
  template: |
    {{ $anotherOne := .Subrequest "another-one" }}
    <p>{{ $anotherOne.Response.StatusCode }}</p>
```

> [!NOTE]
>
> Setting this property will override any query parameters that are already in the URL.

```yaml
parameters:
  param1: value1
  param2:
    - item1
    - item2
```

### `timeout`
The maximum amount of time to wait for the API response. The value must be an integer followed by one of `s` (seconds), `m` (minutes), `h` (hours), or `d` (days), for example `10s`, `1m`, or `2h`.

The timeout can be configured independently for the primary request and each subrequest.

Defaults to `5s`.


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#custom-api)
