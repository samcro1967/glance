package glance

import (
	"fmt"
	"html/template"
	"strings"
)

const defaultMicroMarketSymbolLinkTemplate = "https://finance.yahoo.com/quote/{SYMBOL}"

var microMarketsTemplate = mustParseTemplate("footer-micro-markets.html")

type microMarkets struct {
	marketsWidget `yaml:",inline"`
	Position      int  `yaml:"position"`
	SameTab       bool `yaml:"same-tab"`
}

func (m *microMarkets) GetPosition() int { return m.Position }
func (m *microMarkets) initialize() error {
	if len(m.MarketRequests) == 0 {
		m.MarketRequests = m.StocksRequests
	}
	if len(m.MarketRequests) == 0 {
		return fmt.Errorf("at least one market is required")
	}
	if m.SymbolLinkTemplate == "" {
		m.SymbolLinkTemplate = defaultMicroMarketSymbolLinkTemplate
	}
	for i := range m.MarketRequests {
		if m.MarketRequests[i].Symbol == "" {
			return fmt.Errorf("market symbol is required")
		}
		if m.MarketRequests[i].SymbolLink == "" {
			m.MarketRequests[i].SymbolLink = strings.ReplaceAll(m.SymbolLinkTemplate, "{SYMBOL}", m.MarketRequests[i].Symbol)
		}
	}
	return m.marketsWidget.initialize()
}
func (m *microMarkets) Render() template.HTML { return m.renderTemplate(m, microMarketsTemplate) }
func (m *microMarkets) MicroItems(open bool) []statusBarCompactItem {
	return marketsStatusBarCompactItems(&m.marketsWidget, open)
}
func marketsStatusBarCompactItems(m *marketsWidget, open bool) []statusBarCompactItem {
	if len(m.Markets) == 0 && m.Error != nil {
		return []statusBarCompactItem{{Kind: "error", Error: m.Error, ErrorTitle: m.Title}}
	}
	items := make([]statusBarCompactItem, 0, len(m.Markets))
	for _, market := range m.Markets {
		items = append(items, statusBarCompactItem{Kind: "market", Error: m.Error, Notice: m.Notice, URL: market.SymbolLink, OpenLinksInNewTab: open, MarketSymbol: market.Symbol, MarketName: market.Name, MarketChartURL: market.ChartLink, MarketCurrency: market.Currency, MarketCurrencySymbol: market.CurrencySymbol, MarketPrice: market.Price, MarketPriceHint: market.PriceHint, MarketPercentChange: market.PercentChange})
	}
	return items
}
