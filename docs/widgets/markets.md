# Markets

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display a list of markets, their current value, change for the day and a small 21d chart. Data is taken from Yahoo Finance.

Example:
## Quick start


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

![Markets widget](../images/widgets/markets.png)

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Name | Type | Required |
| ---- | ---- | -------- |
| markets | array | yes |
| sort-by | string | no |
| chart-link-template | string | no |
| symbol-link-template | string | no |

### `markets`
An array of markets for which to display information about.

### `sort-by`
By default the markets are displayed in the order they were defined. You can customize their ordering by setting the `sort-by` property to `change` for descending order based on the stock's percentage change (e.g. 1% would be sorted higher than -1%) or `absolute-change` for descending order based on the stock's absolute price change (e.g. -1% would be sorted higher than +0.5%).

### `chart-link-template`
A template for the link to go to when clicking on the chart that will be applied to all markets. The value `{SYMBOL}` will be replaced with the symbol of the market. You can override this on a per-market basis by specifying a `chart-link` property. Example:

```yaml
chart-link-template: https://www.tradingview.com/chart/?symbol={SYMBOL}
```

### `symbol-link-template`
A template for the link to go to when clicking on the symbol that will be applied to all markets. The value `{SYMBOL}` will be replaced with the symbol of the market. You can override this on a per-market basis by specifying a `symbol-link` property. Example:

```yaml
symbol-link-template: https://www.google.com/search?tbm=nws&q={SYMBOL}
```

#### Properties for each market
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


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#markets)
