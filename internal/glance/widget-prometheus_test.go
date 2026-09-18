package glance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusWidgetInitialize(t *testing.T) {
	t.Run("defaults and path prefix", func(t *testing.T) {
		widget := &prometheusWidget{
			Server: "https://example.com/prometheus/",
			Query:  "up",
		}

		if err := widget.initialize(); err != nil {
			t.Fatalf("initialize returned an error: %v", err)
		}

		if widget.Endpoint != "https://example.com/prometheus/api/v1/query_range" {
			t.Fatalf("unexpected endpoint: %s", widget.Endpoint)
		}
		if widget.QueryRange != 24*time.Hour {
			t.Fatalf("unexpected default range: %s", widget.QueryRange)
		}
		if widget.QueryStep != 12*time.Minute {
			t.Fatalf("unexpected automatic step: %s", widget.QueryStep)
		}
		if !widget.ShowValue {
			t.Fatal("show-value should default to true")
		}
		if widget.Unit != "number" {
			t.Fatalf("unexpected default unit: %s", widget.Unit)
		}
		if widget.cacheDuration != 5*time.Minute {
			t.Fatalf("unexpected default cache duration: %s", widget.cacheDuration)
		}
	})

	t.Run("explicit settings", func(t *testing.T) {
		showValue := false
		widget := &prometheusWidget{
			Server:       "http://example.com",
			Query:        "up",
			Range:        durationField(2 * time.Hour),
			Step:         durationField(30 * time.Second),
			Unit:         "percent",
			ShowValueRaw: &showValue,
		}

		if err := widget.initialize(); err != nil {
			t.Fatalf("initialize returned an error: %v", err)
		}

		if widget.QueryRange != 2*time.Hour || widget.QueryStep != 30*time.Second {
			t.Fatalf("explicit range and step were not preserved: %s, %s", widget.QueryRange, widget.QueryStep)
		}
		if widget.ShowValue {
			t.Fatal("explicit show-value false was not preserved")
		}
	})

	t.Run("automatic step has one second minimum", func(t *testing.T) {
		widget := &prometheusWidget{
			Server: "http://example.com",
			Query:  "up",
			Range:  durationField(time.Second),
		}

		if err := widget.initialize(); err != nil {
			t.Fatalf("initialize returned an error: %v", err)
		}
		if widget.QueryStep != time.Second {
			t.Fatalf("unexpected minimum automatic step: %s", widget.QueryStep)
		}
	})

	tests := []struct {
		name   string
		widget *prometheusWidget
	}{
		{name: "missing server", widget: &prometheusWidget{Query: "up"}},
		{name: "missing query", widget: &prometheusWidget{Server: "https://example.com"}},
		{name: "relative server", widget: &prometheusWidget{Server: "example.com", Query: "up"}},
		{name: "server query string", widget: &prometheusWidget{Server: "https://example.com?x=1", Query: "up"}},
		{name: "negative range", widget: &prometheusWidget{Server: "https://example.com", Query: "up", Range: durationField(-time.Hour)}},
		{name: "negative step", widget: &prometheusWidget{Server: "https://example.com", Query: "up", Step: durationField(-time.Second)}},
		{name: "invalid unit", widget: &prometheusWidget{Server: "https://example.com", Query: "up", Unit: "bananas"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.widget.initialize(); err == nil {
				t.Fatal("expected initialize to return an error")
			}
		})
	}
}

func TestFetchPrometheusGraph(t *testing.T) {
	now := time.Unix(200000, 0)
	start := now.Add(-24 * time.Hour)

	var receivedRequest *http.Request
	var receivedQuery string
	var receivedStart string
	var receivedEnd string
	var receivedStep string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = r.Clone(r.Context())
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing request form: %v", err)
		}
		receivedQuery = r.Form.Get("query")
		receivedStart = r.Form.Get("start")
		receivedEnd = r.Form.Get("end")
		receivedStep = r.Form.Get("step")

		_, _ = fmt.Fprintf(w, `{
			"status":"success",
			"warnings":["partial data"],
			"infos":["query hint"],
			"data":{"resultType":"matrix","result":[
				{"metric":{"instance":"first"},"values":[[%d,"1"],[%d,"NaN"],[%d,"2"]]}
			]}
		}`, start.Unix(), start.Add(time.Hour).Unix(), now.Unix())
	}))
	defer server.Close()

	widget := &prometheusWidget{
		Server:  server.URL + "/prefix/",
		Query:   `sum(rate(http_requests_total{code=~"5.."}[5m]))`,
		Headers: map[string]string{"Authorization": "Bearer test-token"},
	}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned an error: %v", err)
	}

	graph, annotations, err := fetchPrometheusGraph(context.Background(), widget, now)
	if err != nil {
		t.Fatalf("fetch returned an error: %v", err)
	}

	if receivedRequest.Method != http.MethodPost {
		t.Fatalf("unexpected request method: %s", receivedRequest.Method)
	}
	if receivedRequest.URL.Path != "/prefix/api/v1/query_range" {
		t.Fatalf("unexpected request path: %s", receivedRequest.URL.Path)
	}
	if receivedRequest.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected content type: %s", receivedRequest.Header.Get("Content-Type"))
	}
	if receivedRequest.Header.Get("Authorization") != "Bearer test-token" {
		t.Fatal("custom request header was not sent")
	}
	if receivedQuery != widget.Query {
		t.Fatalf("query was not preserved: %q", receivedQuery)
	}
	if receivedStart != formatPrometheusTimestamp(start) || receivedEnd != formatPrometheusTimestamp(now) {
		t.Fatalf("unexpected query bounds: %q, %q", receivedStart, receivedEnd)
	}
	if receivedStep != "720s" {
		t.Fatalf("unexpected query step: %q", receivedStep)
	}
	if graph.LatestValue != "2" {
		t.Fatalf("expected latest value, got %q", graph.LatestValue)
	}
	if graph.MinimumValue != "1" || graph.MaximumValue != "2" {
		t.Fatalf("unexpected graph bounds: %q, %q", graph.MinimumValue, graph.MaximumValue)
	}
	if graph.StartTimeLabel != "1d ago" || graph.EndTimeLabel != "now" {
		t.Fatalf("unexpected time labels: %q, %q", graph.StartTimeLabel, graph.EndTimeLabel)
	}
	if !strings.Contains(graph.Polyline, "0.00,174.00") || !strings.Contains(graph.Polyline, "1000.00,6.00") {
		t.Fatalf("unexpected graph polyline: %s", graph.Polyline)
	}
	if len(graph.Points) != 2 {
		t.Fatalf("unexpected number of interactive points: %d", len(graph.Points))
	}
	if graph.Points[0].FormattedValue != "1" || graph.Points[1].FormattedValue != "2" {
		t.Fatalf("unexpected interactive point values: %+v", graph.Points)
	}
	if strings.Join(annotations, "|") != "partial data|query hint" {
		t.Fatalf("unexpected annotations: %v", annotations)
	}
}

func TestFetchPrometheusGraphAllowsInsecureTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{"resultType":"matrix","result":[{"values":[[1,"1"],[2,"2"]]}]}
		}`))
	}))
	defer server.Close()

	widget := &prometheusWidget{
		Server:        server.URL,
		Query:         "up",
		AllowInsecure: true,
	}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned an error: %v", err)
	}

	if _, _, err := fetchPrometheusGraph(context.Background(), widget, time.Unix(100, 0)); err != nil {
		t.Fatalf("fetch over insecure TLS returned an error: %v", err)
	}
}

func TestFetchPrometheusGraphErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		errorText  string
	}{
		{
			name:       "prometheus error",
			statusCode: http.StatusUnprocessableEntity,
			body:       `{"status":"error","errorType":"execution","error":"bad query"}`,
			errorText:  "execution: bad query",
		},
		{
			name:       "invalid JSON",
			statusCode: http.StatusOK,
			body:       `{`,
			errorText:  "decoding Prometheus response",
		},
		{
			name:       "empty result",
			statusCode: http.StatusOK,
			body:       `{"status":"success","data":{"resultType":"matrix","result":[]}}`,
			errorText:  "returned no series",
		},
		{
			name:       "wrong result type",
			statusCode: http.StatusOK,
			body:       `{"status":"success","data":{"resultType":"vector","result":[]}}`,
			errorText:  "unexpected Prometheus result type",
		},
		{
			name:       "insufficient finite values",
			statusCode: http.StatusOK,
			body:       `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1,"NaN"],[2,"+Inf"],[3,"1"],[4,2]]}]}}`,
			errorText:  "fewer than two finite samples",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			widget := &prometheusWidget{Server: server.URL, Query: "up"}
			if err := widget.initialize(); err != nil {
				t.Fatalf("initialize returned an error: %v", err)
			}

			_, _, err := fetchPrometheusGraph(context.Background(), widget, time.Unix(100, 0))
			if err == nil || !strings.Contains(err.Error(), test.errorText) {
				t.Fatalf("expected error containing %q, got %v", test.errorText, err)
			}
		})
	}
}

func TestFetchPrometheusGraphHonorsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	widget := &prometheusWidget{Server: server.URL, Query: "up"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned an error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := fetchPrometheusGraph(ctx, widget, time.Now())
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestPrometheusGraphPointsFlatSeries(t *testing.T) {
	polyline, points := prometheusGraphCoordinates([]prometheusSample{
		{Timestamp: 0, Value: 5},
		{Timestamp: 10, Value: 5},
	}, 0, 10, 5, 5, "number")

	if polyline != "0.00,90.00 1000.00,90.00" {
		t.Fatalf("unexpected flat graph polyline: %s", polyline)
	}
	if len(points) != 2 || points[0].YPercent != 50 || points[1].YPercent != 50 {
		t.Fatalf("unexpected interactive points: %+v", points)
	}
	if points[0].HitLeft != 0 || points[0].HitWidth != 50 || points[0].HitPointOffset != 0 {
		t.Fatalf("unexpected first point hit area: %+v", points[0])
	}
	if points[1].HitLeft != 50 || points[1].HitWidth != 50 || points[1].HitPointOffset != 100 {
		t.Fatalf("unexpected last point hit area: %+v", points[1])
	}
}

func TestFormatPrometheusValue(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		unit     string
		expected string
	}{
		{name: "number", value: 1234.5, unit: "number", expected: "1234.5"},
		{name: "compact below boundary", value: 999, unit: "compact-number", expected: "999.00"},
		{name: "compact thousands", value: 1500, unit: "compact-number", expected: "1.50k"},
		{name: "compact negative millions", value: -2500000, unit: "compact-number", expected: "-2.50M"},
		{name: "bytes zero", value: 0, unit: "bytes", expected: "0.00 B"},
		{name: "bytes kibibytes", value: 1536, unit: "bytes", expected: "1.50 KiB"},
		{name: "duration milliseconds", value: 0.5, unit: "duration", expected: "500.00 ms"},
		{name: "duration minutes", value: 90, unit: "duration", expected: "1.50 m"},
		{name: "duration days", value: 172800, unit: "duration", expected: "2.00 d"},
		{name: "percent no scaling", value: 0.5, unit: "percent", expected: "0.50%"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := formatPrometheusValue(test.value, test.unit); actual != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}

func TestPrometheusWidgetRender(t *testing.T) {
	widget := &prometheusWidget{
		widgetBase: widgetBase{
			Type:             "prometheus",
			Title:            "Request Rate",
			ContentAvailable: true,
		},
		Link:           "https://grafana.example.com/d/test?a=1&b=2",
		ShowValue:      true,
		ShowScale:      true,
		ShowTimeLabels: true,
		Graph: &prometheusGraph{
			Polyline: "0.00,90.00 1000.00,90.00",
			Points: []prometheusGraphPoint{
				{
					X:              0,
					Y:              90,
					YPercent:       50,
					HitLeft:        0,
					HitWidth:       50,
					HitPointOffset: 0,
					FormattedValue: "1.00k",
					FormattedTime:  "12:00",
					TooltipLeft:    true,
				},
				{
					X:              1000,
					Y:              90,
					YPercent:       50,
					HitLeft:        50,
					HitWidth:       50,
					HitPointOffset: 100,
					FormattedValue: "2.00k",
					FormattedTime:  "13:00",
					TooltipRight:   true,
				},
			},
			LatestValue:    "2.00k",
			MinimumValue:   "1.00k",
			MaximumValue:   "2.00k",
			StartTimeLabel: "1d ago",
			EndTimeLabel:   "now",
		},
	}

	rendered := string(widget.Render())
	for _, expected := range []string{
		`href="https://grafana.example.com/d/test?a=1&amp;b=2"`,
		`target="_blank"`,
		`rel="noreferrer"`,
		`aria-label="Open Request Rate graph"`,
		`2.00k`,
		`1.00k`,
		`12:00`,
		`1d ago`,
		`prometheus-scale`,
		`prometheus-point-marker`,
		`prometheus-hover-point`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget does not contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "ZgotmplZ") {
		t.Fatalf("rendered widget contains unsafe template output:\n%s", rendered)
	}

	widget.Link = ""
	widget.ShowValue = false
	widget.ShowScale = false
	widget.ShowTimeLabels = false
	rendered = string(widget.Render())
	for _, unexpected := range []string{`<a class="prometheus-chart-link"`, `prometheus-current-value`, `prometheus-scale`, `prometheus-time-labels`} {
		if strings.Contains(rendered, unexpected) {
			t.Fatalf("rendered widget unexpectedly contains %q:\n%s", unexpected, rendered)
		}
	}
}

func TestFetchPrometheusGraphRejectsMultipleSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"a"},"values":[[1,"1"],[2,"2"]]},{"metric":{"job":"b"},"values":[[1,"3"],[2,"4"]]}]}}`)
	}))
	defer server.Close()

	widget := &prometheusWidget{Server: server.URL, Query: "up"}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	_, _, err := fetchPrometheusGraph(context.Background(), widget, time.Now())
	if err == nil || !strings.Contains(err.Error(), "must return exactly one series") {
		t.Fatalf("error = %v, want single-series validation", err)
	}
}
