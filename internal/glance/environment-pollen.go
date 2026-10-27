package glance

import (
	"sort"
	"strings"
	"time"
)

func environmentRiskRank(risk string) int {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "very high":
		return 4
	case "high":
		return 3
	case "moderate":
		return 2
	case "low":
		return 1
	case "absent":
		return 0
	default:
		return -1
	}
}

func (p *environmentPollen) currentAt(now time.Time) *environmentPollenDay {
	location, err := time.LoadLocation(p.Timezone)
	if err != nil {
		location = time.UTC
	}
	today := now.In(location).Format("2006-01-02")
	var selected *environmentPollenDay
	for i := range p.Days {
		day := &p.Days[i]
		if _, err := time.Parse("2006-01-02", day.Date); err != nil || day.Date > today {
			continue
		}
		if selected == nil || day.Date > selected.Date {
			selected = day
		}
	}
	return selected
}

func (p *environmentPollen) IsObservation() bool { return p.Provider == "st-louis-county" }
func (p *environmentPollen) SourceName() string {
	if p.IsObservation() {
		return "St. Louis County"
	}
	return "AtmoSpore"
}
func (p *environmentPollen) SourceURL() string {
	if p.IsObservation() {
		return "https://pollenandmold.stlouisco.com/"
	}
	return "https://atmospore.com/"
}
func (p *environmentPollen) ReadingLabel() string {
	if p.IsObservation() {
		return "Regional observation"
	}
	return "Pollen forecast"
}
func (p *environmentPollen) DateStatus() string {
	day := p.Current()
	if day == nil {
		return "No current report"
	}
	location, err := time.LoadLocation(p.Timezone)
	if err != nil {
		location = time.UTC
	}
	if day.Date != time.Now().In(location).Format("2006-01-02") {
		return "Earlier report; not today's conditions"
	}
	return "Today's report"
}
func (p *environmentPollen) TimelineLabel() string {
	if p.IsObservation() {
		return "Recent regional observations"
	}
	return "Pollen forecast by date"
}
func (s environmentPollenSpecies) Units() string {
	if s.Category == "mold" {
		return "spores/m³"
	}
	return "grains/m³"
}

// County categories are measured totals. AtmoSpore categories instead use the
// highest individual risk: adding its family and species entries would double
// count overlapping taxonomic groups and invent a concentration total.
func (d *environmentPollenDay) CategoryLevels() []environmentPollenSpecies {
	if len(d.Categories) > 0 {
		return d.Categories
	}
	levels := []environmentPollenSpecies{
		{Name: "Tree", Category: "tree", Unavailable: true},
		{Name: "Grass", Category: "grass", Unavailable: true},
		{Name: "Weed", Category: "weed", Unavailable: true},
		{Name: "Mold", Category: "mold", Unavailable: true},
	}
	for i := range levels {
		level := &levels[i]
		seen, complete := false, true
		for _, species := range d.Species {
			if species.Category != level.Category {
				continue
			}
			seen = true
			if species.Unavailable || environmentRiskRank(species.RiskLevel) < 0 {
				complete = false
				continue
			}
			if environmentRiskRank(species.RiskLevel) > environmentRiskRank(level.RiskLevel) {
				level.RiskLevel = species.RiskLevel
			}
		}
		level.Unavailable = !seen || !complete
	}
	return levels
}

func (d *environmentPollenDay) AllSpecies() []environmentPollenSpecies {
	// Sort a copy so presentation never changes the cached provider data.
	out := append([]environmentPollenSpecies(nil), d.Species...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Unavailable != out[j].Unavailable {
			return !out[i].Unavailable
		}
		if a, b := environmentRiskRank(out[i].RiskLevel), environmentRiskRank(out[j].RiskLevel); a != b {
			return a > b
		}
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (d *environmentPollenDay) ReportedCategoryLevels() []environmentPollenSpecies {
	var out []environmentPollenSpecies
	for _, level := range d.CategoryLevels() {
		visible := level.Unavailable
		if len(d.Categories) > 0 {
			visible = visible || level.Value > 0
		} else {
			// Forecast categories have no concentration total. Check their
			// species rather than treating the unset category value as zero.
			for _, species := range d.Species {
				if species.Category == level.Category && species.Value > 0 {
					visible = true
					break
				}
			}
		}
		if visible {
			out = append(out, level)
		}
	}
	return out
}

func (d *environmentPollenDay) ReportedSpecies() []environmentPollenSpecies {
	// Hide only confirmed zeros. Missing measurements must remain visible so
	// incomplete reports cannot be mistaken for an absence of allergens.
	var out []environmentPollenSpecies
	for _, species := range d.AllSpecies() {
		if species.Unavailable || species.Value > 0 {
			out = append(out, species)
		}
	}
	return out
}

func environmentPollenCacheValid(c cachedEntry[*environmentPollen], now time.Time) bool {
	location, err := time.LoadLocation(c.value.Timezone)
	if err != nil {
		return false
	}
	return now.Sub(c.timestamp) < 6*time.Hour &&
		c.timestamp.In(location).Format("2006-01-02") == now.In(location).Format("2006-01-02")
}
