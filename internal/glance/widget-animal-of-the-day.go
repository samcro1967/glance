package glance

import (
	"context"
	"fmt"
	"hash/fnv"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var animalOfTheDayWidgetTemplate = mustParseTemplate("animal-of-the-day.html", "widget-base.html")
var animalOfTheDayGroups = []string{"Mammalia", "Aves", "Reptilia", "Amphibia", "Actinopterygii", "Insecta", "Arachnida", "Mollusca"}

const iNaturalistSpeciesCountsURL = "https://api.inaturalist.org/v1/observations/species_counts"

type animalOfTheDayWidget struct {
	widgetBase `yaml:",inline"`
	Animal     animalOfTheDay `yaml:"-"`
}
type animalOfTheDay struct {
	CommonName, ScientificName, Group, ImageURL, ImageAttribution, ImageLicense, WikipediaURL, INaturalistURL, ConservationStatus, ConservationAuthority string
	ObservationCount                                                                                                                                     int
}
type iNaturalistSpeciesCountsResponse struct {
	Results []struct {
		Count int `json:"count"`
		Taxon struct {
			ID                  int    `json:"id"`
			Rank                string `json:"rank"`
			IsActive            bool   `json:"is_active"`
			Extinct             bool   `json:"extinct"`
			Name                string `json:"name"`
			PreferredCommonName string `json:"preferred_common_name"`
			WikipediaURL        string `json:"wikipedia_url"`
			DefaultPhoto        struct {
				LicenseCode *string `json:"license_code"`
				Attribution string  `json:"attribution"`
				MediumURL   string  `json:"medium_url"`
			} `json:"default_photo"`
			ConservationStatus *struct {
				Authority  string `json:"authority"`
				StatusName string `json:"status_name"`
			} `json:"conservation_status"`
		} `json:"taxon"`
	} `json:"results"`
}

func (w *animalOfTheDayWidget) initialize() error {
	w.withTitle("Animal of the Day").withCacheDuration(24 * time.Hour)
	return nil
}
func (w *animalOfTheDayWidget) update(ctx context.Context) {
	date := time.Now().Format("2006-01-02")
	group := animalOfTheDayGroups[stableDailyIndex(date, len(animalOfTheDayGroups))]
	query := url.Values{"iconic_taxa": {group}, "rank": {"species"}, "quality_grade": {"research"}, "per_page": {"100"}}
	providerURL := dailyDiscoveryProviderURL("/daily-discovery/animal", iNaturalistSpeciesCountsURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, providerURL+"?"+query.Encode(), nil)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	req.Header.Set("User-Agent", glanceUserAgentString)
	response, err := decodeJsonFromRequest[iNaturalistSpeciesCountsResponse](defaultHTTPClient, req)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("fetching iNaturalist species: %w", err))
		return
	}
	animal, err := selectAnimalOfTheDay(response, group, date)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	animal.ImageURL = w.resolveResourceProxyImageURL(animal.ImageURL)
	if animal.ImageURL == "" {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("iNaturalist animal image could not be rendered safely"))
		return
	}
	if !w.canContinueUpdateAfterHandlingErr(nil) {
		return
	}
	w.Animal = animal
}
func (w *animalOfTheDayWidget) Render() template.HTML {
	return w.renderTemplate(w, animalOfTheDayWidgetTemplate)
}
func stableDailyIndex(seed string, count int) int {
	if count < 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return int(h.Sum32() % uint32(count))
}
func selectAnimalOfTheDay(r iNaturalistSpeciesCountsResponse, group, date string) (animalOfTheDay, error) {
	candidates := make([]animalOfTheDay, 0, len(r.Results))
	for _, result := range r.Results {
		t := result.Taxon
		if t.Rank != "species" || !t.IsActive || t.Extinct || strings.TrimSpace(t.PreferredCommonName) == "" || strings.TrimSpace(t.WikipediaURL) == "" || strings.TrimSpace(t.DefaultPhoto.MediumURL) == "" || t.DefaultPhoto.LicenseCode == nil || !acceptableINaturalistLicense(*t.DefaultPhoto.LicenseCode) {
			continue
		}
		animal := animalOfTheDay{CommonName: t.PreferredCommonName, ScientificName: t.Name, Group: group, ImageURL: t.DefaultPhoto.MediumURL, ImageAttribution: t.DefaultPhoto.Attribution, ImageLicense: strings.ToUpper(*t.DefaultPhoto.LicenseCode), WikipediaURL: t.WikipediaURL, INaturalistURL: fmt.Sprintf("https://www.inaturalist.org/taxa/%d", t.ID), ObservationCount: result.Count}
		if t.ConservationStatus != nil {
			animal.ConservationStatus = titleWords(t.ConservationStatus.StatusName)
			animal.ConservationAuthority = t.ConservationStatus.Authority
		}
		candidates = append(candidates, animal)
	}
	if len(candidates) == 0 {
		return animalOfTheDay{}, fmt.Errorf("iNaturalist returned no reusable animal candidates for %s", group)
	}
	return candidates[stableDailyIndex(date+":"+group, len(candidates))], nil
}
func acceptableINaturalistLicense(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "cc0", "cc-by", "cc-by-sa", "cc-by-nc", "cc-by-nc-sa", "cc-by-nd", "cc-by-nc-nd":
		return true
	default:
		return false
	}
}

func titleWords(value string) string {
	words := strings.Fields(strings.ToLower(value))
	for i := range words {
		if len(words[i]) > 0 {
			words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
		}
	}
	return strings.Join(words, " ")
}
