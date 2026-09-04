# Configuring Glance

- [Preconfigured page](#preconfigured-page)
- [The config file](#the-config-file)
  - [Auto reload](#auto-reload)
  - [Environment variables](#environment-variables)
    - [Other ways of providing tokens/passwords/secrets](#other-ways-of-providing-tokenspasswordssecrets)
  - [Including other config files](#including-other-config-files)
  - [Widget defaults](#widget-defaults)
  - [Icons](#icons)
  - [Config schema](#config-schema)
- [Authentication](#authentication)
- [Server](#server)
- [Document](#document)
- [Branding](#branding)
- [Theme](#theme)
  - [Available themes](#available-themes)
- [Pages & Columns](#pages--columns)
  - [Named dashboards](#named-dashboards)
- [Widgets](#widgets)
  - [RSS](#rss)
  - [Videos](#videos)
  - [Hacker News](#hacker-news)
  - [Lobsters](#lobsters)
  - [Reddit](#reddit)
  - [Search](#search-widget)
  - [Group](#group)
  - [Stack](#stack)
  - [Status Bar](#status-bar)
  - [Split Column](#split-column)
  - [Custom API](#custom-api)
  - [Extension](#extension)
  - [Weather](#weather)
  - [Todo](#todo)
  - [Timer](#timer)
  - [Unit Converter](#unit-converter)
  - [Calculator](#calculator)
  - [Monitor](#monitor)
  - [Releases](#releases)
  - [Docker Containers](#docker-containers)
  - [DNS Stats](#dns-stats)
  - [Server Stats](#server-stats)
  - [Repository](#repository)
  - [Bookmarks](#bookmarks)
  - [Calendar](#calendar)
  - [ICS Events](#ics-events)
  - [Calendar (legacy)](#calendar-legacy)
  - [ChangeDetection.io](#changedetectionio)
  - [Clock](#clock)
  - [Analog Clock](#analog-clock)
  - [Markets](#markets)
  - [Twitch Channels](#twitch-channels)
  - [Twitch Top Games](#twitch-top-games)
  - [iframe](#iframe)
  - [Markdown](#markdown)
  - [HTML](#html)


## Preconfigured page
If you don't want to spend time reading through all the available configuration options and just want something to get you going quickly you can use [this `glance.yml` file](glance.yml) and make changes to it as you see fit. It will give you a page that looks like the following:

![](images/preconfigured-page-preview.png)

Configure the widgets, add more of them, add extra pages, etc. Make it your own!

## The config file

### Auto reload
Automatic config reload is supported, meaning that you can make changes to the config file and have them take effect on save without having to restart the container/service. Making changes to environment variables does not trigger a reload and requires manual restart. Deleting a config file will stop that file from being watched, even if it is recreated.

> [!NOTE]
>
> If you attempt to start Glance with an invalid config it will exit with an error outright. If you successfully started Glance with a valid config and then made changes to it which result in an error, you'll see that error in the console and Glance will continue to run with the old configuration. You can then continue to make changes and when there are no errors the new configuration will be loaded.

> [!CAUTION]
>
> Reloading the configuration file clears your cached data, meaning that you have to request the data anew each time you do this. This can lead to rate limiting for some APIs if you do it too frequently. Having a cache that persists between reloads will be added in the future.

### Environment variables
Inserting environment variables is supported anywhere in the config. This is done via the `${ENV_VAR}` syntax. Attempting to use an environment variable that doesn't exist will result in an error and Glance will either not start or load your new config on save. Example:

```yaml
server:
  host: ${HOST}
  port: ${PORT}
```

Can also be in the middle of a string:

```yaml
- type: rss
  title: ${RSS_TITLE}
  feeds:
    - url: http://domain.com/rss/${RSS_CATEGORY}.xml
```

Works with any type of value, not just strings:

```yaml
- type: rss
  limit: ${RSS_LIMIT}
```

If you need to use the syntax `${NAME}` in your config without it being interpreted as an environment variable, you can escape it by prefixing with a backslash `\`:

```yaml
something: \${NOT_AN_ENV_VAR}
```

#### Other ways of providing tokens/passwords/secrets

You can use [Docker secrets](https://docs.docker.com/compose/how-tos/use-secrets/) with the following syntax:

```yaml
# This will be replaced with the contents of the file /run/secrets/github_token
# so long as the secret `github_token` is provided to the container
token: ${secret:github_token}
```

Alternatively, you can load the contents of a file who's path is provided by an environment variable:

`docker-compose.yml`
```yaml
services:
  glance:
    image: glanceapp/glance
    environment:
      - TOKEN_FILE=/home/user/token
    volumes:
      - /home/user/token:/home/user/token
```

`glance.yml`
```yaml
token: ${readFileFromEnv:TOKEN_FILE}
```

> [!NOTE]
>
> The contents of the file will be stripped of any leading/trailing whitespace before being used.

### Including other config files
Including config files from within your main config file is supported. This is done via the `$include` directive along with a relative or absolute path to the file you want to include. If the path is relative, it will be relative to the main config file. Additionally, environment variables can be used within included files, and changes to the included files will trigger an automatic reload. Example:

```yaml
pages:
  - $include: home.yml
  - $include: videos.yml
  - $include: homelab.yml
```

The file you are including should not have any additional indentation, its values should be at the top level and the appropriate amount of indentation will be added automatically depending on where the file is included. Example:

`glance.yml`

```yaml
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - $include: rss.yml
  - name: News
    columns:
      - size: full
        widgets:
          - type: group
            widgets:
              - $include: rss.yml
              - type: reddit
                subreddit: news
```

`rss.yml`

```yaml
- type: rss
  title: News
  feeds:
    - url: ${RSS_URL}
```

The `$include` directive can be used anywhere in the config file, not just in the `pages` property, however it must be on its own line and have the appropriate indentation.

If you encounter YAML parsing errors when using the `$include` directive, the reported line numbers will likely be incorrect. This is because the inclusion of files is done before the YAML is parsed, as YAML itself does not support file inclusion. To help with debugging in cases like this, you can use the `config:print` command and pipe it into `less -N` to see the full config file with includes resolved and line numbers added:

```sh
glance --config /path/to/glance.yml config:print | less -N
```

This is a bit more convoluted when running Glance inside a Docker container:

```sh
docker run --rm -v ./glance.yml:/app/config/glance.yml glanceapp/glance config:print | less -N
```

This assumes that the config you want to print is in your current working directory and is named `glance.yml`.

### Widget defaults

The optional top-level `widget-defaults` property lets you define shared widget settings once and inherit them across the dashboard. Existing configurations do not need to use `widget-defaults`; when it is omitted, existing widget syntax and built-in behavior remain unchanged.

Defaults can be defined globally and refined for a particular widget type:

```yaml
widget-defaults:
  global:
    cache: 30m
    new-tab: true

  types:
    rss:
      cache: 15m
      collapse-after: 8
    monitor:
      timeout: 5s
```

#### Resolution and precedence

Defaults are resolved from least specific to most specific:

```text
built-in widget default
        ↓
widget-defaults.global
        ↓
widget-defaults.types.<widget-type>
        ↓
widget instance
        ↓
child/source setting, where supported
```

The most specific explicitly configured value wins. Explicit widget and child/source settings therefore continue to override inherited defaults.

For example:

```yaml
widget-defaults:
  global:
    cache: 30m
  types:
    rss:
      cache: 15m

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/news.xml

          - type: rss
            cache: 5m
            feeds:
              - url: https://example.com/updates.xml
```

The first RSS widget uses the RSS type default of `15m`. The second uses `5m` because the widget instance is more specific.

Container widgets do not implicitly pass their type defaults to their children. Each child widget resolves defaults according to its own widget type. For example, an RSS widget inside a Status Bar, Group, or Stack still resolves RSS defaults independently.

#### Common capabilities

The following capabilities are common to registered widgets and may be configured under `widget-defaults.global` or `widget-defaults.types.<type>`:

| Name | Type | Description |
| ---- | ---- | ----------- |
| `title` | string | Default widget title |
| `title-url` | string | Default URL opened by the widget title |
| `hide-header` | boolean | Whether the widget header is hidden |
| `css-class` | string | Default custom CSS class |
| `cache` | duration | Default refresh/cache interval where the widget supports configurable caching |
| `new-tab` | boolean | Whether links governed by the widget's common link policy open in a new tab |

`new-tab` is the canonical hierarchical setting for link destination. Existing widget-specific properties such as `same-tab` remain supported and are not deprecated. Where a widget or child exposes one of those existing controls, the more specific setting wins.

#### Capability-specific defaults

Some capabilities are meaningful only for particular widget types. They can be configured under `widget-defaults.types.<type>` and, where supported, directly on widget instances or their child/source entries.

| Name | Type | Description |
| ---- | ---- | ----------- |
| `limit` | integer | Maximum number of displayed items |
| `collapse-after` | integer | Number of visible items before the remainder is collapsed |
| `collapse-after-rows` | integer | Number of visible rows before the remainder is collapsed |
| `timeout` | duration | HTTP request timeout |
| `allow-insecure` | boolean | Allow invalid/self-signed TLS certificates |
| `headers` | map | HTTP request headers |
| `basic-auth` | object | HTTP Basic Authentication credentials |
| `proxy` | string or object | HTTP proxy configuration where supported |

Applicability and inheritance scope are validated. A capability is applied only to widget types and scopes for which it has defined semantics; unsupported combinations are rejected rather than silently changing unrelated behavior.

List capabilities such as `limit` and `collapse-after` are not global defaults. They are available only for compatible widget types. `basic-auth` is also intentionally not global and is available only for compatible type, instance, or child/source scopes.

HTTP defaults apply only to widgets whose endpoint semantics support them. Provider-owned services do not automatically inherit generic HTTP configuration merely because they perform network requests.

#### Child and source settings

Some widgets contain child entries or independently configurable remote sources. Supported defaults can flow to those entries while preserving explicit child/source overrides.

For example:

```yaml
widget-defaults:
  global:
    timeout: 10s
    headers:
      User-Agent: Glance

  types:
    rss:
      timeout: 5s
      basic-auth:
        username: reader
        password: ${RSS_PASSWORD}

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: rss
            feeds:
              - url: https://example.com/feed.xml
              - url: https://example.com/private.xml
                timeout: 15s
```

The first feed inherits the RSS type timeout of `5s`. The second explicitly uses `15s`.

Inherited HTTP headers are merged with more-specific headers, with the more-specific value winning when the same header name is configured at multiple levels. Dedicated authentication settings take precedence over an inherited or custom `Authorization` header.

#### Backward compatibility

`widget-defaults` is additive and optional. A valid configuration that does not use it continues to use the existing widget-specific syntax and built-in defaults. Existing names such as `same-tab` remain valid and do not produce deprecation warnings merely because `new-tab` is the canonical hierarchical capability name.

## Icons

For widgets which provide you with the ability to specify icons such as the monitor, bookmarks, docker containers, etc, you can use the `icon` property to specify a URL to an image or use icon names from multiple libraries via prefixes:

```yml
icon: si:immich # si for Simple icons https://simpleicons.org/
icon: sh:immich # sh for selfh.st icons https://selfh.st/icons/
icon: di:immich # di for Dashboard icons https://github.com/homarr-labs/dashboard-icons
icon: mdi:camera # mdi for Material Design icons https://pictogrammers.com/library/mdi/
```

The `sh:` and `di:` prefixes request SVG icons by default. If an icon is only available as a PNG, add the extension to its name:

```yaml
icon: sh:unmanic.png
```

> [!NOTE]
>
> The icons are loaded externally and are hosted on `cdn.jsdelivr.net`, if you do not wish to depend on a 3rd party you are free to download the icons individually and host them locally.

Icons from the Simple icons library as well as Material Design icons will automatically invert their color to match your light or dark theme, however you may want to enable this manually for other icons. To do this, you can use the `auto-invert` prefix:

```yaml
icon: auto-invert https://example.com/path/to/icon.png # with a URL
icon: auto-invert sh:glance-dark # with a selfh.st icon
```

This expects the icon to be black and will automatically invert it to white when using a dark theme.

If there is no `.svg` version available for a `selfh.st` or `Dashboard` icon, then you can add the image extension of the format you wish to use.
```yaml
icon: sh:glance.png # use the .png version of the icon
icon: sh:glance.webp # use the .webp version of the icon
```

## Config schema

For property descriptions, validation and autocompletion of the config within your IDE, @not-first has kindly created a [schema](https://github.com/not-first/glance-schema). Massive thanks to them for this, go check it out and give them a star!

## Authentication

To make sure that only you and the people you want to share your dashboard with have access to it, you can set up authentication via username and password. This is done through a top level `auth` property. Example:

```yaml
auth:
  secret-key: # this must be set to a random value generated using the secret:make CLI command
  users:
    admin:
      password: 123456
    svilen:
      password: 123456
```

To generate a secret key, run the following command:

```sh
./glance secret:make
```

Or with Docker:

```sh
docker run --rm glanceapp/glance secret:make
```

### Using hashed passwords

If you do not want to store plain passwords in your config file or in environment variables, you can hash your password and provide its hash instead:

```sh
./glance password:hash mysecretpassword
```

Or with Docker:

```sh
docker run --rm glanceapp/glance password:hash mysecretpassword
```

Then, in your config file use the `password-hash` property instead of `password`:

```yaml
auth:
  secret-key: # this must be set to a random value generated using the secret:make CLI command
  users:
    admin:
      password-hash: $2a$10$o6SXqiccI3DDP2dN4ADumuOeIHET6Q4bUMYZD6rT2Aqt6XQ3DyO.6
```

### Preventing brute-force attacks

Glance will automatically block IP addresses of users who fail to authenticate 5 times in a row in the span of 5 minutes. In order for this feature to work correctly, Glance must know the real IP address of requests. If you're using a reverse proxy such as nginx, Traefik, NPM, etc, you must set the `proxied` property in the `server` configuration to `true`:

```yaml
server:
  proxied: true
```

When set to `true`, Glance will use the `X-Forwarded-For` header to determine the original IP address of the request, so make sure that your reverse proxy is correctly configured to send that header.

## Server
Server configuration is done through a top level `server` property. Example:

```yaml
server:
  port: 8080
  assets-path: /home/user/glance-assets
```

### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| host | string | no |  |
| port | number | no | 8080 |
| proxied | boolean | no | false |
| base-url | string | no | |
| assets-path | string | no |  |

#### `host`
The address which the server will listen on. Setting it to `localhost` means that only the machine that the server is running on will be able to access the dashboard. By default it will listen on all interfaces.

#### `port`
A number between 1 and 65,535, so long as that port isn't already used by anything else.

#### `proxied`
Set to `true` if you're using a reverse proxy in front of Glance. This will make Glance use the `X-Forwarded-*` headers to determine the original request details.

#### `base-url`
The base URL that Glance is hosted under. No need to specify this unless you're using a reverse proxy and are hosting Glance under a directory. If that's the case then you can set this value to `/glance` or whatever the directory is called. Note that the forward slash (`/`) in the beginning is required unless you specify the full domain and path.

> [!IMPORTANT]
> You need to strip the `base-url` prefix before forwarding the request to the Glance server.
> In Caddy you can do this using [`handle_path`](https://caddyserver.com/docs/caddyfile/directives/handle_path) or [`uri strip_prefix`](https://caddyserver.com/docs/caddyfile/directives/uri).

#### `assets-path`
The path to a directory that will be served by the server under the `/assets/` path. This is handy for widgets like the Monitor where you have to specify an icon URL and you want to self host all the icons rather than pointing to an external source.

> [!IMPORTANT]
>
> When installing through docker the path will point to the files inside the container. Don't forget to mount your assets path to the same path inside the container.
> Example:
>
> If your assets are in:
> ```
> /home/user/glance-assets
> ```
>
> You should mount:
> ```
> /home/user/glance-assets:/app/assets
> ```
>
> And your config should contain:
> ```
> assets-path: /app/assets
> ```

##### Examples

Say you have a directory `glance-assets` with a file `gitea-icon.png` in it and you specify your assets path like:

```yaml
assets-path: /home/user/glance-assets
```

To be able to point to an asset from your assets path, use the `/assets/` path like such:

```yaml
icon: /assets/gitea-icon.png
```

## Document
If you want to insert custom HTML into the `<head>` of the document for all pages, you can do so by using the `document` property. Example:

```yaml
document:
  head: |
    <script src="/assets/custom.js"></script>
```

## Branding
You can adjust the various parts of the branding through a top level `branding` property. Example:

```yaml
branding:
  custom-footer: |
    <p>Powered by <a href="https://github.com/glanceapp/glance">Glance</a></p>
  logo-url: /assets/logo.png
  favicon-url: /assets/logo.png
  app-name: "My Dashboard"
  app-icon-url: "/assets/app-icon.png"
  app-background-color: "#151519"
```

### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| hide-footer | bool | no | false |
| custom-footer | string | no |  |
| logo-text | string | no | G |
| logo-url | string | no | |
| favicon-url | string | no | |
| app-name | string | no | Glance |
| app-icon-url | string | no | Glance's default icon |
| app-background-color | string | no | Glance's default background color |

#### `hide-footer`
Hides the footer when set to `true`.

#### `custom-footer`
Specify custom HTML to use for the footer.

#### `logo-text`
Specify custom text to use instead of the "G" found in the navigation.

#### `logo-url`
Specify a URL to a custom image to use instead of the "G" found in the navigation. If both `logo-text` and `logo-url` are set, only `logo-url` will be used.

#### `favicon-url`
Specify a URL to a custom image to use for the favicon.

#### `app-name`
Specify the name of the web app shown in browser tab and PWA.

#### `app-icon-url`
Specify URL for PWA and browser tab icon (512x512 PNG).

#### `app-background-color`
Specify background color for PWA. Must be a valid CSS color.

## Theme
Theming is done through a top level `theme` property. Values for the colors are in [HSL](https://giggster.com/guide/basics/hue-saturation-lightness/) (hue, saturation, lightness) format. You can use a color picker [like this one](https://hslpicker.com/) to convert colors from other formats to HSL. The values are separated by a space and `%` is not required for any of the numbers.

Example:

```yaml
theme:
  # This will be the default theme
  background-color: 100 20 10
  primary-color: 40 90 40
  contrast-multiplier: 1.1

  disable-picker: false
  presets:
    gruvbox-dark:
      background-color: 0 0 16
      primary-color: 43 59 81
      positive-color: 61 66 44
      negative-color: 6 96 59

    zebra:
      light: true
      background-color: 0 0 95
      primary-color: 0 0 10
      negative-color: 0 90 50
```

### Available themes
If you don't want to spend time configuring your own theme, there are [several available themes](themes.md) which you can simply copy the values for.

### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| light | boolean | no | false |
| background-color | HSL | no | 240 8 9 |
| primary-color | HSL | no | 43 50 70 |
| positive-color | HSL | no | same as `primary-color` |
| negative-color | HSL | no | 0 70 70 |
| contrast-multiplier | number | no | 1 |
| text-saturation-multiplier | number | no | 1 |
| custom-css-file | string | no | |
| disable-picker | bool | false | |
| presets | object | no | |

#### `light`
Whether the scheme is light or dark. This does not change the background color, it inverts the text colors so that they look appropriately on a light background.

#### `background-color`
Color of the page and widgets.

#### `primary-color`
Color used across the page, largely to indicate unvisited links.

#### `positive-color`
Used to indicate that something is positive, such as stock price being up, twitch channel being live or a monitored site being online. If not set, the value of `primary-color` will be used.

#### `negative-color`
Oppposite of `positive-color`.

#### `contrast-multiplier`
Used to increase or decrease the contrast (in other words visibility) of the text. A value of `1.3` means that the text will be 30% lighter/darker depending on the scheme. Use this if you think that some of the text on the page is too dark and hard to read. Example:

![difference between 1 and 1.3 contrast](images/contrast-multiplier-example.png)

#### `text-saturation-multiplier`
Used to increase or decrease the saturation of text, useful when using a custom background color with a high amount of saturation and needing the text to have a more neutral color. `0.5` means that the saturation will be 50% lower and `1.5` means that it'll be 50% higher.

#### `custom-css-file`
Path to a custom CSS file, either external or one from within the server configured assets path. Example:

```yaml
theme:
  custom-css-file: /assets/my-style.css
```

> [!TIP]
>
> Because Glance uses a lot of utility classes it might be difficult to target some elements. To make it easier to style specific widgets, each widget has a `widget-type-{name}` class, so for example if you wanted to make the links inside just the RSS widget bigger you could use the following selector:
>
> ```css
> .widget-type-rss a {
>     font-size: 1.5rem;
> }
> ```
>
> In addition, you can also use the `css-class` property which is available on every widget to set custom class names for individual widgets.

#### `disable-picker`
When set to `true` hides the theme picker and disables the abiltity to switch between themes. All users who previously picked a non-default theme will be switched over to the default theme.

#### `presets`
Define additional theme presets that can be selected from the theme picker on the page. For each preset, you can specify the same properties as for the default theme, such as `background-color`, `primary-color`, `positive-color`, `negative-color`, `contrast-multiplier`, etc., except for the `custom-css-file` property.

Example:

```yaml
theme:
  presets:
    my-custom-dark-theme:
      background-color: 229 19 23
      contrast-multiplier: 1.2
      primary-color: 222 74 74
      positive-color: 96 44 68
      negative-color: 359 68 71
    my-custom-light-theme:
      light: true
      background-color: 220 23 95
      contrast-multiplier: 1.1
      primary-color: 220 91 54
      positive-color: 109 58 40
      negative-color: 347 87 44
```

To override the default dark and light themes, use the key names `default-dark` and `default-light`.

## Pages & Columns
![illustration of pages and columns](images/pages-and-columns-illustration.png)

Using pages and columns is how widgets are organized. Each page contains up to 3 columns and each column can have any number of widgets.

### Pages
Pages are defined through a top level `pages` property.

When `dashboards` is not configured, Glance uses its standard page behavior: the page defined first becomes the home page and all pages are automatically added to the navigation bar in the order that they were defined.

```yaml
pages:
  - name: Home
    columns: ...

  - name: Page 2
    columns: ...

  - name: Page 3
    columns: ...
```

When [named dashboards](#named-dashboards) are configured, pages are still defined only once under `pages`, but each dashboard controls which pages appear in its navigation and in what order.

### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| name | string | yes | |
| slug | string | no | |
| width | string | no | |
| desktop-navigation-width | string | no | |
| center-vertically | boolean | no | false |
| hide-desktop-navigation | boolean | no | false |
| show-mobile-header | boolean | no | false |
| head-widgets | array | no | |
| bottom-widgets | array | no | |
| columns | array | yes | |

#### `name`
The name of the page which gets shown in the navigation bar.

#### `slug`
The URL friendly version of the title which is used to access the page. For example if the title of the page is "RSS Feeds" you can make the page accessible via `localhost:8080/feeds` by setting the slug to `feeds`. If not defined, it will automatically be generated from the title.

#### `width`
The maximum width of the page on desktop. Possible values are `default`, `slim` and `wide`.

#### `desktop-navigation-width`
The maximum width of the desktop navigation. Useful if you have a few pages that use a different width than the rest and don't want the navigation to jump abruptly when going to and away from those pages. Possible values are `default`, `slim` and `wide`.

Here are the pixel equivalents for each value:

* default: `1600px`
* slim: `1100px`
* wide: `1920px`

> [!NOTE]
>
> When using `slim`, the maximum number of columns allowed for that page is `2`.

#### `center-vertically`
When set to `true`, vertically centers the content on the page. Has no effect if the content is taller than the height of the viewport.

#### `hide-desktop-navigation`
Whether to show the navigation links at the top of the page on desktop.

#### `show-mobile-header`
Whether to show a header displaying the name of the page on mobile. The header purposefully has a lot of vertical whitespace in order to push the content down and make it easier to reach on tall devices.

Preview:

![](images/mobile-header-preview.png)

#### `head-widgets`

Head widgets will be shown at the top of the page, above the columns, and take up the combined width of all columns. You can specify any widget, though some will look better than others, such as the markets, RSS feed with `horizontal-cards` style, and videos widgets. Example:

![](images/head-widgets-preview.png)

```yaml
pages:
  - name: Home
    head-widgets:
      - type: markets
        hide-header: true
        markets:
          - symbol: SPY
            name: S&P 500
          - symbol: BTC-USD
            name: Bitcoin
          - symbol: NVDA
            name: NVIDIA
          - symbol: AAPL
            name: Apple
          - symbol: MSFT
            name: Microsoft

    columns:
      - size: small
        widgets:
          - type: calendar
      - size: full
        widgets:
          - type: hacker-news
      - size: small
        widgets:
          - type: weather
            location: London, United Kingdom
```

#### `bottom-widgets`

Bottom widgets will be shown at the bottom of the page, below the columns, and take up the combined width of all columns. As with `head-widgets`, you can specify any widget, though widgets designed for wider layouts will generally work best.

Example:

```yaml
pages:
  - name: Home
    columns:
      - size: small
        widgets:
          - type: calendar
      - size: full
        widgets:
          - type: hacker-news
      - size: small
        widgets:
          - type: weather
            location: London, United Kingdom

    bottom-widgets:
      - type: videos
        channels:
          - UC_x5XG1OV2P6uZZ5FSM9Ttw
```

Preview:

![](images/bottom-widgets-preview.png)

### Named dashboards

Named dashboards allow the same configured pages to be organized into multiple independently addressable navigation sets.

Pages continue to be defined once under the top-level `pages` property. The optional top-level `dashboards` property selects which pages belong to each dashboard and controls their navigation order.

A page can belong to multiple dashboards. The page itself is not duplicated: each dashboard uses the same underlying page, widgets, cached data, and update lifecycle.

Example:

```yaml
pages:
  - name: Home
    slug: home
    columns: ...

  - name: Page 2
    slug: page2
    columns: ...

  - name: Page 3
    slug: page3
    columns: ...

  - name: Shared
    slug: shared
    columns: ...

dashboards:
  Default:
    - home
    - page2
    - page3
    - shared

  Personal:
    - home
    - page2
    - shared

  Family:
    - home
    - page3
    - shared
```

Dashboard entries reference pages by their slug. If a page does not explicitly define a `slug`, its automatically generated slug is used.

Preview:

![](images/named-dashboards-preview.png)

#### Default dashboard

When `dashboards` is configured, a dashboard named `Default` is required.

The `Default` dashboard controls the standard Glance routes. Its first page becomes the home page at `/`, and its pages use their normal top-level paths.

Using the example above:

```text
/          -> Home
/page2     -> Page 2
/page3     -> Page 3
/shared    -> Shared
```

Only pages assigned to `Default` appear in the default navigation.

#### Named dashboard routes

Every dashboard other than `Default` receives its own URL prefix generated from the dashboard name.

The first page assigned to the dashboard becomes its dashboard home page.

For example, the `Personal` dashboard above is available at:

```text
/personal/         -> Home
/personal/page2    -> Page 2
/personal/shared   -> Shared
```

The `Family` dashboard is available at:

```text
/family/         -> Home
/family/page3    -> Page 3
/family/shared   -> Shared
```

#### Dashboard switcher

When named dashboards are configured, the application logo in the desktop navigation acts as a dashboard switcher.

Selecting the logo opens a menu containing all available dashboards in the same order they are defined under `dashboards`. The currently active dashboard is indicated in the menu.

Selecting a dashboard navigates to that dashboard's home page:

```text
Default  -> /
Personal -> /personal/
Family   -> /family/
```

The switcher can be closed by selecting the logo again, clicking outside the menu, or pressing <kbd>Escape</kbd>. When the logo is focused using keyboard navigation, <kbd>Enter</kbd> or <kbd>Space</kbd> opens or closes the switcher.

On mobile, the available dashboards are shown with the other mobile navigation actions.

When `dashboards` is not configured, the application logo retains the standard Glance behavior and no dashboard switcher is shown.

Navigation within a named dashboard remains inside that dashboard. For example, selecting Shared while viewing the `Family` dashboard links to `/family/shared` rather than `/shared`.

A page that exists globally but is not assigned to a particular dashboard cannot be accessed through that dashboard's route.

#### Shared pages

Pages referenced by multiple dashboards are shared rather than copied.

For example, because `shared` belongs to `Default`, `Personal`, and `Family`, all three dashboard routes render the same configured Shared page:

```text
/shared
/personal/shared
/family/shared
```

This means you do not need separate page configurations for each dashboard, and widgets retain the same caching and update behavior regardless of which dashboard is used to view the page.

#### Backward compatibility

The `dashboards` property is optional.

If it is omitted, Glance behaves exactly as it does without this feature:

- the first configured page is the home page;
- all configured pages appear in navigation;
- pages use their normal top-level routes.

Existing configurations therefore continue to work without modification.

#### Validation

When `dashboards` is configured:

- a `Default` dashboard is required;
- each dashboard must contain at least one page;
- every referenced page slug must exist;
- the same page cannot be listed more than once in a dashboard;
- dashboard names must generate unique URL slugs;
- dashboard slugs cannot conflict with reserved Glance routes.

Named dashboard slugs also cannot conflict with page slugs. If a named dashboard generates the same slug as an existing page, that dashboard is ignored and a warning is logged. Glance continues running with the remaining valid dashboards and pages.

For example, if a page uses the slug `personal`, a named dashboard that also generates the slug `personal` will be ignored. The existing `/personal` page route remains available.

The order of pages in each dashboard determines both the navigation order and which page becomes that dashboard's home page.

### Columns
Columns are defined for each page using a `columns` property. There are three column sizes: `full`, `medium` and `small`.

A `small` column has a fixed width of 300px. A `full` column takes up the remaining available width. A `medium` column is used for proportional layouts: three `medium` columns divide the available width evenly, while a `medium` column paired with a `full` column uses approximately one third and two thirds of the available width, respectively.

Pages can have up to 3 columns. Traditional layouts using only `small` and `full` columns must contain either 1 or 2 `full` columns. When using `medium`, the supported layouts are three `medium` columns, or one `medium` column paired with one `full` column in either order.

When the page is displayed using the mobile layout, one column is shown at a time regardless of its configured size.

Example:

```yaml
pages:
  - name: Home
    columns:
      - size: small
        widgets: ...
      - size: full
        widgets: ...
      - size: small
        widgets: ...
```

### Properties
| Name | Type | Required |
| ---- | ---- | -------- |
| size | string | yes |
| widgets | array | no |

The `size` property accepts `small`, `medium` or `full`.

Here are some of the possible traditional column configurations:

![column configuration small-full-small](images/column-configuration-1.png)

```yaml
columns:
  - size: small
    widgets: ...
  - size: full
    widgets: ...
  - size: small
    widgets: ...
```

![column configuration full-small](images/column-configuration-2.png)

```yaml
columns:
  - size: full
    widgets: ...
  - size: small
    widgets: ...
```

![column configuration full-full](images/column-configuration-3.png)

```yaml
columns:
  - size: full
    widgets: ...
  - size: full
    widgets: ...
```

Three equal-width columns can be configured using `medium`:

```yaml
columns:
  - size: medium
    widgets: ...
  - size: medium
    widgets: ...
  - size: medium
    widgets: ...
```

Preview:

![](images/medium-columns-preview.png)

A one-third/two-thirds layout can be configured with `medium` and `full`:

```yaml
columns:
  - size: medium
    widgets: ...
  - size: full
    widgets: ...
```

Preview:

![](images/medium-full-columns-preview.png)

The order can be reversed to place the wider column first:

```yaml
columns:
  - size: full
    widgets: ...
  - size: medium
    widgets: ...
```

## Widgets
Widgets are defined for each column using a `widgets` property. Example:

```yaml
pages:
  - name: Home
    columns:
      - size: small
        widgets:
          - type: weather
            location: London, United Kingdom
```

> [!NOTE]
>
> Currently not all widgets are designed to fit every column size, however some widgets offer different "styles" that help alleviate this limitation.

### Shared Properties
| Name | Type | Required |
| ---- | ---- | -------- |
| type | string | yes |
| title | string | no |
| title-url | string | no |
| hide-header | boolean | no | false |
| cache | string | no |
| css-class | string | no |

#### `type`
Used to specify the widget.

#### `title`
The title of the widget. If left blank it will be defined by the widget.

#### `title-url`
The URL to go to when clicking on the widget's title. If left blank it will be defined by the widget (if available).

#### `hide-header`
When set to `true`, the header (title) of the widget will be hidden. You cannot hide the header of the group widget.

> [!NOTE]
>
> If a widget fails to update, a red dot or circle is shown next to the title of that widget indicating that the it is not working. You will not be able to see this if you hide the header.

#### `cache`
How long to keep the fetched data in memory. The value is a string and must be a number followed by one of s, m, h, d. Examples:

```yaml
cache: 30s # 30 seconds
cache: 5m  # 5 minutes
cache: 2h  # 2 hours
cache: 1d  # 1 day
```

> [!NOTE]
>
> Not all widgets can have their cache duration modified. The calendar and weather widgets update on the hour and this cannot be changed.

#### `css-class`
Set custom CSS classes for the specific widget instance.

### RSS
Display a list of articles from multiple RSS feeds.

Example:

```yaml
- type: rss
  title: News
  style: horizontal-cards
  feeds:
    - url: https://feeds.bloomberg.com/markets/news.rss
      title: Bloomberg
    - url: https://moxie.foxbusiness.com/google-publisher/markets.xml
      title: Fox Business
    - url: https://moxie.foxbusiness.com/google-publisher/technology.xml
      title: Fox Business
```

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| style | string | no | vertical-list |
| feeds | array | yes |
| thumbnail-height | float | no | 10 |
| card-height | float | no | 27 |
| limit | integer | no | 25 |
| preserve-order | bool | no | false |
| single-line-titles | boolean | no | false |
| collapse-after | integer | no | 5 |

##### `limit`
The maximum number of articles to show.

##### `collapse-after`
How many articles are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse.

##### `preserve-order`
When set to `true`, the order of the articles will be preserved as they are in the feeds. Useful if a feed uses its own sorting order which denotes the importance of the articles. If you use this property while having a lot of feeds, it's recommended to set a `limit` to each individual feed since if the first defined feed has 15 articles, the articles from the second feed will start after the 15th article in the list.

##### `single-line-titles`
When set to `true`, truncates the title of each post if it exceeds one line. Only applies when the style is set to `vertical-list`.

##### `style`
Used to change the appearance of the widget. Possible values are:

* `vertical-list` - suitable for `full` and `small` columns
* `detailed-list` - suitable for `full` columns
* `horizontal-cards` - suitable for `full` columns
* `horizontal-cards-2` - suitable for `full` columns

Below is a preview of each style:

`vertical-list`

![preview of vertical-list style for RSS widget](images/rss-feed-vertical-list-preview.png)

`detailed-list`

![preview of detailed-list style for RSS widget](images/rss-widget-detailed-list-preview.png)

`horizontal-cards`

![preview of horizontal-cards style for RSS widget](images/rss-feed-horizontal-cards-preview.png)

`horizontal-cards-2`

![preview of horizontal-cards-2 style for RSS widget](images/rss-widget-horizontal-cards-2-preview.png)

##### `thumbnail-height`
Used to modify the height of the thumbnails. Works only when the style is set to `horizontal-cards`. The default value is `10` and the units are `rem`, if you want to for example double the height of the thumbnails you can set it to `20`.

##### `card-height`
Used to modify the height of cards when using the `horizontal-cards-2` style. The default value is `27` and the units are `rem`.

##### `feeds`
An array of RSS/atom feeds. The title can optionally be changed.

###### Properties for each feed
| Name | Type | Required | Default | Notes |
| ---- | ---- | -------- | ------- | ----- |
| url | string | yes | | |
| title | string | no | the title provided by the feed | |
| hide-categories | boolean | no | false | Only applicable for `detailed-list` style |
| hide-description | boolean | no | false | Only applicable for `detailed-list` style |
| limit | integer | no | | |
| item-link-prefix | string | no | | |
| timeout | duration string | no | | |
| allow-insecure | boolean | no | false | |
| headers | key (string) & value (string) | no | | |
| basic-auth | object | no | | |

###### `limit`
The maximum number of articles to show from that specific feed. Useful if you have a feed which posts a lot of articles frequently and you want to prevent it from excessively pushing down articles from other feeds.

###### `item-link-prefix`
If an RSS feed isn't returning item links with a base domain and Glance has failed to automatically detect the correct domain you can manually add a prefix to each link with this property.

###### `timeout`
The maximum time to wait for the feed request. When omitted, an applicable inherited HTTP timeout may be used.

###### `allow-insecure`
Whether to allow invalid/self-signed certificates when fetching this feed.

###### `basic-auth`
Optional HTTP Basic Authentication credentials for this feed.

##### `headers`
Optionally specify the headers that will be sent with the request. Example:

```yaml
- type: rss
  feeds:
    - url: https://domain.com/rss
      headers:
        User-Agent: Custom User Agent
```

### Videos
Display a list of the latest videos from specific YouTube channels.

Example:

```yaml
- type: videos
  channels:
    - UCXuqSBlHAE6Xw-yeJA0Tunw
    - UCBJycsmduvYEL83R_U4JriQ
    - UCHnyfMqiRRG1u-2MsSQLbXA
```

Preview:
![](images/videos-widget-preview.png)

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| channels | array | yes | |
| playlists | array | no | |
| limit | integer | no | 25 |
| style | string | no | horizontal-cards |
| collapse-after | integer | no | 7 |
| collapse-after-rows | integer | no | 4 |
| include-shorts | boolean | no | false |
| video-url-template | string | no | https://www.youtube.com/watch?v={VIDEO-ID} |

##### `channels`
A list of channels IDs.

One way of getting the ID of a channel is going to the channel's page and clicking on its description:

![](images/videos-channel-description-example.png)

Then scroll down and click on "Share channel", then "Copy channel ID":

![](images/videos-copy-channel-id-example.png)

##### `playlists`

A list of playlist IDs:

```yaml
- type: videos
  playlists:
    - PL8mG-RkN2uTyZZ00ObwZxxoG_nJbs3qec
    - PL8mG-RkN2uTxTK4m_Vl2dYR9yE41kRdBg
```

The playlist ID can be found in its link which is in the form of
```
https://www.youtube.com...&list={ID}&...
```

##### `limit`
The maximum number of videos to show.

##### `collapse-after`
Specify the number of videos to show when using the `vertical-list` style before the "SHOW MORE" button appears.

##### `collapse-after-rows`
Specify the number of rows to show when using the `grid-cards` style before the "SHOW MORE" button appears.

##### `style`
Used to change the appearance of the widget. Possible values are `horizontal-cards`, `vertical-list` and `grid-cards`.

Preview of `vertical-list`:

![](images/videos-widget-vertical-list-preview.png)

Preview of `grid-cards`:

![](images/videos-widget-grid-cards-preview.png)

##### `video-url-template`
Used to replace the default link for videos. Useful when you're running your own YouTube front-end. Example:

```yaml
video-url-template: https://invidious.your-domain.com/watch?v={VIDEO-ID}
```

Placeholders:

`{VIDEO-ID}` - the ID of the video

### Hacker News
Display a list of posts from [Hacker News](https://news.ycombinator.com/).

Example:

```yaml
- type: hacker-news
  limit: 15
  collapse-after: 5
```

Preview:
![](images/hacker-news-widget-preview.png)

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| limit | integer | no | 15 |
| collapse-after | integer | no | 5 |
| comments-url-template | string | no | https://news.ycombinator.com/item?id={POST-ID} |
| sort-by | string | no | top |
| extra-sort-by | string | no | |

##### `comments-url-template`
Used to replace the default link for post comments. Useful if you want to use an alternative front-end. Example:

```yaml
comments-url-template: https://www.hckrnws.com/stories/{POST-ID}
```

Placeholders:

`{POST-ID}` - the ID of the post

##### `sort-by`
Used to specify the order in which the posts should get returned. Possible values are `top`, `new`, and `best`.

##### `extra-sort-by`
Can be used to specify an additional sort which will be applied on top of the already sorted posts. By default does not apply any extra sorting and the only available option is `engagement`.

The `engagement` sort tries to place the posts with the most points and comments on top, also prioritizing recent over old posts.

### Lobsters
Display a list of posts from [Lobsters](https://lobste.rs).

Example:

```yaml
- type: lobsters
  sort-by: hot
  tags:
    - go
    - security
    - linux
  limit: 15
  collapse-after: 5
```

Preview:
![](images/lobsters-widget-preview.png)

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| instance-url | string | no | https://lobste.rs/ |
| custom-url | string | no | |
| limit | integer | no | 15 |
| collapse-after | integer | no | 5 |
| sort-by | string | no | hot |
| tags | array | no | |

##### `instance-url`
The base URL for a lobsters instance hosted somewhere other than on lobste.rs. Example:

```yaml
instance-url: https://www.journalduhacker.net/
```

##### `custom-url`
A custom URL to retrieve lobsters posts from. If this is specified, the `instance-url`, `sort-by` and `tags` properties are ignored.

##### `limit`
The maximum number of posts to show.

##### `collapse-after`
How many posts are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse.

##### `sort-by`
The sort order in which posts are returned. Possible options are `hot` and `new`.

##### `tags`
Limit to posts containing one of the given tags. **You cannot specify a sort order when filtering by tags, it will default to `hot`.**

### Reddit
Display a list of posts from a specific subreddit.

> [!WARNING]
>
> Reddit does not allow unauthorized API access from VPS IPs, if you're hosting Glance on a VPS you will get a 403
> response. As a workaround you can either [register an app on Reddit](https://ssl.reddit.com/prefs/apps/) and use the
> generated ID and secret in the widget configuration to authenticate your requests (see `app-auth` property), use a proxy
> (see `proxy` property) or route the traffic from Glance through a VPN.

Example:

```yaml
- type: reddit
  subreddit: technology
```

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| subreddit | string | yes |  |
| style | string | no | vertical-list |
| show-thumbnails | boolean | no | false |
| show-flairs | boolean | no | false |
| limit | integer | no | 15 |
| collapse-after | integer | no | 5 |
| comments-url-template | string | no | https://www.reddit.com/{POST-PATH} |
| request-url-template | string | no |  |
| proxy | string or multiple parameters | no |  |
| sort-by | string | no | hot |
| top-period | string | no | day |
| search | string | no | |
| extra-sort-by | string | no | |
| app-auth | object | no | |

##### `subreddit`
The subreddit for which to fetch the posts from.

##### `style`
Used to change the appearance of the widget. Possible values are `vertical-list`, `horizontal-cards` and `vertical-cards`. The first two were designed for full columns and the last for small columns.

`vertical-list`

![](images/reddit-widget-preview.png)

`horizontal-cards`

![](images/reddit-widget-horizontal-cards-preview.png)

`vertical-cards`

![](images/reddit-widget-vertical-cards-preview.png)

##### `show-thumbnails`
Shows or hides thumbnails next to the post. This only works if the `style` is `vertical-list`. Preview:

![](images/reddit-widget-vertical-list-thumbnails.png)

> [!NOTE]
>
> Thumbnails don't work for some subreddits due to Reddit's API not returning the thumbnail URL. No workaround for this yet.

##### `show-flairs`
Shows post flairs when set to `true`.

##### `limit`
The maximum number of posts to show.

##### `collapse-after`
How many posts are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse. Not available when using the `vertical-cards` and `horizontal-cards` styles.

##### `comments-url-template`
Used to replace the default link for post comments. Useful if you want to use the old Reddit design or any other 3rd party front-end. Example:

```yaml
comments-url-template: https://old.reddit.com/{POST-PATH}
```

Placeholders:

`{POST-PATH}` - the full path to the post, such as:

```
r/selfhosted/comments/bsp01i/welcome_to_rselfhosted_please_read_this_first/
```

`{POST-ID}` - the ID that comes after `/comments/`

`{SUBREDDIT}` - the subreddit name

##### `request-url-template`
A custom request URL that will be used to fetch the data. This is useful when you're hosting Glance on a VPS where Reddit is blocking the requests and you want to route them through a proxy that accepts the URL as either a part of the path or a query parameter.

Placeholders:

`{REQUEST-URL}` - will be templated and replaced with the expanded request URL (i.e. https://www.reddit.com/r/selfhosted/hot.json). Example:

```
https://proxy/{REQUEST-URL}
https://your.proxy/?url={REQUEST-URL}
```

##### `proxy`
A custom HTTP/HTTPS proxy URL that will be used to fetch the data. This is useful when you're hosting Glance on a VPS where Reddit is blocking the requests and you want to bypass the restriction by routing the requests through a proxy. Example:

```yaml
proxy: http://user:pass@proxy.com:8080
proxy: https://user:pass@proxy.com:443
```

Alternatively, you can specify the proxy URL as well as additional options by using multiple parameters:

```yaml
proxy:
  url: http://proxy.com:8080
  allow-insecure: true
  timeout: 10s
```

###### `allow-insecure`
When set to `true`, allows the use of insecure connections such as when the proxy has a self-signed certificate.

###### `timeout`
The maximum time to wait for a response from the proxy. The value is a string and must be a number followed by one of s, m, h, d. Example: `10s` for 10 seconds, `1m` for 1 minute, etc

##### `sort-by`
Can be used to specify the order in which the posts should get returned. Possible values are `hot`, `new`, `top` and `rising`.

##### `top-period`
Available only when `sort-by` is set to `top`. Possible values are `hour`, `day`, `week`, `month`, `year` and `all`.

##### `search`
Keywords to search for. Searching within specific fields is also possible, **though keep in mind that Reddit may remove the ability to use any of these at any time**:

![](images/reddit-field-search.png)

##### `extra-sort-by`
Can be used to specify an additional sort which will be applied on top of the already sorted posts. By default does not apply any extra sorting and the only available option is `engagement`.

The `engagement` sort tries to place the posts with the most points and comments on top, also prioritizing recent over old posts.

##### `app-auth`
```yaml
widgets:
  - type: reddit
    subreddit: technology
    app-auth:
      name: ${REDDIT_APP_NAME}
      id: ${REDDIT_APP_CLIENT_ID}
      secret: ${REDDIT_APP_SECRET}
```

To register an app on Reddit, go to [this page](https://ssl.reddit.com/prefs/apps/).

### Search Widget
Display a search bar that can be used to search for specific terms on various search engines.

Example:

```yaml
- type: search
  search-engine: duckduckgo
  bangs:
    - title: YouTube
      shortcut: "!yt"
      url: https://www.youtube.com/results?search_query={QUERY}
```

Preview:

![](images/search-widget-preview.png)

#### Keyboard shortcuts
| Keys | Action | Condition |
| ---- | ------ | --------- |
| <kbd>S</kbd> | Focus the search bar | Not already focused on another input field |
| <kbd>Enter</kbd> | Perform search in the same tab | Search input is focused and not empty |
| <kbd>Ctrl</kbd> + <kbd>Enter</kbd> | Perform search in a new tab | Search input is focused and not empty |
| <kbd>Escape</kbd> | Leave focus | Search input is focused |
| <kbd>Up</kbd> | Insert the last search query since the page was opened into the input field | Search input is focused |

> [!TIP]
>
> You can use the property `new-tab` with a value of `true` if you want to show search results in a new tab by default. <kbd>Ctrl</kbd> + <kbd>Enter</kbd> will then show results in the same tab.

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| search-engine | string | no | duckduckgo |
| new-tab | boolean | no | false |
| autofocus | boolean | no | false |
| target | string | no | _blank |
| placeholder | string | no | Type here to search… |
| bangs | array | no | |

##### `search-engine`
Either a value from the table below or a URL to a custom search engine. Use `{QUERY}` to indicate where the query value gets placed.

| Name | URL |
| ---- | --- |
| duckduckgo | `https://duckduckgo.com/?q={QUERY}` |
| google | `https://www.google.com/search?q={QUERY}` |
| bing | `https://www.bing.com/search?q={QUERY}` |
| perplexity | `https://www.perplexity.ai/search?q={QUERY}` |
| kagi | `https://kagi.com/search?q={QUERY}` |
| startpage | `https://www.startpage.com/search?q={QUERY}` |

##### `new-tab`
When set to `true`, swaps the shortcuts for showing results in the same or new tab, defaulting to showing results in a new tab.

##### `autofocus`
When set to `true`, automatically focuses the search input on page load.

##### `target`
The target to use when opening the search results in a new tab. Possible values are `_blank`, `_self`, `_parent` and `_top`.

##### `placeholder`
When set, modifies the text displayed in the input field before typing.

##### `bangs`
What now? [Bangs](https://duckduckgo.com/bangs). They're shortcuts that allow you to use the same search box for many different sites. Assuming you have it configured, if for example you start your search input with `!yt` you'd be able to perform a search on YouTube:

![](images/search-widget-bangs-preview.png)

##### Properties for each bang
| Name | Type | Required |
| ---- | ---- | -------- |
| title | string | no |
| shortcut | string | yes |
| url | string | yes |

###### `title`
Optional title that will appear on the right side of the search bar when the query starts with the associated shortcut.

###### `shortcut`
Any value you wish to use as the shortcut for the search engine. It does not have to start with `!`.

> [!IMPORTANT]
>
> In YAML some characters have special meaning when placed in the beginning of a value. If your shortcut starts with `!` (and potentially some other special characters) you'll have to wrap the value in quotes:
> ```yaml
> shortcut: "!yt"
>```

###### `url`
The URL of the search engine. Use `{QUERY}` to indicate where the query value gets placed. Examples:

```yaml
url: https://www.reddit.com/search?q={QUERY}
url: https://store.steampowered.com/search/?term={QUERY}
url: https://www.amazon.com/s?k={QUERY}
```

### Group
Group multiple widgets into one using tabs. Widgets are defined using a `widgets` property exactly as you would on a page column.

Groups can also contain other groups, allowing you to create multiple levels of tabbed navigation. A `split-column` widget cannot be placed inside a `group` widget.

Example:

```yaml
- type: group
  widgets:
    - type: reddit
      subreddit: gamingnews
      show-thumbnails: true
      collapse-after: 6
    - type: reddit
      subreddit: games
    - type: reddit
      subreddit: pcgaming
      show-thumbnails: true
```

Preview:

![](images/group-widget-preview.png)

#### Nested groups

A group can contain other group widgets to create multiple levels of tabs. The title of each nested group is used as the tab title in its parent group.

For example:

```yaml
- type: group
  widgets:
    - type: group
      title: Major Leagues
      widgets:
        - type: custom-api
          title: MLB
          url: https://example.com/mlb.json
          template: |
            <p>{{ .JSON.String "name" }}</p>

        - type: custom-api
          title: NFL
          url: https://example.com/nfl.json
          template: |
            <p>{{ .JSON.String "name" }}</p>

    - type: group
      title: College
      widgets:
        - type: custom-api
          title: NCAAF
          url: https://example.com/ncaaf.json
          template: |
            <p>{{ .JSON.String "name" }}</p>

        - type: custom-api
          title: NCAAM
          url: https://example.com/ncaam.json
          template: |
            <p>{{ .JSON.String "name" }}</p>
```

This produces an outer set of tabs for `Major Leagues` and `College`, with each tab containing its own inner set of tabs.

Preview:

![](images/nested-groups-preview.png)

#### Sharing properties

To avoid repetition you can use [YAML anchors](https://support.atlassian.com/bitbucket-cloud/docs/yaml-anchors/) and share properties between widgets.

Example:

```yaml
- type: group
  define: &shared-properties
      type: reddit
      show-thumbnails: true
      collapse-after: 6
  widgets:
    - subreddit: gamingnews
      <<: *shared-properties
    - subreddit: games
      <<: *shared-properties
    - subreddit: pcgaming
      <<: *shared-properties
```

### Stack
Stack multiple widgets vertically and treat them as a single widget when used inside a group.

This is useful when you want one group tab to display multiple widgets at the same time instead of displaying only a single widget.

Widgets are defined using a `widgets` property exactly as you would on a page column.

Example:

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

In this example, the group has two tabs: `News` and `Social`. Selecting `News` displays both the Hacker News and Lobsters widgets vertically, while selecting `Social` displays both Reddit widgets vertically.

Preview:

![](images/stack-widget-preview.png)

### Status Bar
Display compact information from supported existing widgets in a full-width status bar.

A status bar can only be placed directly in a page's `head-widgets` or `bottom-widgets`. It cannot be placed in a column or nested inside another widget such as a `group`, `stack`, `split-column`, or another `status-bar`.

A page can contain one or multiple status bars in `head-widgets`, one or multiple status bars in `bottom-widgets`, or status bars in both sections.

Preview:

![](images/status-bar-preview.png)

Supported child widget types are:

* `weather`
* `markets`
* `rss`

Child widgets are configured through the `widgets` property using their normal widget configuration. They retain their existing provider fetching, caching, refresh, recovery, error handling, limits, sorting, and link behavior, but are displayed using a compact status-bar presentation rather than their normal full widget layout.

Example:

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

    columns:
      - size: full
        widgets:
          - type: hacker-news
```

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| mode | string | no | ticker |
| widgets | array | yes | |

##### `mode`

Controls how the compact items are laid out.

* `ticker` continuously scrolls the items horizontally. Scrolling pauses while the status bar is hovered or focused. When reduced motion is requested by the browser or operating system, the items are displayed statically instead.
* `wrap` displays the items statically and allows them to wrap onto additional lines when necessary.

Possible values are `ticker` and `wrap`.

##### `widgets`

An array of supported Glance widgets to display in compact form. Each child uses the normal configuration for its widget type.

The status bar intentionally provides an alternate presentation of existing widgets rather than a separate data-source system. Weather uses the configured location and units, Markets preserves configured symbols, names, sorting and links, and RSS preserves its configured feeds, limits, ordering and article links.

Where equivalent underlying resources are requested by multiple configured widgets, Glance shares the applicable fetched resource work while keeping each widget's configuration and presentation independent.

Presentation-specific options of a child widget that apply only to its normal full layout do not change the status bar's compact renderer.


### Split Column
Splits a full sized column in half, allowing you to place widgets side by side horizontally. This is converted to a single column on mobile devices or if not enough width is available. Widgets are defined using a `widgets` property exactly as you would on a page column.

Two widgets side by side in a `full` column:

![](images/split-column-widget-preview.png)

<details>
<summary>View <code>glance.yml</code></summary>
<br>

```yaml
# ...
- size: full
  widgets:
    - type: split-column
      widgets:
        - type: hacker-news
          collapse-after: 3
        - type: lobsters
          collapse-after: 3

    - type: videos
# ...
```
</details>
<br>

You can also achieve a number of different full page layouts using just this widget, such as:

3 column layout where all columns have equal width:

![](images/split-column-widget-3-columns.png)

<details>
<summary>View <code>glance.yml</code></summary>
<br>

```yaml
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: split-column
            max-columns: 3
            widgets:
              - type: reddit
                subreddit: selfhosted
                collapse-after: 15
              - type: reddit
                subreddit: homelab
                collapse-after: 15
              - type: reddit
                subreddit: sysadmin
                collapse-after: 15
```
</details>
<br>

4 column layout where all columns have equal width (and the page is set to `width: wide`):

![](images/split-column-widget-4-columns.png)

<details>
<summary>View <code>glance.yml</code></summary>
<br>

```yaml
pages:
  - name: Home
    width: wide
    columns:
      - size: full
        widgets:
          - type: split-column
            max-columns: 4
            widgets:
              - type: reddit
                subreddit: selfhosted
                collapse-after: 15
              - type: reddit
                subreddit: homelab
                collapse-after: 15
              - type: reddit
                subreddit: linux
                collapse-after: 15
              - type: reddit
                subreddit: sysadmin
                collapse-after: 15
```
</details>
<br>

Masonry layout with up to 5 columns where all columns have equal width (and the page is set to `width: wide`):

![](images/split-column-widget-masonry.png)

<details>
<summary>View <code>glance.yml</code></summary>
<br>

```yaml
define:
  - &subreddit-settings
    type: reddit
    collapse-after: 5

pages:
  - name: Home
    width: wide
    columns:
      - size: full
        widgets:
          - type: split-column
            max-columns: 5
            widgets:
              - subreddit: selfhosted
                <<: *subreddit-settings
              - subreddit: homelab
                <<: *subreddit-settings
              - subreddit: linux
                <<: *subreddit-settings
              - subreddit: sysadmin
                <<: *subreddit-settings
              - subreddit: DevOps
                <<: *subreddit-settings
              - subreddit: Networking
                <<: *subreddit-settings
              - subreddit: DataHoarding
                <<: *subreddit-settings
              - subreddit: OpenSource
                <<: *subreddit-settings
              - subreddit: Privacy
                <<: *subreddit-settings
              - subreddit: FreeSoftware
                <<: *subreddit-settings
```
</details>
<br>

A `split-column` can contain other widget types, including `group` widgets. A `split-column` widget cannot be placed inside a `group` widget.

### Custom API

Display data from a JSON API using a custom template.

> [!NOTE]
>
> The configuration of this widget requires some basic knowledge of programming, HTML, CSS, the Go template language and Glance-specific concepts.

Examples:

![](images/custom-api-preview-1.png)

<details>
<summary>View <code>glance.yml</code></summary>
<br>

```yaml
- type: custom-api
  title: Random Fact
  cache: 6h
  url: https://uselessfacts.jsph.pl/api/v2/facts/random
  template: |
    <p class="size-h4 color-paragraph">{{ .JSON.String "text" }}</p>
```
</details>
<br>

![](images/custom-api-preview-2.png)

<details>
<summary>View <code>glance.yml</code></summary>
<br>

```yaml
- type: custom-api
  title: Immich stats
  cache: 1d
  url: https://${IMMICH_URL}/api/server/statistics
  headers:
    x-api-key: ${IMMICH_API_KEY}
    Accept: application/json
  template: |
    <div class="flex justify-between text-center">
      <div>
          <div class="color-highlight size-h3">{{ .JSON.Int "photos" | formatNumber }}</div>
          <div class="size-h6">PHOTOS</div>
      </div>
      <div>
          <div class="color-highlight size-h3">{{ .JSON.Int "videos" | formatNumber }}</div>
          <div class="size-h6">VIDEOS</div>
      </div>
      <div>
          <div class="color-highlight size-h3">{{ div (.JSON.Int "usage" | toFloat) 1073741824 | toInt | formatNumber }}GB</div>
          <div class="size-h6">USAGE</div>
      </div>
    </div>
```
</details>
<br>

![](images/custom-api-preview-3.png)

<details>
<summary>View <code>glance.yml</code></summary>
<br>

```yaml
- type: custom-api
  title: Steam Specials
  cache: 12h
  url: https://store.steampowered.com/api/featuredcategories?cc=us
  template: |
    <ul class="list list-gap-10 collapsible-container" data-collapse-after="5">
    {{ range .JSON.Array "specials.items" }}
      <li>
        <a class="size-h4 color-highlight block text-truncate" href="https://store.steampowered.com/app/{{ .Int "id" }}/">{{ .String "name" }}</a>
        <ul class="list-horizontal-text">
          <li>{{ div (.Int "final_price" | toFloat) 100 | printf "$%.2f" }}</li>
          {{ $discount := .Int "discount_percent" }}
          <li{{ if ge $discount 40 }} class="color-positive"{{ end }}>{{ $discount }}% off</li>
        </ul>
      </li>
    {{ end }}
    </ul>
```
</details>

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| url | string | no | |
| headers | key (string) & value (string) | no | |
| method | string | no | GET |
| body-type | string | no | json |
| body | any | no | |
| basic-auth | map | no | |
| frameless | boolean | no | false |
| allow-insecure | boolean | no | false |
| skip-json-validation | boolean | no | false |
| template | string | yes | |
| options | map | no | |
| parameters | key (string) & value (string|array) | no | |
| subrequests | map of requests | no | |
| timeout | duration string | no | 5s |

##### Stale fallback

After a `custom-api` widget has completed at least one successful refresh, Glance preserves that last successfully rendered content if a later refresh fails. The previous content remains visible and is marked with a `STALE` indicator showing how long ago the last successful refresh occurred.

The `cache` setting continues to control how often fresh data is requested. A failed refresh does not replace the last successful content or reset its age. Glance retries the update according to its normal retry schedule, and the stale indicator is automatically cleared when a later refresh succeeds.

If the initial request fails before any content has been successfully rendered, there is no previous content to fall back to and the normal widget error behavior is used.

A non-success HTTP response with an empty body is treated as a failed refresh. Non-success responses containing a body retain the existing Custom API behavior and remain available to templates through `.Response`, allowing templates to handle status codes explicitly.

##### `url`
The URL to fetch the data from. It must be accessible from the server that Glance is running on.

##### `headers`
Optionally specify the headers that will be sent with the request. Example:

```yaml
headers:
  x-api-key: your-api-key
  Accept: application/json
```

##### `method`
The HTTP method to use when making the request. Possible values are `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` and `HEAD`.

##### `body-type`
The type of the body that will be sent with the request. Possible values are `json`, and `string`.

##### `body`
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

##### `basic-auth`
Optionally specify credentials to be sent with the request using HTTP basic authentication. Example:

```yaml
basic-auth:
  username: your-username
  password: your-password
```

##### `frameless`
When set to `true`, removes the border and padding around the widget.

##### `allow-insecure`
Whether to ignore invalid/self-signed certificates.

##### `skip-json-validation`
When set to `true`, skips the JSON validation step. This is useful when the API returns JSON Lines/newline-delimited JSON, which is a format that consists of several JSON objects separated by newlines.

##### `template`
The template that will be used to display the data. It relies on Go's `html/template` package so it's recommended to go through [its documentation](https://pkg.go.dev/text/template) to understand how to do basic things such as conditionals, loops, etc. In addition, it also uses [tidwall's gjson](https://github.com/tidwall/gjson) package to parse the JSON data so it's worth going through its documentation if you want to use more advanced JSON selectors. You can view additional examples with explanations and function definitions [here](custom-api.md).

##### `options`
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

Currently, the available methods on the `.Options` object are: `StringOr`, `IntOr`, `BoolOr` and `FloatOr`.

</details>

##### `parameters`
A list of keys and values that will be sent to the custom-api as query paramters.

##### `subrequests`
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

#### `timeout`
The maximum amount of time to wait for the API response. The value must be an integer followed by one of `s` (seconds), `m` (minutes), `h` (hours), or `d` (days), for example `10s`, `1m`, or `2h`.

The timeout can be configured independently for the primary request and each subrequest.

Defaults to `5s`.

### Extension
Display a widget provided by an external source (3rd party). If you want to learn more about developing extensions, checkout the [extensions documentation](extensions.md) (WIP).

```yaml
- type: extension
  url: https://domain.com/widget/display-a-message
  allow-potentially-dangerous-html: true
  parameters:
    message: Hello, world!
```

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| url | string | yes | |
| fallback-content-type | string | no | |
| allow-potentially-dangerous-html | boolean | no | false |
| timeout | duration string | no | |
| allow-insecure | boolean | no | false |
| headers | key & value | no | |
| basic-auth | object | no | |
| parameters | key & value | no | |

##### `url`
The URL of the extension. **Note that the query gets stripped from this URL and the one defined by `parameters` gets used instead.**

##### `fallback-content-type`
Optionally specify the fallback content type of the extension if the URL does not return a valid `Widget-Content-Type` header. Currently the only supported value for this property is `html`.

##### `timeout`
The maximum time to wait for the extension request.

##### `allow-insecure`
Whether to allow invalid/self-signed certificates when requesting the extension.

##### `headers`
Optionally specify the headers that will be sent with the request. Example:

```yaml
headers:
  x-api-key: ${SECRET_KEY}
```

##### `basic-auth`
Optional HTTP Basic Authentication credentials for the extension request.

##### `allow-potentially-dangerous-html`
Whether to allow the extension to display HTML.

> [!WARNING]
>
> There's a reason this property is scary-sounding. It's intended to be used by developers who are comfortable with developing and using their own extensions. Do not enable it if you have no idea what it means or if you're not **absolutely sure** that the extension URL you're using is safe.

##### `parameters`
A list of keys and values that will be sent to the extension as query paramters.

### Weather
Display weather information for a specific location. The data is provided by https://open-meteo.com/.

Example:

```yaml
- type: weather
  units: metric
  hour-format: 12h
  location: London, United Kingdom
```

> [!NOTE]
>
> US cities which have common names can have their state specified as the second parameter as such:
>
> * Greenville, North Carolina, United States
> * Greenville, South Carolina, United States
> * Greenville, Mississippi, United States


Preview:

![](images/weather-widget-preview.png)

Each bar represents a 2 hour interval. The yellow background represents sunrise and sunset. The blue dots represent the times of the day where there is a high chance for precipitation. You can hover over the bars to view the exact temperature for that time.

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| location | string | yes |  |
| units | string | no | metric |
| hour-format | string | no | 12h |
| hide-location | boolean | no | false |
| show-area-name | boolean | no | false |

##### `location`
The name of the city and country to fetch weather information for. Attempting to launch the applcation with an invalid location will result in an error. You can use the [gecoding API page](https://open-meteo.com/en/docs/geocoding-api) to search for your specific location. Glance will use the first result from the list if there are multiple.

##### `units`
Whether to show the temperature in celsius or fahrenheit, possible values are `metric` or `imperial`.

#### `hour-format`
Whether to show the hours of the day in 12-hour format or 24-hour format. Possible values are `12h` and `24h`.

##### `hide-location`
Optionally don't display the location name on the widget.

##### `show-area-name`
Whether to display the state/administrative area in the location name. If set to `true` the location will be displayed as:

```
Greenville, North Carolina, United States
```

Otherwise, if set to `false` (which is the default) it'll be displayed as:

```
Greenville, United States
```

### Todo

A simple to-do list that allows you to add, edit and delete tasks. The tasks are stored in the browser's local storage.

Example:

```yaml
- type: to-do
```

Preview:

![](images/todo-widget-preview.png)

To reorder tasks, drag and drop them by grabbing the top side of the task:

![](images/reorder-todo-tasks-preview.gif)

To delete a task, hover over it and click on the trash icon.

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| id | string | no | |

##### `id`

The ID of the todo list. If you want to have multiple todo lists, you must specify a different ID for each one. The ID is used to store the tasks in the browser's local storage. This means that if you have multiple todo lists with the same ID, they will share the same tasks.

#### Keyboard shortcuts
| Keys | Action | Condition |
| ---- | ------ | --------- |
| <kbd>Enter</kbd> | Add a task to the bottom of the list | When the "Add a task" field is focused |
| <kbd>Ctrl</kbd> + <kbd>Enter</kbd> | Add a task to the top of the list | When the "Add a task" field is focused |
| <kbd>Down Arrow</kbd> | Focus the last task that was added | When the "Add a task" field is focused |
| <kbd>Escape</kbd> | Focus the "Add a task" field | When a task is focused |

### Timer

Create countdown timers for specific dates and times. Timers can be added, edited, deleted, and reordered directly in the widget.

Timer data is stored in the browser's local storage and is not stored or synchronized by the Glance server. Timers therefore remain specific to the browser and browser profile where they were created.

Example:

```yaml
- type: timer
  id: important-dates
  hour-format: 12h
```

Preview:

![](images/timer-widget-preview.png)

To edit a timer, click its name or target date and time. To reorder timers, drag and drop them by grabbing the top side of a timer. Use the trash icon to delete a timer.

Countdowns display days, hours, and minutes as applicable. Once a target time has passed, the timer remains visible and shows the elapsed time followed by `ago`.

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| id | string | no | |
| hour-format | string | no | 12h |

##### `id`

The ID used to store the timers in the browser's local storage. Timer widgets with the same ID share the same timers when viewed in the same browser profile. Specify different IDs for independent timer lists.

##### `hour-format`

Whether target times are displayed in 12-hour or 24-hour format. Possible values are `12h` and `24h`.

Dates and times entered in the widget are interpreted as local calendar time in the browser. The Timer widget does not perform server-side timezone conversion or synchronization.

### Unit Converter

Convert values between commonly used measurement, scientific, electrical, and digital-information units directly in the browser.

Select a category, source unit, value, and destination unit. The result updates immediately whenever the value or either unit selection changes.

Example:

```yaml
- type: unit-converter
```

Preview:

![](images/unit-converter-preview.png)

The widget includes 35 conversion categories and 379 units covering common measurement, scientific, digital-information, electrical, and fuel-economy conversions.

No external service or network connection is required. The conversion catalog is built into Glance and conversions are performed locally in the browser.

The widget has no required configuration properties beyond the standard [shared widget properties](#shared-properties). Each category provides default source and destination units, which can be changed interactively in the widget.

### Calculator

Perform calculations directly in the browser using a native Glance calculator.

The calculator supports standard arithmetic together with percentage, reciprocal, square, square root, exponent, nth-root, parentheses, sign change, clear-entry, clear, and backspace controls. Expressions support operator precedence and parentheses, and the calculator can also be operated from the keyboard.

Example:

```yaml
- type: calculator
```

Preview:

![](images/calculator-preview.png)

No external service or network connection is required. Calculations are performed locally in the browser.

The widget has no required configuration properties beyond the standard [shared widget properties](#shared-properties).

### Monitor

Display a list of sites and whether they are reachable (online) or not. This is determined by sending a GET request to the specified URL, if the response is 200 then the site is OK. The time it took to receive a response is also shown in milliseconds.

Example:

```yaml
- type: monitor
  cache: 1m
  title: Services
  sites:
    - title: Jellyfin
      url: https://jellyfin.yourdomain.com
      icon: /assets/jellyfin-logo.png
    - title: Gitea
      url: https://gitea.yourdomain.com
      icon: /assets/gitea-logo.png
    - title: Immich
      url: https://immich.yourdomain.com
      icon: /assets/immich-logo.png
    - title: AdGuard Home
      url: https://adguard.yourdomain.com
      icon: /assets/adguard-logo.png
    - title: Vaultwarden
      url: https://vault.yourdomain.com
      icon: /assets/vaultwarden-logo.png
```

Preview:

![](images/monitor-widget-preview.png)

You can hover over the "ERROR" text to view more information.

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| sites | array | yes | |
| style | string | no | |
| show-failing-only | boolean | no | false |

##### `show-failing-only`
Shows only a list of failing sites when set to `true`.

##### `style`
Used to change the appearance of the widget. Possible values are `compact`.

Preview of `compact`:

![](images/monitor-widget-compact-preview.png)

##### `sites`

Properties for each site:

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| title | string | yes | |
| url | string | yes | |
| check-url | string | no | |
| error-url | string | no | |
| icon | string | no | |
| timeout | string | no | 3s |
| allow-insecure | boolean | no | false |
| headers | key & value | no | |
| same-tab | boolean | no | false |
| alt-status-codes | array | no | |
| basic-auth | object | no | |

`title`

The title used to indicate the site.

`url`

The URL of the monitored service, which must be reachable by Glance, and will be used as the link to go to when clicking on the title. If `check-url` is not specified, this is used as the status check.

`check-url`

The URL which will be requested and its response will determine the status of the site. If not specified, the `url` property is used.

`error-url`

If the monitored service returns an error, the user will be redirected here. If not specified, the `url` property is used.

`icon`

See [Icons](#icons) for more information on how to specify icons.

`timeout`

How long to wait for a response from the server before considering it unreachable. The value is a string and must be a number followed by one of s, m, h, d. Example: `5s` for 5 seconds, `1m` for 1 minute, etc.

`allow-insecure`

Whether to ignore invalid/self-signed certificates.

`same-tab`

Whether to open the link in the same or a new tab.

`alt-status-codes`

Status codes other than 200 that you want to return "OK".

```yaml
alt-status-codes:
  - 403
```

`basic-auth`

HTTP Basic Authentication credentials for protected sites.

```yaml
basic-auth:
  username: your-username
  password: your-password
```

### Releases
Display a list of latest releases for specific repositories on Github, GitLab, Codeberg or Docker Hub.

Example:

```yaml
- type: releases
  show-source-icon: true
  repositories:
    - go-gitea/gitea
    - jellyfin/jellyfin
    - glanceapp/glance
    - codeberg:redict/redict
    - gitlab:fdroid/fdroidclient
    - dockerhub:gotify/server
```

Preview:

![](images/releases-widget-preview.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| repositories | array | yes |  |
| show-source-icon | boolean | no | false |  |
| token | string | no | |
| gitlab-token | string | no | |
| limit | integer | no | 10 |
| collapse-after | integer | no | 5 |

##### `repositories`
A list of repositores to fetch the latest release for. Only the name/repo is required, not the full URL. A prefix can be specified for repositories hosted elsewhere such as GitLab, Codeberg and Docker Hub. Example:

```yaml
repositories:
  - gitlab:inkscape/inkscape
  - dockerhub:glanceapp/glance
  - codeberg:redict/redict
```

Official images on Docker Hub can be specified by omitting the owner:

```yaml
repositories:
  - dockerhub:nginx
  - dockerhub:node
  - dockerhub:alpine
```

You can also specify exact tags for Docker Hub images:

```yaml
repositories:
  - dockerhub:nginx:latest
  - dockerhub:nginx:stable-alpine
```

To include prereleases you can specify the repository as an object and use the `include-prereleases` property:

**Note: This feature is currently only available for GitHub repositories.**

```yaml
repositories:
  - gitlab:inkscape/inkscape
  - repository: glanceapp/glance
    include-prereleases: true
  - codeberg:redict/redict
```

##### `show-source-icon`
Shows an icon of the source (GitHub/GitLab/Codeberg/Docker Hub) next to the repository name when set to `true`.

##### `token`
Without authentication Github allows for up to 60 requests per hour. You can easily exceed this limit and start seeing errors if you're tracking lots of repositories or your cache time is low. To circumvent this you can [create a read only token from your Github account](https://github.com/settings/personal-access-tokens/new) and provide it here.

You can also specify the value for this token through an ENV variable using the syntax `${GITHUB_TOKEN}` where `GITHUB_TOKEN` is the name of the variable that holds the token. If you've installed Glance through docker you can specify the token in your docker-compose:

```yaml
services:
  glance:
    image: glanceapp/glance
    environment:
      - GITHUB_TOKEN=<your token>
```

and then use it in your `glance.yml` like this:

```yaml
- type: releases
  token: ${GITHUB_TOKEN}
  repositories: ...
```

This way you can safely check your `glance.yml` in version control without exposing the token.

##### `gitlab-token`
Same as the above but used when fetching GitLab releases.

##### `limit`
The maximum number of releases to show.

#### `collapse-after`
How many releases are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse.

### Docker Containers

Display the status of your Docker containers along with an icon and an optional short description.

![](images/docker-containers-preview.png)

```yaml
- type: docker-containers
  hide-by-default: false
```

> [!NOTE]
>
> The widget requires access to `docker.sock`. If you're running Glance inside a container, this can be done by mounting the socket as a volume:
>
> ```yaml
> services:
>   glance:
>     image: glanceapp/glance
>     volumes:
>       - /var/run/docker.sock:/var/run/docker.sock
> ```

Configuration of the containers is done via labels applied to each container:

```yaml
  jellyfin:
    image: jellyfin/jellyfin:latest
    labels:
      glance.name: Jellyfin
      glance.icon: si:jellyfin
      glance.url: https://jellyfin.domain.com
      glance.description: Movies & shows
```

Alternatively, you can also define the values within your `glance.yml` via the `containers` property, where the key is the container name and each value is the same as the labels but without the "glance." prefix:

```yaml
- type: docker-containers
  containers:
    container_name_1:
      name: Container Name
      description: Description of the container
      url: https://container.domain.com
      icon: si:container-icon
      hide: false
```

For services with multiple containers you can specify a `glance.id` on the "main" container and `glance.parent` on each "child" container:

<details>
<summary>View <code>docker-compose.yml</code></summary>
<br>

```yaml
services:
  immich-server:
    image: ghcr.io/immich-app/immich-server
    labels:
      glance.name: Immich
      glance.icon: si:immich
      glance.url: https://immich.domain.com
      glance.description: Image & video management
      glance.id: immich

  redis:
    image: docker.io/redis:6.2-alpine
    labels:
      glance.parent: immich
      glance.name: Redis

  database:
    image: docker.io/tensorchord/pgvecto-rs:pg14-v0.2.0
    labels:
      glance.parent: immich
      glance.name: DB

  proxy:
    image: nginx:stable
    labels:
      glance.parent: immich
      glance.name: Proxy
```
</details>
<br>

This will place all child containers under the `Immich` container when hovering over its icon:

![](images/docker-container-parent.png)

If any of the child containers are down, their status will propagate up to the parent container:

![](images/docker-container-parent2.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| hide-by-default | boolean | no | false |
| format-container-names | boolean | no | false |
| sock-path | string | no | /var/run/docker.sock |
| category | string | no | |
| running-only | boolean | no | false |

##### `hide-by-default`
Whether to hide the containers by default. If set to `true` you'll have to manually add a `glance.hide: false` label to each container you want to display. By default all containers will be shown and if you want to hide a specific container you can add a `glance.hide: true` label.

##### `format-container-names`
When set to `true`, automatically converts container names such as `container_name_1` into `Container Name 1`.

##### `sock-path`
The path to the Docker socket. This can also be a [remote socket](https://docs.docker.com/engine/daemon/remote-access/) or proxied socket using something like [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy).

If the socket path starts with `tcp://` or `http://`, it will be treated as a remote socket. Anything else will be treated as a path to a Unix socket.

##### `category`
Filter to only the containers which have this category specified via the `glance.category` label. Useful if you want to have multiple containers widgets, each showing a different set of containers.

<details>
<summary>View example</summary>
<br>


```yaml
services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    labels:
      glance.name: Jellyfin
      glance.icon: si:jellyfin
      glance.url: https://jellyfin.domain.com
      glance.category: media

  gitea:
    image: gitea/gitea:latest
    labels:
      glance.name: Gitea
      glance.icon: si:gitea
      glance.url: https://gitea.domain.com
      glance.category: dev-tools

  vaultwarden:
    image: vaultwarden/server:latest
    labels:
      glance.name: Vaultwarden
      glance.icon: si:vaultwarden
      glance.url: https://vaultwarden.domain.com
      glance.category: dev-tools
```

Then you can use the `category` property to filter the containers:

```yaml
- type: docker-containers
  title: Dev tool containers
  category: dev-tools

- type: docker-containers
  title: Media containers
  category: media
```

</details>

##### `running-only`
Whether to only show running containers. If set to `true` only containers that are currently running will be displayed. If set to `false` all containers will be displayed regardless of their state.

#### Labels
| Name | Description |
| ---- | ----------- |
| glance.name | The name displayed in the UI. If not specified, the name of the container will be used. |
| glance.icon | See [Icons](#icons) for more information on how to specify icons |
| glance.url | The URL that the user will be redirected to when clicking on the container. |
| glance.same-tab | Whether to open the link in the same or a new tab. Default is `false`. |
| glance.description | A short description displayed in the UI. Default is empty. |
| glance.hide | Whether to hide the container. If set to `true` the container will not be displayed. Defaults to `false`. |
| glance.id | The custom ID of the container. Used to group containers under a single parent. |
| glance.parent | The ID of the parent container. Used to group containers under a single parent. |
| glance.category | The category of the container. Used to filter containers by category. |

### DNS Stats
Display statistics from a self-hosted ad-blocking DNS resolver such as AdGuard Home, Pi-hole, or Technitium.

Example:

```yaml
- type: dns-stats
  service: adguard
  url: https://adguard.domain.com/
  username: admin
  password: ${ADGUARD_PASSWORD}
```

Preview:

![](images/dns-stats-widget-preview.png)

> [!NOTE]
>
> When using AdGuard Home the 3rd statistic on top will be the average latency and when using Pi-hole or Technitium it will be the total number of blocked domains from all adlists.

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| service | string | no | pihole |
| timeout | duration string | no | |
| allow-insecure | bool | no | false |
| url | string | yes |  |
| username | string | when service is `adguard` |  |
| password | string | when service is `adguard` or `pihole-v6` |  |
| token | string | when service is `pihole` |  |
| hide-graph | bool | no | false |
| hide-top-domains | bool | no | false |
| hour-format | string | no | 12h |

##### `service`
Either `adguard`, `technitium`, or `pihole` (major version 5 and below) or `pihole-v6` (major version 6 and above).

##### `timeout`
The maximum time to wait for a response from the DNS service.

##### `allow-insecure`
Whether to allow invalid/self-signed certificates when making the request to the service.

##### `url`
The base URL of the service.

##### `username`
Only required when using AdGuard Home. The username used to log into the admin dashboard.

##### `password`
Required when using AdGuard Home, where the password is the one used to log into the admin dashboard.

For Pi-hole version 6+, this field is required if you have set a password to log into Pi-hole. You can either use the password you use to log into the admin dashboard or the application password, which can be found in `Settings -> Web Interface / API -> Configure app password`.

##### `token`
Required when using Pi-hole major version 5 or earlier. The API token which can be found in `Settings -> API -> Show API token`.

Also required when using Technitium, an API token can be generated at `Administration -> Sessions -> Create Token`.

##### `hide-graph`
Whether to hide the graph showing the number of queries over time.

##### `hide-top-domains`
Whether to hide the list of top blocked domains.

##### `hour-format`
Whether to display the relative time in the graph in `12h` or `24h` format.

### Server Stats
Display statistics such as CPU usage, memory usage and disk usage of the server Glance is running on or other servers.

Example:

```yaml
- type: server-stats
  servers:
    - type: local
      name: Services
```

Preview:

![](images/server-stats-preview.gif)

> [!NOTE]
>
> This widget is currently under development, some features might not function as expected or may change.

To display data from a remote server you need to have the Glance Agent running on that server. You can download the agent from [here](https://github.com/glanceapp/agent), though keep in mind that it is still in development and may not work as expected. Support for other providers such as Glances will be added in the future.

In the event that the CPU temperature goes over 80°C, a flame icon will appear next to the CPU. The progress indicators will also turn red (or the equivalent of your negative color) to hopefully grab your attention if anything is unusually high:

![](images/server-stats-flame-icon.png)

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| servers | array | no |  |

##### `servers`
If not provided it will display the statistics of the server Glance is running on.

##### Properties for both `local` and `remote` servers
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| type | string | yes |  |
| name | string | no |  |
| hide-swap | boolean | no | false |

###### `type`
Whether to display statistics for the local server or a remote server. Possible values are `local` and `remote`.

###### `name`
The name of the server which will be displayed on the widget. If not provided it will default to the server's hostname.

###### `hide-swap`
Whether to hide the swap usage.

##### Additional properties for the `local` server
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| cpu-temp-sensor | string | no |  |

##### Mountpoint properties for both `local` and `remote` servers
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| hide-mountpoints-by-default | boolean | no | false |
| mountpoints | map\[string\]object | no |  |

###### `cpu-temp-sensor`
The name of the sensor to use for the CPU temperature. When not provided the widget will attempt to find the correct one, if it fails to do so the temperature will not be displayed. To view the available sensors you can use `sensors` command.

###### `hide-mountpoints-by-default`
If set to `true` you'll have to manually make each mountpoint visible by adding a `hide: false` property to it like so:

```yaml
- type: server-stats
  servers:
    - type: local
      hide-mountpoints-by-default: true
      mountpoints:
        "/":
          hide: false
        "/mnt/data":
          hide: false
```

This is useful if you're running Glance inside of a container which usually mounts a lot of irrelevant filesystems.

###### `mountpoints`
A map of mountpoints to display disk usage for. The key is the path to the mountpoint and the value is an object with optional properties. For remote servers, these settings filter and rename mountpoints reported by the remote agent; they cannot add paths that the agent did not report. Example:

```yaml
mountpoints:
  "/":
    name: Root
  "/mnt/data":
    name: Data
  "/boot/efi":
    hide: true
```

##### Properties for each `mountpoint`
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| name | string | no |  |
| hide | boolean | no | false |

###### `name`
The name of the mountpoint which will be displayed on the widget. If not provided it will default to the mountpoint's path.

###### `hide`
Whether to hide this mountpoint from the widget.

##### Properties for `remote` servers
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| url | string | yes |  |
| token | string | no |  |
| timeout | string | no | 3s |
| allow-insecure | boolean | no | false |

###### `url`
The URL and port of the server to fetch the statistics from.

###### `token`
The authentication token to use when fetching the statistics.

###### `timeout`
The maximum time to wait for a response from the server. The value is a string and must be a number followed by one of s, m, h, d. Example: `10s` for 10 seconds, `1m` for 1 minute, etc.

###### `allow-insecure`
Whether to allow invalid/self-signed certificates when fetching statistics from a remote server.

### Repository
Display general information about a repository as well as a list of the latest open pull requests and issues.

Example:

```yaml
- type: repository
  repository: glanceapp/glance
  pull-requests-limit: 5
  issues-limit: 3
  commits-limit: 3
```

Preview:

![](images/repository-preview.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| repository | string | yes |  |
| token | string | no | |
| pull-requests-limit | integer | no | 3 |
| issues-limit | integer | no | 3 |
| commits-limit | integer | no | -1 |

##### `repository`
The owner and repository name that will have their information displayed.

##### `token`
Without authentication Github allows for up to 60 requests per hour. You can easily exceed this limit and start seeing errors if your cache time is low or you have many instances of this widget. To circumvent this you can [create a read only token from your Github account](https://github.com/settings/personal-access-tokens/new) and provide it here.

##### `pull-requests-limit`
The maximum number of latest open pull requests to show. Set to `-1` to not show any.

##### `issues-limit`
The maximum number of latest open issues to show. Set to `-1` to not show any.

##### `commits-limit`
The maximum number of lastest commits to show from the default branch. Set to `-1` to not show any.

### Bookmarks
Display a list of links which can be grouped.

Example:

```yaml
- type: bookmarks
  groups:
    - links:
        - title: Gmail
          url: https://mail.google.com/mail/u/0/
        - title: Amazon
          url: https://www.amazon.com/
        - title: Github
          url: https://github.com/
        - title: Wikipedia
          url: https://en.wikipedia.org/
    - title: Entertainment
      color: 10 70 50
      links:
        - title: Netflix
          url: https://www.netflix.com/
        - title: Disney+
          url: https://www.disneyplus.com/
        - title: YouTube
          url: https://www.youtube.com/
        - title: Prime Video
          url: https://www.primevideo.com/
    - title: Social
      color: 200 50 50
      links:
        - title: Reddit
          url: https://www.reddit.com/
        - title: Twitter
          url: https://twitter.com/
        - title: Instagram
          url: https://www.instagram.com/
```

Preview:

![](images/bookmarks-widget-preview.png)


#### Properties

| Name | Type | Required |
| ---- | ---- | -------- |
| groups | array | yes |

##### `groups`
An array of groups which can optionally have a title and a custom color.

###### Properties for each group
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| title | string | no | |
| color | HSL | no | the primary color of the theme |
| links | array | yes | |
| same-tab | boolean | no | false |
| hide-arrow | boolean | no | false |
| target | string | no | |

> [!TIP]
>
> You can set `same-tab`, `hide-arrow` and `target` either on the group which will apply them to all links in that group, or on each individual link which will override the value set on the group.

###### Properties for each link
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| title | string | yes | |
| url | string | yes | |
| description | string | no | |
| icon | string | no | |
| same-tab | boolean | no | false |
| hide-arrow | boolean | no | false |
| target | string | no | |

`icon`

See [Icons](#icons) for more information on how to specify icons.

`same-tab`

Whether to open the link in the same tab or a new one.

`hide-arrow`

Whether to hide the colored arrow on each link.

`target`

Set a custom value for the link's `target` attribute. Possible values are `_blank`, `_self`, `_parent` and `_top`, you can read more about what they do [here](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/a#target). This property has precedence over `same-tab`.

### ChangeDetection.io
Display a list watches from changedetection.io.

Example

```yaml
- type: change-detection
  instance-url: https://changedetection.mydomain.com/
  token: ${CHANGE_DETECTION_TOKEN}
```

Preview:

![](images/change-detection-widget-preview.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| instance-url | string | no | `https://www.changedetection.io` |
| token | string | no |  |
| timeout | duration string | no | |
| allow-insecure | boolean | no | false |
| headers | key & value | no | |
| limit | integer | no | 10 |
| collapse-after | integer | no | 5 |
| watches | array of strings | no |  |

##### `instance-url`
The URL pointing to your instance of `changedetection.io`.

##### `token`
The API access token which can be found in `SETTINGS > API`. Optionally, you can specify this using an environment variable with the syntax `${VARIABLE_NAME}`.

##### `timeout`
The maximum time to wait for requests to the ChangeDetection.io instance.

##### `allow-insecure`
Whether to allow invalid/self-signed certificates when contacting the ChangeDetection.io instance.

##### `headers`
Optional HTTP headers sent with requests to the ChangeDetection.io instance. The dedicated `token` setting remains authoritative for its authentication header.

##### `limit`
The maximum number of watches to show.

##### `collapse-after`
How many watches are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse.

##### `watches`
By default all of the configured watches will be shown. Optionally, you can specify a list of UUIDs for the specific watches you want to have listed:

```yaml
  - type: change-detection
    watches:
      - 1abca041-6d4f-4554-aa19-809147f538d3
      - 705ed3e4-ea86-4d25-a064-822a6425be2c
```

### Clock
Display a clock showing the current time and date. Optionally, also display the the time in other timezones.

Example:

```yaml
- type: clock
  hour-format: 24h
  timezones:
    - timezone: Europe/Paris
      label: Paris
    - timezone: America/New_York
      label: New York
    - timezone: Asia/Tokyo
      label: Tokyo
```

Preview:

![](images/clock-widget-preview.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| hour-format | string | no | 24h |
| timezones | array | no |  |

##### `hour-format`
Whether to show the time in 12 or 24 hour format. Possible values are `12h` and `24h`.

#### Properties for each timezone

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| timezone | string | yes | |
| label | string | no | |

##### `timezone`
A timezone identifier such as `Europe/London`, `America/New_York`, etc. The full list of available identifiers can be found [here](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones).

##### `label`
Optionally, override the display value for the timezone to something more meaningful such as "Home", "Work" or anything else.

### Analog Clock
Display an analog clock showing the current time. Optionally, display the date, AM/PM indicator, numerical dial markers, and clocks for additional timezones.

Example:

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

Preview:

![](images/analog-clock-preview.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| hide-am-pm-indicator | boolean | no | false |
| hide-date | boolean | no | false |
| dial-markers | string | no | NumericalFull |
| timezones | array | no | |

##### `hide-am-pm-indicator`
Whether to hide the AM/PM indicator from the clock face.

##### `hide-date`
Whether to hide the date from the clock face.

##### `dial-markers`
Controls the numerical markers displayed around the clock face. Possible values are `NumericalFull`, `NumericalMinimal`, and `None`.

`NumericalFull` displays all twelve hour numbers, `NumericalMinimal` displays only 12, 3, 6, and 9, and `None` hides the numerical markers.

#### Properties for each timezone

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| timezone | string | yes | |
| label | string | no | |

##### `timezone`
A timezone identifier such as `Europe/London`, `America/New_York`, etc.

##### `label`
Optionally, override the display value for the timezone with a more meaningful label such as "Home" or "Work".

### Calendar
Display a calendar.

Example:

```yaml
- type: calendar
  first-day-of-week: monday
```

Preview:

![](images/calendar-widget-preview.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| first-day-of-week | string | no | monday |

##### `first-day-of-week`
The day of the week that the calendar starts on. All week days are available as possible values.

### ICS Events
Display upcoming events from one or more iCalendar (`.ics`) sources as a chronological agenda.

Sources can be remote HTTP/HTTPS URLs or local files. Events from all available sources are merged and sorted by start time. Recurring events, recurrence dates and exclusions, recurrence overrides, cancellations, all-day events, multi-day events, and ICS timezones are supported.

Example:

```yaml
- type: ics-events
  title: Upcoming Events
  days-ahead: 14
  limit: 25
  collapse-after: 5
  sources:
    - url: https://example.com/calendar.ics
      title: Family
    - file: /config/calendars/maintenance.ics
      title: Maintenance
```

Preview:

![](images/ics-events-preview.png)

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| sources | array | yes | |
| days-ahead | integer | no | 14 |
| limit | integer | no | 25 |
| collapse-after | integer | no | 5 |

##### `sources`
An array of iCalendar sources. Each source must specify exactly one of `url` or `file`.

Remote sources are fetched over HTTP or HTTPS and use conditional requests when the server provides `ETag` or `Last-Modified` headers. Local file paths refer to files accessible to the Glance process. When running Glance in a container, local calendar files must therefore be mounted into the container.

If one source fails while another succeeds, events from the successful sources remain available and the widget reports a degraded refresh. If all sources fail, previously fetched widget content is retained while the widget reports the refresh error.

Properties for each source:

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| url | string | conditional | |
| file | string | conditional | |
| title | string | no | |
| timeout | duration string | no | |
| allow-insecure | boolean | no | false |
| headers | key & value | no | |
| basic-auth | object | no | |

###### `url`
The HTTP or HTTPS URL of a remote iCalendar source. Specify either `url` or `file`, but not both.

###### `file`
The path to a local iCalendar file. Specify either `file` or `url`, but not both.

###### `title`
An optional source label shown with events from this source.

###### `timeout`
For remote URL sources, the maximum time to wait for the HTTP request.

###### `allow-insecure`
For remote URL sources, whether to allow invalid/self-signed certificates.

###### `headers`
Optional HTTP headers sent when fetching a remote URL source.

###### `basic-auth`
Optional HTTP Basic Authentication credentials for a remote URL source. These HTTP properties do not affect local file sources.

##### `days-ahead`
The number of days ahead to include when loading events. Recurring events are expanded only within this bounded time window.

##### `limit`
The maximum number of events shown after all sources have been merged and sorted.

##### `collapse-after`
The number of events to show before the remaining events are collapsed. Set to `-1` to disable collapsing.

### Calendar (legacy)
Display a calendar.

Example:

```yaml
- type: calendar-legacy
  start-sunday: false
```

Preview:

![](images/calendar-legacy-widget-preview.png)

> [!NOTE]
>
> This widget is deprecated and may be removed in a future version.

#### Properties

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| start-sunday | boolean | no | false |

##### `start-sunday`
Whether calendar weeks start on Sunday or Monday.

> [!NOTE]
>
> There is currently little customizability available for the calendar. Extra features will be added in the future.

### Markets
Display a list of markets, their current value, change for the day and a small 21d chart. Data is taken from Yahoo Finance.

Example:

```yaml
- type: markets
  markets:
    - symbol: SPY
      name: S&P 500
    - symbol: BTC-USD
      name: Bitcoin
      chart-link: https://www.tradingview.com/chart/?symbol=INDEX:BTCUSD
    - symbol: NVDA
      name: NVIDIA
    - symbol: AAPL
      symbol-link: https://www.google.com/search?tbm=nws&q=apple
      name: Apple
```

Preview:

![](images/markets-widget-preview.png)

#### Properties

| Name | Type | Required |
| ---- | ---- | -------- |
| markets | array | yes |
| sort-by | string | no |
| chart-link-template | string | no |
| symbol-link-template | string | no |

##### `markets`
An array of markets for which to display information about.

##### `sort-by`
By default the markets are displayed in the order they were defined. You can customize their ordering by setting the `sort-by` property to `change` for descending order based on the stock's percentage change (e.g. 1% would be sorted higher than -1%) or `absolute-change` for descending order based on the stock's absolute price change (e.g. -1% would be sorted higher than +0.5%).

##### `chart-link-template`
A template for the link to go to when clicking on the chart that will be applied to all markets. The value `{SYMBOL}` will be replaced with the symbol of the market. You can override this on a per-market basis by specifying a `chart-link` property. Example:

```yaml
chart-link-template: https://www.tradingview.com/chart/?symbol={SYMBOL}
```

##### `symbol-link-template`
A template for the link to go to when clicking on the symbol that will be applied to all markets. The value `{SYMBOL}` will be replaced with the symbol of the market. You can override this on a per-market basis by specifying a `symbol-link` property. Example:

```yaml
symbol-link-template: https://www.google.com/search?tbm=nws&q={SYMBOL}
```

###### Properties for each market
| Name | Type | Required |
| ---- | ---- | -------- |
| symbol | string | yes |
| name | string | no |
| symbol-link | string | no |
| chart-link | string | no |

`symbol`

The symbol, as seen in Yahoo Finance.

`name`

The name that will be displayed under the symbol.

`symbol-link`

The link to go to when clicking on the symbol.

`chart-link`

The link to go to when clicking on the chart.

### Twitch Channels
Display a list of channels from Twitch.

Example:

```yaml
- type: twitch-channels
  channels:
    - jembawls
    - giantwaffle
    - asmongold
    - cohhcarnage
    - j_blow
    - xQc
```

Preview:

![](images/twitch-channels-widget-preview.png)

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| channels | array | yes | |
| collapse-after | integer | no | 5 |
| sort-by | string | no | viewers |

##### `channels`
A list of channels to display.

##### `collapse-after`
How many channels are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse.

##### `sort-by`
Can be used to specify the order in which the channels are displayed. Possible values are `viewers` and `live`.

### Twitch top games
Display a list of games with the most viewers on Twitch.

Example:

```yaml
- type: twitch-top-games
  exclude:
    - just-chatting
    - pools-hot-tubs-and-beaches
    - music
    - art
    - asmr
```

Preview:

![](images/twitch-top-games-widget-preview.png)

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| exclude | array | no | |
| limit | integer | no | 10 |
| collapse-after | integer | no | 5 |

##### `exclude`
A list of categories that will never be shown. You must provide the slug found by clicking on the category and looking at the URL:

```
https://www.twitch.tv/directory/category/grand-theft-auto-v
                                         ^^^^^^^^^^^^^^^^^^
```

##### `limit`
The maximum number of games to show.

##### `collapse-after`
How many games are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse.

### iframe
Embed an iframe as a widget.

Example:

```yaml
- type: iframe
  source: <url>
  height: 400
```

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| source | string | yes | |
| height | integer | no | 300 |

##### `source`
The source of the iframe.

##### `height`
The height of the iframe. The minimum allowed height is 50.

### Markdown
Render Markdown directly in a widget. Markdown can be provided inline or read from a file on the Glance filesystem.

Inline example:

```yaml
- type: markdown
  title: Notes
  source: |
    ## Hello

    This is **Markdown**.
```

File example:

```yaml
- type: markdown
  title: Notes
  file: /config/notes.md
  cache: 5m
```

Preview:

![](images/markdown-widget-preview.png)

#### Properties
| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| source | string | yes* | |
| file | string | yes* | |
| cache | duration | no | 5m for file sources |

\* Exactly one of `source` or `file` must be specified.

##### `source`
Inline Markdown content. Inline Markdown is rendered when the configuration is loaded and does not require periodic refreshes.

##### `file`
Path to a Markdown file accessible from inside the Glance container. File-backed Markdown is refreshed every 5 minutes by default. Use the standard `cache` property to change the refresh interval.

If a file cannot be read during a refresh, the last successfully rendered content remains visible and the widget reports the refresh error through the normal Glance widget status.

Markdown uses GitHub Flavored Markdown features including tables, strikethrough, autolinks and task lists. Raw HTML embedded in Markdown is not rendered, and potentially dangerous link destinations are not emitted as executable links. Use the [HTML](#html) widget when trusted arbitrary HTML is intentionally required.

### HTML
Embed any HTML.

Example:

```yaml
- type: html
  source: |
    <p>Hello, <span class="color-primary">World</span>!</p>
```

Note the use of `|` after `source:`, this allows you to insert a multi-line string.
