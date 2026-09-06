# Search Widget

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Search the web from Glance using a built-in or custom search engine, with optional bangs, direct domain navigation, and keyboard shortcuts.

## Quick start

```yaml
- type: search
  search-engine: duckduckgo
  bangs:
    - title: YouTube
      shortcut: "!yt"
      url: https://www.youtube.com/results?search_query={QUERY}
```

## Preview

![Search widget with a configured YouTube bang](../images/search-widget-preview.png)

## Keyboard shortcuts
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

## Configuration
| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `search-engine` | string | no | `duckduckgo` |
| `new-tab` | boolean | no | `false` |
| `autofocus` | boolean | no | `false` |
| `target` | string | no | `_blank` |
| `placeholder` | string | no | `Type here to search…` |
| `open-domains` | boolean | no | `false` |
| `bangs` | array | no | — |

All widgets also support the [shared widget properties](../widgets.md#shared-properties).

### `search-engine`
Either a value from the table below or a URL to a custom search engine. Use `{QUERY}` to indicate where the query value gets placed.

| Name | URL |
| ---- | --- |
| duckduckgo | `https://duckduckgo.com/?q={QUERY}` |
| google | `https://www.google.com/search?q={QUERY}` |
| bing | `https://www.bing.com/search?q={QUERY}` |
| perplexity | `https://www.perplexity.ai/search?q={QUERY}` |
| kagi | `https://kagi.com/search?q={QUERY}` |
| startpage | `https://www.startpage.com/search?q={QUERY}` |

### `new-tab`
When set to `true`, swaps the shortcuts for showing results in the same or new tab, defaulting to showing results in a new tab.

### `autofocus`
When set to `true`, automatically focuses the search input on page load.

### `target`
The target to use when opening the search results in a new tab. Possible values are `_blank`, `_self`, `_parent` and `_top`.

### `placeholder`
When set, modifies the text displayed in the input field before typing.

### `open-domains`
When set to `true`, an input that looks like a domain or HTTP(S) URL is opened directly instead of being searched for. `example.com`, `sub.example.com:8080/path`, and `https://example.com` all navigate directly; `https://` is added when no scheme is present. The input must contain no spaces and bare hostnames such as `localhost` are still searched for. Bang searches always take precedence, and non-HTTP schemes are treated as normal search input.

### `bangs`
Bangs are shortcuts that route a query to a specific search engine or site. For example, a configured `!yt` bang can send the query directly to YouTube:

![Search widget with an active bang shortcut](../images/search-widget-bangs-preview.png)

#### Bang properties
| Property | Type | Required |
| --- | --- | --- |
| `title` | string | no |
| `shortcut` | string | yes |
| `url` | string | yes |

#### `title`
Optional title that will appear on the right side of the search bar when the query starts with the associated shortcut.

#### `shortcut`
Any value you wish to use as the shortcut for the search engine. It does not have to start with `!`.

> [!IMPORTANT]
>
> In YAML some characters have special meaning when placed in the beginning of a value. If your shortcut starts with `!` (and potentially some other special characters) you'll have to wrap the value in quotes:
> ```yaml
> shortcut: "!yt"
>```

#### `url`
The URL of the search engine. Use `{QUERY}` to indicate where the query value gets placed. Examples:

```yaml
url: https://www.reddit.com/search?q={QUERY}
url: https://store.steampowered.com/search/?term={QUERY}
url: https://www.amazon.com/s?k={QUERY}
```


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#search-widget)
