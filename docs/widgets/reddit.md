# Reddit

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display posts from a subreddit with optional thumbnails, flairs, search, authentication, and custom request routing.

When thumbnails are enabled, thumbnail images link to the post discussion and follow the widget's `open-links-in-new-tab` setting.

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
| `use-playwright` | boolean | no | `false` |

All widgets also support the [shared widget properties](../widgets.md#shared-properties).

### `subreddit`

The subreddit from which posts are fetched.

### `style`

Supported values are `vertical-list`, `horizontal-cards`, and `vertical-cards`.

#### Vertical list

![Reddit widget using the vertical-list style](../images/widgets/reddit-vertical-list.png)

**Mobile preview:**

![Reddit mobile preview](../images/widgets/mobile/reddit.png)

#### Horizontal cards

![Reddit horizontal-cards layout](../images/widgets/reddit.png)


#### Vertical cards

![Reddit vertical-cards layout](../images/widgets/reddit-vertical-cards.png)


### `show-thumbnails`

![Reddit vertical-list layout with thumbnails enabled](../images/widgets/reddit-vertical-list-thumbnails.png)

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
>
> Anonymous Reddit requests normally use a guest bearer token obtained from Reddit. For security, Glance does not acquire or forward that token when `request-url-template` is configured. The configured endpoint must handle Reddit authentication itself if required.

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

**Due to changes in Reddit API access, `search` requires `app-auth` to be configured.** Without application authentication, the widget reports a configuration error.

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

## Browser-backed Reddit requests

Set `use-playwright: true` on individual Reddit widgets to fetch Reddit JSON
through a shared authenticated Chromium profile. Configure the CDP relay and
Reddit account once in the root configuration (see [Playwright](../configuration.md#playwright)). For Docker Compose setup, persistent profiles, and troubleshooting, see the [Playwright browser guide](../playwright.md).

```yaml
- type: reddit
  subreddit: selfhosted
  sort-by: new
  use-playwright: true
```

Browser mode is opt-in. Without it, Reddit uses the existing HTTP, proxy, or
application-authentication path. Browser mode cannot be combined with
`app-auth`, `proxy`, or `request-url-template`. Browser-backed search uses the
Reddit browser session instead of requiring `app-auth`.

The browser must have a persistent profile and expose its CDP relay only on a
trusted private network. On an expired session Glance attempts one coordinated
login using the configured credentials. Login challenges requiring human action
are reported as refresh errors; Glance does not bypass CAPTCHA or MFA.

The endpoint must be reachable from the **Glance container**. For example,
`http://playwright:9223` requires both containers on the same Docker network.
Avoid `127.0.0.1:9222` unless Chromium runs inside the Glance container.

## HTTP request timeout

Configure `timeout` on the widget, or inherit it from global/type defaults. The built-in default is 5s; explicit `0s` selects the central 5s baseline. The limit applies per HTTP request rather than to the complete refresh. The setting covers HTTP and CDP metadata only; authenticated Playwright login, navigation, session reuse, and listing waits keep their existing independent limits. Positive `proxy.timeout` overrides the widget budget for proxy requests. See [HTTP request timeouts](../request-timeouts.md) for supported scopes, precedence, cache behavior, and exceptions.

---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#reddit)
