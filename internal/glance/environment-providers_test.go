package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func countyTestRow(t *testing.T, date string) map[string]json.RawMessage {
	t.Helper()
	row := make(map[string]json.RawMessage)
	row["CountDate"] = json.RawMessage(fmt.Sprintf("%q", date+"T00:00:00"))
	for _, field := range stLouisCountySpecies {
		row[field.Field] = json.RawMessage("0")
	}
	return row
}

func TestEnvironmentProviderSelection(t *testing.T) {
	for _, name := range []string{"", "atmospore", " ST-LOUIS-COUNTY "} {
		w := &environmentWidget{Location: "O Fallon, Missouri", PollenProvider: name}
		if err := w.initialize(); err != nil {
			t.Fatalf("provider %q: %v", name, err)
		}
		if name == "" && w.PollenProvider != "atmospore" {
			t.Fatal("default provider changed")
		}
		if w.HasPollenProvider() != (w.PollenProvider == "st-louis-county") {
			t.Fatal("county must work without a key; default pollen stays optional")
		}
	}
	w := &environmentWidget{Location: "Test", PollenProvider: "unknown"}
	if w.initialize() == nil {
		t.Fatal("unknown provider accepted")
	}
	config, err := newConfigFromYAML([]byte(`
widget-defaults:
  global:
    timeout: 12s
pages:
  - name: Test
    columns:
      - size: full
        widgets:
          - type: environment
            location: O Fallon, Missouri
            pollen-provider: st-louis-county
`))
	if err != nil {
		t.Fatal(err)
	}
	configured := config.Pages[0].Columns[0].Widgets[0].(*environmentWidget)
	if configured.Timeout != durationField(12*time.Second) || configured.PollenProvider != "st-louis-county" {
		t.Fatal("provider config or inherited timeout lost")
	}
}

func TestEnvironmentCountyParsingAndThresholds(t *testing.T) {
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	older, latest := countyTestRow(t, "2026-10-23"), countyTestRow(t, "2026-10-26")
	latest["Mold"] = json.RawMessage("12180")
	latest["WeedRagweed"] = json.RawMessage("12")
	latest["WeedNettle"] = json.RawMessage("3")
	latest["GrassCattail"] = json.RawMessage("2")
	p, err := parseStLouisCountyPollen([]map[string]json.RawMessage{latest, older}, now)
	if err != nil {
		t.Fatal(err)
	}
	d := p.currentAt(now)
	if !p.IsObservation() || d.Date != "2026-10-26" || d.OverallRisk != "moderate" {
		t.Fatalf("unexpected county report: %+v", d)
	}
	if d.Categories[0].RiskLevel != "absent" || d.Categories[1].Value != 0 || d.Categories[2].Value != 17 || d.Categories[3].RiskLevel != "moderate" {
		t.Fatalf("incorrect category mapping: %+v", d.Categories)
	}
	if d.Categories[3].Units() != "spores/m³" {
		t.Fatal("mold has pollen units")
	}
	for category, thresholds := range map[string][]float64{"tree": {15, 90, 1500}, "grass": {5, 20, 200}, "weed": {10, 50, 500}, "mold": {6500, 13000, 50000}} {
		if stLouisCountyRisk(category, 0) != "absent" {
			t.Fatal("zero must remain absent")
		}
		for i, value := range thresholds {
			if got := stLouisCountyRisk(category, value); got != []string{"moderate", "high", "very high"}[i] {
				t.Fatalf("%s at %v: %s", category, value, got)
			}
			if got := stLouisCountyRisk(category, value-0.1); got != []string{"low", "moderate", "high"}[i] {
				t.Fatalf("%s below %v: %s", category, value, got)
			}
		}
	}
}

func TestEnvironmentCountyMissingAndInvalidData(t *testing.T) {
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	row := countyTestRow(t, "2026-10-26")
	delete(row, "TreeOak")
	row["Mold"] = json.RawMessage("null")
	p, err := parseStLouisCountyPollen([]map[string]json.RawMessage{row}, now)
	if err != nil {
		t.Fatal(err)
	}
	d := p.currentAt(now)
	if !d.Categories[0].Unavailable || !d.Categories[3].Unavailable || d.Categories[1].Unavailable || d.OverallRisk != "unknown" {
		t.Fatal("missing measurements confused with reported zero")
	}
	for _, rows := range [][]map[string]json.RawMessage{
		nil,
		{{"CountDate": json.RawMessage(`"invalid"`)}},
		{{"CountDate": json.RawMessage(`"2026-10-26T00:00:00"`)}},
		{countyTestRow(t, "2026-10-28")},
	} {
		if _, err := parseStLouisCountyPollen(rows, now); err == nil {
			t.Fatal("invalid or empty county response accepted")
		}
	}
	row = countyTestRow(t, "2026-10-26")
	row["Mold"] = json.RawMessage(`"not a number"`)
	if _, err := parseStLouisCountyPollen([]map[string]json.RawMessage{row}, now); err == nil {
		t.Fatal("invalid concentration accepted")
	}
}

func TestEnvironmentPollenDateAndCacheRollover(t *testing.T) {
	p := &environmentPollen{Timezone: "America/Chicago", Days: []environmentPollenDay{{Date: "2026-10-27"}, {Date: "2026-10-26"}}}
	before := time.Date(2026, 10, 27, 4, 59, 0, 0, time.UTC)
	after := before.Add(2 * time.Minute)
	if p.currentAt(before).Date != "2026-10-26" || p.currentAt(after).Date != "2026-10-27" {
		t.Fatal("current report ignores local date")
	}
	cached := cachedEntry[*environmentPollen]{value: p, timestamp: before}
	if !environmentPollenCacheValid(cached, before.Add(30*time.Second)) || environmentPollenCacheValid(cached, after) {
		t.Fatal("cache reused across local midnight")
	}
	p.Days = []environmentPollenDay{{Date: "2026-10-28"}, {Date: "invalid"}}
	if p.currentAt(after) != nil {
		t.Fatal("future/invalid date treated as current")
	}
}

func TestEnvironmentAllergenRiskOrdering(t *testing.T) {
	d := &environmentPollenDay{Species: []environmentPollenSpecies{
		{Name: "High grass", Category: "grass", Value: 20, RiskLevel: "high"},
		{Name: "Low tree", Category: "tree", Value: 100, RiskLevel: "low"},
		{Name: "Zero", Category: "weed", Value: 0, RiskLevel: "low"},
		{Name: "Missing", Category: "weed", Unavailable: true},
	}}
	active := d.ActiveSpecies()
	if len(active) != 2 || active[0].Name != "High grass" || len(d.AllSpecies()) != 4 {
		t.Fatal("risk ordering or expanded availability lost")
	}
	levels := d.CategoryLevels()
	if levels[1].RiskLevel != "high" || !levels[3].Unavailable {
		t.Fatal("category risk or missing mold incorrect")
	}
}

func TestEnvironmentAtmoSporeNullConcentrations(t *testing.T) {
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		return wave3Response(http.StatusOK, `{"data":[{"date":"2026-10-27","overall_risk":"low","species":{"oak":{"value":null,"category":"tree"},"grass":{"value":0,"risk_level":"low","category":"grass"},"weed":{"category":"weed"}}}]}`, nil), nil
	})
	p, err := fetchAtmoSporePollen(context.Background(), environmentTestPlace(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	d := &p.Days[0]
	levels := d.CategoryLevels()
	if !levels[0].Unavailable || levels[1].Unavailable || !levels[2].Unavailable || d.OverallRisk != "unknown" {
		t.Fatalf("null/zero distinction lost: %+v", levels)
	}
}

func TestEnvironmentCountyRequestAndFailurePreservation(t *testing.T) {
	old := &environmentPollen{Provider: "st-louis-county", Timezone: "America/Chicago", Days: []environmentPollenDay{{Date: "2026-10-26"}}}
	stLouisCountyPollenCache.mu.Lock()
	previous := stLouisCountyPollenCache.entries
	stLouisCountyPollenCache.entries = make(map[durationField]*keyedResourceCacheEntry[*environmentPollen])
	stLouisCountyPollenCache.mu.Unlock()
	t.Cleanup(func() {
		stLouisCountyPollenCache.mu.Lock()
		stLouisCountyPollenCache.entries = previous
		stLouisCountyPollenCache.mu.Unlock()
	})
	resetEnvironmentResourceCaches(t)
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "pollenapi.com" {
			t.Fatal("county provider called AtmoSpore")
		}
		if r.URL.Host == "pollenandmoldcharts.stlouisco.com" {
			if r.Header.Get("x-api-key") != "" || r.URL.Query().Get("begDate") == "" || r.URL.Query().Get("endDate") == "" {
				t.Fatal("county request contract incorrect")
			}
			return nil, errors.New("county unavailable")
		}
		return wave3Response(http.StatusOK, `{"current":{"us_aqi":25}}`, nil), nil
	})
	w := &environmentWidget{Location: "O Fallon", Place: environmentTestPlace(), PollenProvider: "st-louis-county", PollenAPIKey: "must-not-send", Pollen: old}
	w.ContentAvailable = true
	w.update(context.Background())
	if w.Pollen != old || w.AirQuality == nil || w.PollenError == nil || !errors.Is(w.Notice, errPartialContent) {
		t.Fatal("independent provider failure lost successful or cached data")
	}
	for _, rendered := range []string{string(w.Render()), string(w.RenderExpanded())} {
		for _, expected := range []string{"St. Louis County", "2026-10-26", "Earlier report", "showing last available data"} {
			if !strings.Contains(rendered, expected) {
				t.Fatalf("missing %q", expected)
			}
		}
		if strings.Contains(rendered, "AtmoSpore") || strings.Contains(rendered, "Active pollen today") {
			t.Fatal("wrong source or misleading today label")
		}
	}
}

func TestEnvironmentCountySuccessCachesByTimeout(t *testing.T) {
	stLouisCountyPollenCache.mu.Lock()
	previous := stLouisCountyPollenCache.entries
	stLouisCountyPollenCache.entries = make(map[durationField]*keyedResourceCacheEntry[*environmentPollen])
	stLouisCountyPollenCache.mu.Unlock()
	t.Cleanup(func() {
		stLouisCountyPollenCache.mu.Lock()
		stLouisCountyPollenCache.entries = previous
		stLouisCountyPollenCache.mu.Unlock()
	})
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	row := countyTestRow(t, time.Now().In(location).AddDate(0, 0, -1).Format("2006-01-02"))
	row["Mold"] = json.RawMessage("12180")
	body, err := json.Marshal([]map[string]json.RawMessage{row})
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "pollenandmoldcharts.stlouisco.com" || r.Header.Get("x-api-key") != "" {
			t.Fatal("wrong county endpoint or leaked API key")
		}
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("county request is unbounded")
		}
		expected := 5 * time.Second
		if requests == 2 {
			expected = 8 * time.Second
		}
		if remaining := time.Until(deadline); remaining > expected || remaining < expected-time.Second {
			t.Fatalf("timeout not applied: %v", remaining)
		}
		return wave3Response(http.StatusOK, string(body), nil), nil
	})
	var first *environmentPollen
	for i, timeout := range []durationField{0, durationField(5 * time.Second), durationField(8 * time.Second)} {
		p, err := fetchStLouisCountyPollenResource(context.Background(), timeout)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = p
		}
		if i == 1 && p != first {
			t.Fatal("equivalent timeout budgets did not share cache")
		}
		if p.Current().Categories[3].Value != 12180 {
			t.Fatal("county mold observation lost")
		}
	}
	if requests != 2 {
		t.Fatalf("requests=%d, want two distinct timeout budgets", requests)
	}
}

func TestEnvironmentCountyHistoryAndRendering(t *testing.T) {
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	rows := []map[string]json.RawMessage{}
	for i := 0; i < 10; i++ {
		rows = append(rows, countyTestRow(t, now.AddDate(0, 0, -i).Format("2006-01-02")))
	}
	p, err := parseStLouisCountyPollen(rows, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Days) != 7 || p.Days[0].Date != "2026-10-21" || p.currentAt(now).Date != "2026-10-27" {
		t.Fatal("history must retain latest seven reports in date order")
	}
	w := &environmentWidget{Pollen: p}
	w.ContentAvailable = true
	expanded := string(w.RenderExpanded())
	for _, expected := range []string{"Recent regional observations", "No nonzero allergen concentrations reported", "Absent", "St. Louis metropolitan area"} {
		if !strings.Contains(expanded, expected) {
			t.Fatalf("expanded missing %q", expected)
		}
	}
	if strings.Contains(expanded, "Pollen forecast by date") {
		t.Fatal("observations labeled as forecasts")
	}
	m := &microEnvironment{Display: []string{"pollen"}}
	m.Pollen = p
	items := m.MicroItems(false)
	if len(items) != 1 || !strings.Contains(items[0].Line1, "Regional allergens") || !strings.Contains(items[0].Line1, "St. Louis County") {
		t.Fatal("micro view lost regional source")
	}
}

func TestEnvironmentZeroAllergensHiddenInBothViews(t *testing.T) {
	const empty = "No nonzero allergen concentrations reported for this date."
	for _, provider := range []string{"atmospore", "st-louis-county"} {
		for _, scenario := range []string{"mixed", "all-zero", "missing"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				day := environmentPollenDay{Date: "2026-10-26", OverallRisk: "low", Species: []environmentPollenSpecies{
					{Name: "Zero tree", Category: "tree", RiskLevel: "low"},
					{Name: "Zero grass", Category: "grass", RiskLevel: "low"},
					{Name: "Present weed", Category: "weed", RiskLevel: "low"},
				}}
				if scenario == "mixed" {
					day.Species[2].Value = 2
				} else if scenario == "missing" {
					day.Species[2].Unavailable = true
				}
				if provider == "st-louis-county" {
					for _, species := range day.Species {
						day.Categories = append(day.Categories, environmentPollenSpecies{
							Name: environmentDisplayRisk(species.Category), Category: species.Category,
							Value: species.Value, Unavailable: species.Unavailable,
							RiskLevel: stLouisCountyRisk(species.Category, species.Value),
						})
					}
				}
				w := &environmentWidget{Pollen: &environmentPollen{Provider: provider, Timezone: "America/Chicago", Days: []environmentPollenDay{day}}}
				w.ContentAvailable = true
				for view, html := range map[string]string{"compact": string(w.Render()), "expanded": string(w.RenderExpanded())} {
					for _, hidden := range []string{"Zero tree", "Zero grass", `>Tree</span>`, `>Grass</span>`, "0.0 grains/m³"} {
						if strings.Contains(html, hidden) {
							t.Fatalf("%s still shows %q", view, hidden)
						}
					}
					if scenario == "all-zero" {
						if strings.Count(html, empty) != 1 || strings.Contains(html, `>Weed</span>`) || strings.Contains(html, "Present weed") {
							t.Fatalf("%s must show one empty state and no zero rows: %s", view, html)
						}
					} else if strings.Contains(html, empty) || !strings.Contains(html, `>Weed</span>`) {
						t.Fatalf("%s lost nonzero or unavailable category", view)
					}
					if scenario == "mixed" && !strings.Contains(html, "Present weed") {
						t.Fatalf("%s lost a nonzero allergen below moderate risk", view)
					}
					if scenario == "missing" && (!strings.Contains(html, "Unavailable") || view == "expanded" && !strings.Contains(html, "Present weed")) {
						t.Fatalf("%s lost unavailable data", view)
					}
					if provider == "atmospore" && !strings.Contains(html, "Not provided") {
						t.Fatalf("%s lost unsupported mold disclosure", view)
					}
				}
				if len(day.AllSpecies()) != 3 || day.Species[0].Name != "Zero tree" || day.CategoryLevels()[0].Unavailable {
					t.Fatal("presentation filtering changed provider data")
				}
			})
		}
	}
}
