# Split Column

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Arrange child widgets across multiple equal-width columns. Split Column automatically collapses to fewer columns when the available width is insufficient, including a single-column layout on smaller screens.

## Quick start

Two widgets side by side in a `full` column:

![Two widgets displayed side by side with Split Column](../images/split-column-widget-preview.png)

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

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Property | Type | Required | Default |
| --- | --- | --- | --- |
| `max-columns` | integer | no | `2` |
| `widgets` | array | yes | — |

### `max-columns`

Maximum number of columns used by the layout. The minimum effective value is `2`; values below `2` are normalized to `2`.

### `widgets`

Child widgets arranged across the available columns. Split Column can contain normal widgets as well as Group widgets.

A Split Column cannot be placed inside a Group.

## Layout examples

### Three columns

Three equal-width columns:

![Three equal-width widgets in a Split Column layout](../images/split-column-widget-3-columns.png)

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

### Four columns

Four equal-width columns on a page configured with `width: wide`:

![Four equal-width widgets on a wide page using Split Column](../images/split-column-widget-4-columns.png)

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

### Masonry layout

A masonry layout with up to five equal-width columns on a page configured with `width: wide`:

![Masonry dashboard layout using up to five Split Column columns](../images/split-column-widget-masonry.png)

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


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#split-column)
