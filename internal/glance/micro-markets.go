package glance

import (
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"
)

type microMarkets struct {
	widgetBase         `yaml:",inline"`
	Position           int             `yaml:"position"`
	MarketsRequests    []marketRequest `yaml:"markets"`
	StocksRequests     []marketRequest `yaml:"stocks"`
	Sort               string          `yaml:"sort-by"`
	ChartLinkTemplate  string          `yaml:"chart-link-template"`
	SymbolLinkTemplate string          `yaml:"symbol-link-template"`
	Markets            marketList      `yaml:"-"`
}

func (m *microMarkets) GetPosition() int {
	return m.Position
}

func (m *microMarkets) initialize() error {
	m.withCacheDuration(time.Hour)

	if len(m.MarketsRequests) == 0 {
		m.MarketsRequests = m.StocksRequests
	}

	if len(m.MarketsRequests) == 0 {
		return fmt.Errorf("at least one market is required")
	}

	for i := range m.MarketsRequests {
		request := &m.MarketsRequests[i]

		if request.Symbol == "" {
			return fmt.Errorf("market symbol is required")
		}

		if request.ChartLink == "" && m.ChartLinkTemplate != "" {
			request.ChartLink = strings.ReplaceAll(m.ChartLinkTemplate, "{SYMBOL}", request.Symbol)
		}

		if request.SymbolLink == "" && m.SymbolLinkTemplate != "" {
			request.SymbolLink = strings.ReplaceAll(m.SymbolLinkTemplate, "{SYMBOL}", request.Symbol)
		}
	}

	return nil
}

func (m *microMarkets) update(ctx context.Context) {
	markets, err := fetchMarketsDataFromYahoo(ctx, m.MarketsRequests)
	if !m.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if m.Sort == "absolute-change" {
		markets.sortByAbsChange()
	} else if m.Sort == "change" {
		markets.sortByChange()
	}

	m.Markets = markets
}

var microMarketsTemplate = mustParseTemplate("footer-micro-markets.html")

func (m *microMarkets) Render() template.HTML {
	return m.renderTemplate(m, microMarketsTemplate)
}
