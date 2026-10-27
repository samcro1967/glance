# HTTP request timeouts

[Configuration](configuration.md) · [Widgets](widgets.md)

Widgets that perform server-side HTTP fetches support a bounded request timeout. This is a selected-widget capability, not a property of every widget. Calculator, Unit Converter, Astrology, and other local-only widgets do not need it. Browser-loaded iframes and external images are not governed by a server fetch timeout.

## Default and overrides

The centralized HTTP default is **5 seconds**. `timeout` accepts a non-negative integer followed by `s`, `m`, `h`, or `d`. An explicit `0s` selects the centralized bounded default; it never disables request deadlines. Invalid duration text is rejected.

Resolution proceeds from the built-in default to `widget-defaults.global.timeout`, then `widget-defaults.types.<type>.timeout`, then an explicit supported widget or source setting. Explicit source overrides win over inherited defaults. Layout containers do not pass their own type defaults to child widgets; each child resolves its own widget type.

```yaml
widget-defaults:
  global:
    timeout: 5s
  types:
    horoscope:
      timeout: 10s

pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: horoscope
            signs: [cancer]
            periods: [weekly]
            timeout: 15s
```

The weekly instance above allows 15 seconds per provider request. Another Horoscope instance without a local override inherits 10 seconds. Other supported types inherit 5 seconds.

The limit includes connection establishment, TLS, receiving response headers, and reading the response body. It applies separately to each HTTP request, not to the entire refresh, a batch, or a chain of fallback requests. Parent cancellation can end a non-shared request sooner. Shared cached requests retain their existing cancellation isolation: a caller can stop waiting without canceling work still useful to another caller. Different effective timeout policies use separate shared-cache identities; equal policies can still coalesce requests.

## Supported scopes

| Widget | Local timeout scope | Built-in HTTP default |
| --- | --- | --- |
| [animal-of-the-day](widgets/animal-of-the-day.md) | Widget instance | 5s |
| [arr](widgets/arr.md) | Widget instance | 5s |
| [astronomy](widgets/astronomy.md) | Widget instance; location lookup only | 5s |
| [change-detection](widgets/change-detection.md) | Widget instance | 5s |
| [custom-api](widgets/custom-api.md) | Primary request and individually configured subrequests | 5s |
| [dilbert](widgets/dilbert.md) | Widget instance | 15s |
| [dns-stats](widgets/dns-stats.md) | Widget instance | 5s |
| [docker-containers](widgets/docker-containers.md) | Widget instance | 5s |
| [environment](widgets/environment.md) | Widget instance | 5s |
| [extension](widgets/extension.md) | Widget instance | 5s |
| [hacker-news](widgets/hacker-news.md) | Widget instance | 5s |
| [horoscope](widgets/horoscope.md) | Widget instance | 5s |
| [latest-media](widgets/latest-media.md) | Widget instance | 5s |
| [lobsters](widgets/lobsters.md) | Widget instance | 5s |
| [markets](widgets/markets.md) | Widget instance | 5s |
| [media-history](widgets/media-history.md) | Widget instance | 5s |
| [nasa-apod](widgets/nasa-apod.md) | Widget instance | 5s |
| [now-playing](widgets/now-playing.md) | Widget instance | 5s |
| [on-this-day](widgets/on-this-day.md) | Widget instance | 5s |
| [prometheus](widgets/prometheus.md) | Widget instance | 5s |
| [quote-of-the-day](widgets/quote-of-the-day.md) | Widget instance | 5s |
| [reddit](widgets/reddit.md) | Widget HTTP requests and CDP metadata; see browser exception below | 5s |
| [releases](widgets/releases.md) | Widget instance | 5s |
| [repository](widgets/repository.md) | Widget instance | 5s |
| [search](widgets/search.md) | Widget instance; remote suggestions only | 5s |
| [seerr](widgets/seerr.md) | Widget instance | 5s |
| [torrenting](widgets/torrenting.md) | Widget instance | 5s |
| [trivia](widgets/trivia.md) | Widget instance | 5s |
| [twitch-channels](widgets/twitch-channels.md) | Widget instance | 5s |
| [twitch-top-games](widgets/twitch-top-games.md) | Widget instance | 5s |
| [videos](widgets/videos.md) | Widget instance | 5s |
| [weather](widgets/weather.md) | Widget instance | 5s |
| [word-of-the-day](widgets/word-of-the-day.md) | Widget instance | 5s |
| `stocks` | Alias of [Markets](widgets/markets.md), with its own type-default entry | 5s |
| [calendar](widgets/calendar.md) | HTTP entries in `sources`; global/type defaults flow to these entries | 5s |
| [ics-events](widgets/ics-events.md) | HTTP entries in `sources`; global/type defaults flow to these entries | 5s |
| [rss](widgets/rss.md) | entries in `feeds`; global/type defaults flow to these entries | 5s |
| [monitor](widgets/monitor.md) | entries in `sites`; global/type defaults flow to these entries | 5s |
| [server-stats](widgets/server-stats.md) | remote entries in `servers`; global/type defaults flow to these entries | 5s |

For Calendar, ICS Events, RSS, Monitor, and Server Stats, configure individual source/site/server entries or use global/type defaults. A top-level instance `timeout` is not their supported scope.

Custom API primary and subrequest timeouts are independent. Global/type defaults apply to each request lacking its own explicit value. A primary request override does not replace the type default for sibling subrequests. Explicit subrequest `0s` selects the centralized 5s baseline.

## Exceptions and compatibility

- **Dilbert:** the built-in archive-request default remains 15s. Global, type, or instance settings can override it. An explicit instance `0s` selects the central 5s baseline. Archive retries remain separate requests.
- **Reddit:** `timeout` controls direct HTTP, OAuth token and guest-challenge HTTP requests, and CDP metadata discovery (including endpoint DNS). Existing specialized TLS and proxy transports are preserved. A positive `proxy.timeout` overrides the widget HTTP budget on proxy requests; otherwise the widget budget applies. Browser attachment, navigation, login, authenticated-session reuse, and listing-stage waits retain their existing behavior and limits. A small global HTTP timeout does not impose that deadline on browser login or listing extraction.
- **Search:** the setting affects remote suggestions, retaining origin validation, redirect restrictions, and response-size limits. Local search is unaffected.
- **Astronomy:** a named `location` requires HTTP geocoding; explicit coordinates and timezone are calculated locally.
- **Monitor and remote Server Stats:** the built-in timeout changes from 3s to the central 5s. Longer configured limits no longer encounter a hidden five-second HTTP-client cap.
- **Resource proxy:** the shared image proxy keeps its independent central 5s HTTP limit and security policy. Widget refresh timeout settings do not change browser image loads or proxy-service settings. Server authentication/release-status/diagnostic HTTP requests are also separate from widget-default inheritance.

Timeout changes do not alter cache freshness schedules, authentication credentials, TLS validation, response-size restrictions, retry classification, or stale/partial-content handling. A longer deadline cannot guarantee provider availability or current content.
