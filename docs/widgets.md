# Widgets

> Browse the widgets available in Glance and jump directly to configuration, examples, and behavior for each one.

[Glance README](../README.md) · [Configuration](configuration.md) · [Shared properties](#shared-properties)

## Browse by category

[Layout & composition](#layout--composition) · [Feeds & content](#feeds--content) · [Search & custom content](#search-links--custom-content) · [Date, time & weather](#date-time--weather) · [Homelab & monitoring](#homelab--monitoring) · [Development & releases](#development--releases) · [Markets & streaming](#markets--streaming) · [Utilities](#utilities)

## Shared properties

Every widget supports a common set of configuration properties in addition to its widget-specific options. Individual widget pages link back to this shared reference instead of duplicating it.

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `type` | string | yes | — |
| `title` | string | no | widget-defined |
| `title-url` | string | no | widget-defined |
| `hide-header` | boolean | no | `false` |
| `cache` | duration | no | widget-defined |
| `css-class` | string | no | — |

### `type`
Used to specify the widget.

### `title`
The title of the widget. If left blank it will be defined by the widget.

### `title-url`
The URL to go to when clicking on the widget's title. If left blank it will be defined by the widget (if available).

### `hide-header`
When set to `true`, the header (title) of the widget will be hidden. You cannot hide the header of the group widget.

> [!NOTE]
>
> If a widget fails to update, a red dot or circle is shown next to the title of that widget indicating that it is not working. You will not be able to see this if you hide the header.

### `cache`
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

### `css-class`
Set custom CSS classes for the specific widget instance.


## Widget reference

### Layout & composition

| Widget | Purpose |
| --- | --- |
| [Group](widgets/group.md) | Combine multiple widgets into a group. |
| [Stack](widgets/stack.md) | Stack widgets within the same dashboard space. |
| [Status Bar](widgets/status-bar.md) | Display compact information in a scrolling or wrapped status bar. |
| [Split Column](widgets/split-column.md) | Divide dashboard space into additional column layouts. |

### Feeds & content

| Widget | Purpose |
| --- | --- |
| [RSS](widgets/rss.md) | Display RSS and Atom feeds. |
| [Videos](widgets/videos.md) | Display videos from configured channels and playlists. |
| [Hacker News](widgets/hacker-news.md) | Display Hacker News stories. |
| [Lobsters](widgets/lobsters.md) | Display stories from Lobsters. |
| [Reddit](widgets/reddit.md) | Display posts from Reddit. |

### Search, links & custom content

| Widget | Purpose |
| --- | --- |
| [Search](widgets/search.md) | Search the web and configured shortcuts. |
| [Bookmarks](widgets/bookmarks.md) | Organize frequently used links. |
| [Custom API](widgets/custom-api.md) | Fetch data from HTTP APIs and render custom content with templates, native presentation components, tables, and charts. |
| [Extension](widgets/extension.md) | Display content provided by Glance extensions. See also the [Extensions guide](extensions.md). |
| [iframe](widgets/iframe.md) | Embed another web page. |
| [Markdown](widgets/markdown.md) | Render Markdown content. |
| [HTML](widgets/html.md) | Render trusted HTML content. |

### Date, time & weather

| Widget | Purpose |
| --- | --- |
| [Weather](widgets/weather.md) | Display current weather, details, hourly conditions, and forecast information. |
| [Calendar](widgets/calendar.md) | Display calendar events from configured sources. |
| [ICS Events](widgets/ics-events.md) | Display upcoming events from ICS sources. |
| [Calendar (legacy)](widgets/calendar-legacy.md) | Display the legacy calendar widget. |
| [Clock](widgets/clock.md) | Display clocks for configured time zones. |
| [Analog Clock](widgets/analog-clock.md) | Display time using an analog clock. |
| [Timer](widgets/timer.md) | Provide an interactive timer. |

### Homelab & monitoring

| Widget | Purpose |
| --- | --- |
| [Monitor](widgets/monitor.md) | Monitor configured services and endpoints. |
| [Docker Containers](widgets/docker-containers.md) | Display Docker container status. |
| [DNS Stats](widgets/dns-stats.md) | Display DNS service statistics. |
| [Server Stats](widgets/server-stats.md) | Display local and remote server statistics. |
| [ChangeDetection.io](widgets/change-detection.md) | Display ChangeDetection.io watches. |

### Development & releases

| Widget | Purpose |
| --- | --- |
| [Repository](widgets/repository.md) | Display repository activity and statistics. |
| [Releases](widgets/releases.md) | Display releases from configured repositories. |

### Markets & streaming

| Widget | Purpose |
| --- | --- |
| [Markets](widgets/markets.md) | Display market prices and changes. |
| [Twitch Channels](widgets/twitch-channels.md) | Display configured Twitch channels. |
| [Twitch Top Games](widgets/twitch-top-games.md) | Display top games on Twitch. |

### Utilities

| Widget | Purpose |
| --- | --- |
| [Todo](widgets/todo.md) | Maintain a browser-local to-do list. |
| [Unit Converter](widgets/unit-converter.md) | Convert between common units. |
| [Calculator](widgets/calculator.md) | Perform calculations directly in Glance. |

---

[Glance README](../README.md) · [Configuration](configuration.md) · [Back to top](#widgets)
