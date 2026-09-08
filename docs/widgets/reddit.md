# Reddit

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display posts from a subreddit with optional thumbnails, flairs, search, authentication, and custom request routing.

## Quick start

```yaml
- type: reddit
  subreddit: technology
```

> [!NOTE]
>
> Reddit may restrict unauthenticated requests from some hosting networks. The widget supports Reddit application authentication, an HTTP/HTTPS proxy, or a custom request URL template when direct access is unsuitable.

## Configuration

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `subreddit` | string | yes | — |
| `style` | string | no | `vertical-list` |
| `show-thumbnails` | boolean | no | `false` |
| `show-flairs` | boolean | no | `false` |
| `limit` | integer | no | `15` |
| `collapse-after` | integer | no | `5` |
| `comments-url-template` | string | no | Reddit comments URL |
| `request-url-template` | string | no | — |
| `proxy` | string or object | no | — |
| `sort-by` | string | no | `hot` |
| `top-period` | string | no | `day` |
| `search` | string | no | — |
| `extra-sort-by` | string | no | — |
| `app-auth` | object | no | — |

All widgets also support the [shared widget properties](../widgets.md#shared-properties).

### `subreddit`

The subreddit from which posts are fetched.

### `style`

Supported values are `vertical-list`, `horizontal-cards`, and `vertical-cards`.

#### Vertical list

![Reddit widget using the vertical-list style](../images/widgets/reddit.png)

#### Horizontal cards


#### Vertical cards


### `show-thumbnails`

Shows thumbnails when using `vertical-list` and Reddit provides a usable thumbnail URL.


> [!NOTE]
>
> Some posts or subreddits do not provide usable thumbnail URLs, so enabling this option does not guarantee that every post has an image.

### `show-flairs`

Displays Reddit post flair when one is available.

### `limit`

Sets the maximum number of posts displayed.

### `collapse-after`

Sets how many posts are visible before **SHOW MORE** when using the list layout. Set it to `-1` to disable collapsing. Card layouts do not use this setting.

### `comments-url-template`

Overrides links to Reddit discussions, which is useful with an alternative Reddit front end.

```yaml
comments-url-template: https://old.reddit.com/{POST-PATH}
```

Supported placeholders are:

- `{POST-PATH}` — full path to the Reddit post
- `{POST-ID}` — Reddit post ID
- `{SUBREDDIT}` — subreddit name

### `request-url-template`

Routes the generated Reddit request through another HTTP endpoint. The template must contain `{REQUEST-URL}`, which is replaced with the complete Reddit request URL.

```yaml
request-url-template: https://your.proxy/?url={REQUEST-URL}
```

> [!NOTE]
>
> When `request-url-template` is configured, it takes precedence over the `proxy` setting for Reddit content requests.

### `proxy`

Routes Reddit requests through an HTTP or HTTPS proxy.

```yaml
proxy: http://user:pass@proxy.example.com:8080
```

For additional proxy options, use object form:

```yaml
proxy:
  url: http://proxy.example.com:8080
  allow-insecure: true
  timeout: 10s
```

#### `allow-insecure`

Allows invalid or self-signed TLS certificates for the configured proxy.

#### `timeout`

Sets the maximum time to wait for the proxy. Use a duration such as `10s` or `1m`.

### `sort-by`

Selects Reddit ordering. Supported values are `hot`, `new`, `top`, and `rising`.

### `top-period`

Controls the time period when `sort-by` is `top`. Supported values are `hour`, `day`, `week`, `month`, `year`, and `all`.

### `search`

Searches within the configured subreddit instead of loading its normal listing.

![Reddit search field syntax examples](../images/instructions/reddit-field-search.png)

### `extra-sort-by`

Applies an additional local sort after posts are fetched. The supported value is `engagement`, which favors posts with more points and comments while also prioritizing newer posts.

## Application authentication

Use `app-auth` to authenticate through a Reddit application:

```yaml
- type: reddit
  subreddit: technology
  app-auth:
    name: ${REDDIT_APP_NAME}
    id: ${REDDIT_APP_CLIENT_ID}
    secret: ${REDDIT_APP_SECRET}
```

If `app-auth` is used, `name`, `id`, and `secret` must all be provided.

---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#reddit)
