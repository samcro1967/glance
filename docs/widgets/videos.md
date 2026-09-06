# Videos

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display the latest videos from YouTube channels or playlists in card, list, or grid layouts.

## Quick start

```yaml
- type: videos
  channels:
    - UCXuqSBlHAE6Xw-yeJA0Tunw
    - UCBJycsmduvYEL83R_U4JriQ
```

![Videos widget using the default horizontal-card layout](../images/videos-widget-preview.png)

## Configuration

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `channels` | array | conditional | — |
| `playlists` | array | conditional | — |
| `limit` | integer | no | `25` |
| `style` | string | no | `horizontal-cards` |
| `collapse-after` | integer | no | `7` |
| `collapse-after-rows` | integer | no | `4` |
| `include-shorts` | boolean | no | `false` |
| `video-url-template` | string | no | YouTube video URL |

At least one channel or playlist must be configured. All widgets also support the [shared widget properties](../widgets.md#shared-properties).

### `channels`

A list of YouTube channel IDs.

Open a channel description to locate the channel sharing controls:

![YouTube channel description showing the channel information panel](../images/videos-channel-description-example.png)

Choose **Share channel**, then **Copy channel ID**:

![YouTube Share channel menu with the Copy channel ID option](../images/videos-copy-channel-id-example.png)

### `playlists`

A list of YouTube playlist IDs. Channels and playlists can be used together.

```yaml
- type: videos
  playlists:
    - PL8mG-RkN2uTyZZ00ObwZxxoG_nJbs3qec
    - PL8mG-RkN2uTxTK4m_Vl2dYR9yE41kRdBg
```

The playlist ID is the value of the `list` parameter in a YouTube playlist URL.

### `limit`

Sets the maximum number of videos displayed after results from all configured channels and playlists are combined and sorted.

### `style`

Supported values are `horizontal-cards`, `vertical-list`, and `grid-cards`.

#### Vertical list

![Videos widget using the vertical-list style](../images/videos-widget-vertical-list-preview.png)

#### Grid cards

![Videos widget using the grid-cards style](../images/videos-widget-grid-cards-preview.png)

### `collapse-after`

Sets the number of videos visible before **SHOW MORE** when using `vertical-list`. Set it to `-1` to disable collapsing.

### `collapse-after-rows`

Sets the number of visible rows before **SHOW MORE** when using `grid-cards`. Set it to `-1` to disable collapsing.

### `include-shorts`

Controls whether YouTube Shorts are included for channel sources. The default is `false`, so Glance attempts to use the channel feed containing regular uploads. If that feed cannot be resolved, Glance falls back to the standard uploads feed.

### `video-url-template`

Overrides the destination URL for videos, which is useful when using an alternative YouTube front end.

```yaml
video-url-template: https://invidious.your-domain.com/watch?v={VIDEO-ID}
```

The `{VIDEO-ID}` placeholder is replaced with the YouTube video ID.

---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#videos)
