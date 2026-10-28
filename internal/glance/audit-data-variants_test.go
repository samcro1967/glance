package glance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestSourceFreeCalendarExpandedRoute(t *testing.T) {
	a := newGlanceTestApplication(t, `pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: calendar
`)
	w := a.Config.Pages[0].Columns[0].Widgets[0].(*calendarWidget)
	if w.now == nil {
		t.Fatal("source-free calendar clock was not initialized")
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/widgets/"+strconv.FormatUint(w.ID, 10)+"/expanded/", nil)
	a.router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "No upcoming events.") {
		t.Fatalf("empty calendar expanded route: %d %s", recorder.Code, recorder.Body.String())
	}

}

func TestEnvironmentMissingReadingsAreNotZero(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"current":{"us_aqi":null,"uv_index":null}}`} {
		t.Run(body, func(t *testing.T) {
			wave3Transport(t, func(*http.Request) (*http.Response, error) { return wave3Response(200, body, nil), nil })
			if _, err := fetchOpenMeteoEnvironment(context.Background(), environmentTestPlace()); err == nil {
				t.Fatal("empty readings accepted as real zero")
			}
		})
	}
	wave3Transport(t, func(*http.Request) (*http.Response, error) {
		return wave3Response(200, `{"current":{"us_aqi":null,"uv_index":0,"pm2_5":0}}`, nil), nil
	})
	air, err := fetchOpenMeteoEnvironment(context.Background(), environmentTestPlace())
	if err != nil {
		t.Fatal(err)
	}
	if air.HasMeasurement("us_aqi") || air.USAQICategory() != "Unavailable" || !air.HasMeasurement("uv_index") || air.UVCategory() != "Low" {
		t.Fatalf("missing versus zero lost: %+v", air)
	}
	w := &environmentWidget{AirQuality: air}
	w.ContentAvailable = true
	for name, output := range map[string]string{"compact": string(w.Render()), "expanded": string(w.RenderExpanded())} {
		if !strings.Contains(output, "Unavailable") || strings.Contains(output, ">Good<") || strings.Contains(output, " · Good") {
			t.Fatalf("%s fabricates current AQI: %s", name, output)
		}
	}
	m := &microEnvironment{environmentWidget: environmentWidget{AirQuality: air}}
	m.Display = []string{"air-quality", "uv"}
	items := m.MicroItems(true)
	if len(items) != 1 || !strings.Contains(items[0].Line1, "AQI unavailable") || !strings.Contains(items[0].Line1, "UV 0.0 Low") {
		t.Fatalf("micro availability lost: %+v", items)
	}
}
