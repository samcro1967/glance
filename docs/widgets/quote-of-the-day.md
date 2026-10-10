# Quote of the Day

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display Wikiquote’s curated Quote of the Day. The widget fetches the official daily Wikiquote page through the MediaWiki API and extracts the quote and author as safe structured text rather than rendering provider HTML.

## Quick start

```yaml
- type: quote-of-the-day
```

Preview:

![Quote of the Day widget](../images/widgets/quote-of-the-day.png)

**Mobile preview:**

![Quote Of The Day mobile preview](../images/widgets/mobile/quote-of-the-day.png)

## Configuration

This widget supports the [shared widget properties](../widgets.md#shared-properties). It requires no API key. By default it refreshes shortly after midnight using `cache-cron: 5 0 * * *`; normal cache overrides remain available. Later provider failures preserve previously successful content through Glance stale/degraded behavior.

---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#quote-of-the-day)
