package glance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const stLouisCountyPollenURL = "https://pollenandmoldcharts.stlouisco.com/Api/PollenMold/GetCounts/"

var stLouisCountyPollenCache = newKeyedResourceCache[durationField, *environmentPollen](24 * time.Hour)

func fetchStLouisCountyPollenResource(ctx context.Context, timeout durationField) (*environmentPollen, error) {
	// This source is a single regional station, independent of the widget's
	// geocoded location. Only explicit provider selection enables it.
	return stLouisCountyPollenCache.Get(ctx, resourceTimeoutPolicy(timeout), func(c cachedEntry[*environmentPollen], now time.Time) bool {
		return now.Sub(c.timestamp) < time.Hour && environmentPollenCacheValid(c, now)
	}, func(ctx context.Context) (*environmentPollen, error) {
		return fetchStLouisCountyPollen(ctx, timeout, time.Now())
	})
}

func fetchStLouisCountyPollen(ctx context.Context, timeout durationField, now time.Time) (*environmentPollen, error) {
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return nil, fmt.Errorf("loading county timezone: %w", err)
	}
	now = now.In(location)
	query := url.Values{}
	query.Set("begDate", now.AddDate(0, 0, -14).Format("01/02/2006"))
	query.Set("endDate", now.Format("01/02/2006"))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, stLouisCountyPollenURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating county pollen request: %w", err)
	}
	response, err := decodeJsonFromRequest[[]map[string]json.RawMessage](newProviderHTTPClient(timeout), request)
	if err != nil {
		return nil, fmt.Errorf("fetching county pollen observations: %w", err)
	}
	return parseStLouisCountyPollen(response, now)
}

var stLouisCountySpecies = []struct{ Field, Name, Category string }{
	{"TreeAsh", "Ash", "tree"}, {"TreeBirch", "Birch", "tree"},
	{"TreeChestnut", "Chestnut", "tree"}, {"TreeElm", "Elm", "tree"},
	{"TreeHickory", "Hickory", "tree"}, {"TreeJuniper", "Juniper", "tree"},
	{"TreeMaple", "Maple", "tree"}, {"TreeMulberry", "Mulberry", "tree"},
	{"TreeOak", "Oak", "tree"}, {"TreePine", "Pine", "tree"},
	{"TreePoplar", "Poplar", "tree"}, {"TreeSweetgum", "Sweetgum", "tree"},
	{"TreeSycamore", "Sycamore", "tree"}, {"TreeWalnut", "Walnut", "tree"},
	{"TreeWillow", "Willow", "tree"}, {"GrassGrass", "Grass", "grass"},
	// The website's field name is historical; its public species page groups
	// cattail with weeds, so do not silently count it as grass.
	{"GrassCattail", "Cattail", "weed"}, {"WeedDock", "Dock", "weed"},
	{"WeedGoldenrod", "Goldenrod", "weed"}, {"WeedLambsquarter", "Lambsquarter", "weed"},
	{"WeedNettle", "Nettle", "weed"}, {"WeedPlantain", "Plantain", "weed"},
	{"WeedRagweed", "Ragweed", "weed"}, {"WeedWormwood", "Wormwood", "weed"},
	{"Mold", "Mold", "mold"},
}

func parseStLouisCountyPollen(rows []map[string]json.RawMessage, now time.Time) (*environmentPollen, error) {
	out := &environmentPollen{Provider: "st-louis-county", Timezone: "America/Chicago"}
	seen := make(map[string]bool)
	for _, row := range rows {
		var timestamp string
		if err := json.Unmarshal(row["CountDate"], &timestamp); err != nil {
			return nil, fmt.Errorf("county observation has invalid CountDate")
		}
		date := strings.SplitN(timestamp, "T", 2)[0]
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, fmt.Errorf("county observation has invalid report date")
		}
		if date > now.Format("2006-01-02") || seen[date] {
			continue
		}
		seen[date] = true
		day := environmentPollenDay{Date: date, OverallRisk: "absent"}
		complete := map[string]bool{"tree": true, "grass": true, "weed": true, "mold": true}
		totals := make(map[string]float64)
		known := 0
		for _, field := range stLouisCountySpecies {
			reading := environmentPollenSpecies{Name: field.Name, Category: field.Category, Unavailable: true}
			var value *float64
			if raw, exists := row[field.Field]; exists {
				if err := json.Unmarshal(raw, &value); err != nil {
					return nil, fmt.Errorf("county observation has invalid %s concentration", field.Field)
				}
			}
			if value != nil && *value >= 0 {
				reading.Value, reading.Unavailable = *value, false
				reading.RiskLevel = stLouisCountyRisk(field.Category, *value)
				totals[field.Category] += *value
				known++
			} else {
				complete[field.Category] = false
			}
			day.Species = append(day.Species, reading)
		}
		if known == 0 {
			return nil, fmt.Errorf("county observation contains no recognized concentrations")
		}
		for _, category := range []string{"tree", "grass", "weed", "mold"} {
			level := environmentPollenSpecies{Name: environmentDisplayRisk(category), Category: category, Value: totals[category], Unavailable: !complete[category]}
			if !level.Unavailable {
				level.RiskLevel = stLouisCountyRisk(category, level.Value)
				if environmentRiskRank(level.RiskLevel) > environmentRiskRank(day.OverallRisk) {
					day.OverallRisk = level.RiskLevel
				}
			} else {
				day.OverallRisk = "unknown"
			}
			day.Categories = append(day.Categories, level)
		}
		for _, category := range day.Categories {
			if category.Unavailable {
				day.OverallRisk = "unknown"
				break
			}
		}
		out.Days = append(out.Days, day)
	}
	if len(out.Days) == 0 {
		return nil, fmt.Errorf("county provider returned no observations")
	}
	sort.Slice(out.Days, func(i, j int) bool { return out.Days[i].Date < out.Days[j].Date })
	if len(out.Days) > 7 {
		out.Days = out.Days[len(out.Days)-7:]
	}
	return out, nil
}

// Category-specific NAB thresholds published by the county:
// https://pollenandmold.stlouisco.com/html/PollenLevels.html
func stLouisCountyRisk(category string, value float64) string {
	if value == 0 {
		return "absent"
	}
	thresholds := map[string][3]float64{
		"tree": {15, 90, 1500}, "grass": {5, 20, 200},
		"weed": {10, 50, 500}, "mold": {6500, 13000, 50000},
	}
	limits := thresholds[category]
	for i, limit := range limits {
		if value < limit {
			return []string{"low", "moderate", "high"}[i]
		}
	}
	return "very high"
}
