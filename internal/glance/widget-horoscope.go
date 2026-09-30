package glance

import (
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"
)

var horoscopeWidgetTemplate = mustParseTemplate("horoscope.html", "widget-base.html")

var horoscopeSigns = map[string]string{
	"aries":       "♈",
	"taurus":      "♉",
	"gemini":      "♊",
	"cancer":      "♋",
	"leo":         "♌",
	"virgo":       "♍",
	"libra":       "♎",
	"scorpio":     "♏",
	"sagittarius": "♐",
	"capricorn":   "♑",
	"aquarius":    "♒",
	"pisces":      "♓",
}

var horoscopePeriods = map[string]struct{}{
	"daily":   {},
	"weekly":  {},
	"monthly": {},
}

type horoscopeWidget struct {
	widgetBase `yaml:",inline"`
	Provider   string                 `yaml:"provider"`
	Signs      []string               `yaml:"signs"`
	Periods    []string               `yaml:"periods"`
	Readings   []horoscopePeriodGroup `yaml:"-"`
	provider   horoscopeProvider
}

type horoscopePeriodGroup struct {
	Period   string
	Readings []horoscopeDisplayReading
}

type horoscopeDisplayReading struct {
	Date      string
	Sign      string
	Glyph     string
	Horoscope string
}

func (widget *horoscopeWidget) initialize() error {
	widget.withTitle("Horoscope")

	if widget.Provider == "" {
		widget.Provider = defaultHoroscopeProvider
	}

	provider, err := newHoroscopeProvider(widget.Provider)
	if err != nil {
		return err
	}
	widget.provider = provider

	if len(widget.Signs) == 0 {
		return fmt.Errorf("at least one horoscope sign is required")
	}

	seenSigns := make(map[string]struct{}, len(widget.Signs))
	normalizedSigns := make([]string, 0, len(widget.Signs))
	for _, configuredSign := range widget.Signs {
		sign := strings.ToLower(strings.TrimSpace(configuredSign))
		if _, ok := horoscopeSigns[sign]; !ok {
			return fmt.Errorf("unknown horoscope sign %q", configuredSign)
		}
		if _, duplicate := seenSigns[sign]; duplicate {
			continue
		}
		seenSigns[sign] = struct{}{}
		normalizedSigns = append(normalizedSigns, sign)
	}
	widget.Signs = normalizedSigns

	if len(widget.Periods) == 0 {
		widget.Periods = []string{"daily"}
	}

	seenPeriods := make(map[string]struct{}, len(widget.Periods))
	normalizedPeriods := make([]string, 0, len(widget.Periods))
	for _, configuredPeriod := range widget.Periods {
		period := strings.ToLower(strings.TrimSpace(configuredPeriod))
		if _, ok := horoscopePeriods[period]; !ok {
			return fmt.Errorf("unknown horoscope period %q; valid periods are daily, weekly, monthly", configuredPeriod)
		}
		if _, duplicate := seenPeriods[period]; duplicate {
			continue
		}
		seenPeriods[period] = struct{}{}
		normalizedPeriods = append(normalizedPeriods, period)
	}
	widget.Periods = normalizedPeriods

	if widget.CustomCacheDuration != 0 || widget.CustomCacheCron != "" {
		widget.withCacheDuration(24 * time.Hour)
		return nil
	}

	// Horoscope content is calendar-based. Refresh shortly after midnight so
	// daily readings roll over predictably without assuming undocumented
	// provider-specific weekly or monthly publication boundaries.
	return widget.withCacheCron("5 0 * * *")
}

func (widget *horoscopeWidget) update(ctx context.Context) {
	groups := make([]horoscopePeriodGroup, 0, len(widget.Periods))

	for _, period := range widget.Periods {
		group := horoscopePeriodGroup{
			Period:   period,
			Readings: make([]horoscopeDisplayReading, 0, len(widget.Signs)),
		}

		for _, sign := range widget.Signs {
			reading, err := widget.provider.fetch(ctx, period, sign)
			if err != nil {
				widget.canContinueUpdateAfterHandlingErr(err)
				return
			}

			group.Readings = append(group.Readings, horoscopeDisplayReading{
				Date:      reading.Date,
				Sign:      reading.Sign,
				Glyph:     horoscopeSigns[sign],
				Horoscope: reading.Horoscope,
			})
		}

		groups = append(groups, group)
	}

	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		return
	}
	widget.Readings = groups
}

func (widget *horoscopeWidget) Render() template.HTML {
	return widget.renderTemplate(widget, horoscopeWidgetTemplate)
}

func (widget *horoscopeWidget) PeriodTitle(period string) string {
	if period == "" {
		return ""
	}
	return strings.ToUpper(period[:1]) + period[1:]
}
