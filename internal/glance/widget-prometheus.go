package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	prometheusGraphWidth        = 1000.0
	prometheusGraphHeight       = 180.0
	prometheusGraphSampleTarget = 120
)

var prometheusWidgetTemplate = mustParseTemplate("prometheus.html", "widget-base.html")

type prometheusWidget struct {
	widgetBase `yaml:",inline"`

	Server         string            `yaml:"server"`
	Query          string            `yaml:"query"`
	Link           string            `yaml:"link"`
	Range          durationField     `yaml:"range"`
	Step           durationField     `yaml:"step"`
	Headers        map[string]string `yaml:"headers"`
	AllowInsecure  bool              `yaml:"allow-insecure"`
	Timeout        durationField     `yaml:"timeout"`
	Unit           string            `yaml:"unit"`
	ShowValueRaw   *bool             `yaml:"show-value"`
	ShowScale      bool              `yaml:"show-scale"`
	ShowTimeLabels bool              `yaml:"show-time-labels"`

	ShowValue  bool             `yaml:"-"`
	Endpoint   string           `yaml:"-"`
	QueryRange time.Duration    `yaml:"-"`
	QueryStep  time.Duration    `yaml:"-"`
	Graph      *prometheusGraph `yaml:"-"`
}

type prometheusGraph struct {
	Polyline       string
	Points         []prometheusGraphPoint
	LatestValue    string
	MinimumValue   string
	MaximumValue   string
	StartTimeLabel string
	EndTimeLabel   string
}

type prometheusGraphPoint struct {
	X              float64
	Y              float64
	YPercent       float64
	HitLeft        float64
	HitWidth       float64
	HitPointOffset float64
	FormattedValue string
	FormattedTime  string
	TooltipBelow   bool
	TooltipLeft    bool
	TooltipRight   bool
}

type prometheusSample struct {
	Timestamp float64
	Value     float64
}

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string   `json:"metric"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"result"`
	} `json:"data"`
	ErrorType string   `json:"errorType"`
	Error     string   `json:"error"`
	Warnings  []string `json:"warnings"`
	Infos     []string `json:"infos"`
}

func (widget *prometheusWidget) initialize() error {
	widget.withTitle("Prometheus").withCacheDuration(5 * time.Minute)

	if strings.TrimSpace(widget.Server) == "" {
		return errors.New("server is required")
	}

	if strings.TrimSpace(widget.Query) == "" {
		return errors.New("query is required")
	}

	serverURL, err := url.Parse(widget.Server)
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}

	if (serverURL.Scheme != "http" && serverURL.Scheme != "https") || serverURL.Host == "" {
		return errors.New("server must be an absolute HTTP or HTTPS URL")
	}

	if serverURL.RawQuery != "" || serverURL.Fragment != "" {
		return errors.New("server URL must not contain a query string or fragment")
	}

	widget.Endpoint = strings.TrimRight(widget.Server, "/") + "/api/v1/query_range"

	if widget.Range < 0 {
		return errors.New("range must be greater than zero")
	}
	if widget.Range == 0 {
		widget.QueryRange = 24 * time.Hour
	} else {
		widget.QueryRange = time.Duration(widget.Range)
	}

	if widget.Step < 0 {
		return errors.New("step must be greater than zero")
	}
	if widget.Step == 0 {
		widget.QueryStep = max(widget.QueryRange/prometheusGraphSampleTarget, time.Second)
	} else {
		widget.QueryStep = time.Duration(widget.Step)
	}

	if widget.Unit == "" {
		widget.Unit = "number"
	}

	switch widget.Unit {
	case "number", "compact-number", "bytes", "duration", "percent":
	default:
		return errors.New("unit must be one of: number, compact-number, bytes, duration, percent")
	}

	widget.ShowValue = widget.ShowValueRaw == nil || *widget.ShowValueRaw

	return nil
}

func (widget *prometheusWidget) update(ctx context.Context) {
	graph, annotations, err := fetchPrometheusGraph(ctx, widget, time.Now())
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Graph = graph

	if len(annotations) > 0 {
		widget.withNotice(errors.New(strings.Join(annotations, "; ")))
	}
}

func (widget *prometheusWidget) Render() template.HTML {
	return widget.renderTemplate(widget, prometheusWidgetTemplate)
}

func fetchPrometheusGraph(ctx context.Context, widget *prometheusWidget, now time.Time) (*prometheusGraph, []string, error) {
	start := now.Add(-widget.QueryRange)
	form := url.Values{
		"query": {widget.Query},
		"start": {formatPrometheusTimestamp(start)},
		"end":   {formatPrometheusTimestamp(now)},
		"step":  {formatPrometheusStep(widget.QueryStep)},
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, widget.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", glanceUserAgentString)
	for key, value := range widget.Headers {
		request.Header.Set(key, value)
	}

	client := newHTTPClient(widget.Timeout, widget.AllowInsecure)
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching Prometheus query: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := readHTTPResponseBody(response.Body, 2*1024*1024)
	if err != nil {
		return nil, nil, fmt.Errorf("reading Prometheus response: %w", err)
	}

	var decoded prometheusResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, nil, fmt.Errorf("Prometheus returned a non-JSON error response: %w", unexpectedHTTPStatusError(response))
		}
		return nil, nil, fmt.Errorf("decoding Prometheus response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 || decoded.Status != "success" {
		message := decoded.Error
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		if decoded.ErrorType != "" {
			message = decoded.ErrorType + ": " + message
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, nil, fmt.Errorf("Prometheus query failed: %s: %w", message, unexpectedHTTPStatusError(response))
		}
		return nil, nil, fmt.Errorf("Prometheus query failed: %s", message)
	}

	if decoded.Data.ResultType != "matrix" {
		return nil, nil, fmt.Errorf("unexpected Prometheus result type %q", decoded.Data.ResultType)
	}

	if len(decoded.Data.Result) == 0 {
		return nil, nil, errors.New("Prometheus query returned no series")
	}
	if len(decoded.Data.Result) != 1 {
		return nil, nil, fmt.Errorf("Prometheus query returned %d series; query must return exactly one series", len(decoded.Data.Result))
	}

	samples := make([]prometheusSample, 0, len(decoded.Data.Result[0].Values))
	for _, pair := range decoded.Data.Result[0].Values {
		if len(pair) != 2 {
			continue
		}

		var timestamp float64
		var valueString string
		if err := json.Unmarshal(pair[0], &timestamp); err != nil {
			continue
		}
		if err := json.Unmarshal(pair[1], &valueString); err != nil {
			continue
		}

		value, err := strconv.ParseFloat(valueString, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.IsNaN(timestamp) || math.IsInf(timestamp, 0) {
			continue
		}

		samples = append(samples, prometheusSample{Timestamp: timestamp, Value: value})
	}

	if len(samples) < 2 {
		return nil, nil, errors.New("Prometheus query returned fewer than two finite samples")
	}

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Timestamp < samples[j].Timestamp
	})

	minimum, maximum := samples[0].Value, samples[0].Value
	for _, sample := range samples[1:] {
		minimum = min(minimum, sample.Value)
		maximum = max(maximum, sample.Value)
	}

	polyline, points := prometheusGraphCoordinates(
		samples,
		float64(start.UnixNano())/1e9,
		float64(now.UnixNano())/1e9,
		minimum,
		maximum,
		widget.Unit,
	)
	graph := &prometheusGraph{
		Polyline:       polyline,
		Points:         points,
		LatestValue:    formatPrometheusValue(samples[len(samples)-1].Value, widget.Unit),
		MinimumValue:   formatPrometheusValue(minimum, widget.Unit),
		MaximumValue:   formatPrometheusValue(maximum, widget.Unit),
		StartTimeLabel: formatPrometheusRangeLabel(widget.QueryRange) + " ago",
		EndTimeLabel:   "now",
	}

	annotations := make([]string, 0, len(decoded.Warnings)+len(decoded.Infos))
	annotations = append(annotations, decoded.Warnings...)
	annotations = append(annotations, decoded.Infos...)

	return graph, annotations, nil
}

func formatPrometheusTimestamp(value time.Time) string {
	return strconv.FormatFloat(float64(value.UnixNano())/1e9, 'f', 3, 64)
}

func formatPrometheusStep(value time.Duration) string {
	if value%time.Second == 0 {
		return strconv.FormatInt(int64(value/time.Second), 10) + "s"
	}
	return strconv.FormatFloat(value.Seconds(), 'f', 3, 64)
}

func prometheusGraphCoordinates(
	samples []prometheusSample,
	start, end, minimum, maximum float64,
	unit string,
) (string, []prometheusGraphPoint) {
	polylinePoints := make([]string, 0, len(samples))
	graphPoints := make([]prometheusGraphPoint, 0, len(samples))
	timeSpan := end - start
	valueSpan := maximum - minimum
	const verticalPadding = 6.0
	usableHeight := prometheusGraphHeight - verticalPadding*2

	for _, sample := range samples {
		x := (sample.Timestamp - start) / timeSpan * prometheusGraphWidth
		x = max(0, min(prometheusGraphWidth, x))

		y := prometheusGraphHeight / 2
		if valueSpan != 0 {
			y = (maximum-sample.Value)/valueSpan*usableHeight + verticalPadding
		}

		polylinePoints = append(polylinePoints, fmt.Sprintf("%.2f,%.2f", x, y))
		graphPoints = append(graphPoints, prometheusGraphPoint{
			X:              x,
			Y:              y,
			YPercent:       y / prometheusGraphHeight * 100,
			FormattedValue: formatPrometheusValue(sample.Value, unit),
			FormattedTime:  time.Unix(int64(sample.Timestamp), int64(math.Mod(sample.Timestamp, 1)*1e9)).Format("15:04"),
			TooltipBelow:   y < prometheusGraphHeight*0.25,
			TooltipLeft:    x < prometheusGraphWidth*0.1,
			TooltipRight:   x > prometheusGraphWidth*0.9,
		})
	}

	for i := range graphPoints {
		xPercent := graphPoints[i].X / prometheusGraphWidth * 100
		left := 0.0
		right := 100.0

		if i > 0 {
			previousXPercent := graphPoints[i-1].X / prometheusGraphWidth * 100
			left = (previousXPercent + xPercent) / 2
		}
		if i < len(graphPoints)-1 {
			nextXPercent := graphPoints[i+1].X / prometheusGraphWidth * 100
			right = (xPercent + nextXPercent) / 2
		}

		graphPoints[i].HitLeft = left
		graphPoints[i].HitWidth = max(right-left, 0.01)
		graphPoints[i].HitPointOffset = (xPercent - left) / graphPoints[i].HitWidth * 100
	}

	return strings.Join(polylinePoints, " "), graphPoints
}

func formatPrometheusValue(value float64, unit string) string {
	switch unit {
	case "compact-number":
		return formatPrometheusScaled(value, []prometheusScale{
			{1e12, "T"}, {1e9, "B"}, {1e6, "M"}, {1e3, "k"},
		})
	case "bytes":
		if math.Abs(value) < 1 {
			return formatPrometheusDecimal(value) + " B"
		}
		return formatPrometheusScaled(value, []prometheusScale{
			{1 << 40, " TiB"}, {1 << 30, " GiB"}, {1 << 20, " MiB"}, {1 << 10, " KiB"}, {1, " B"},
		})
	case "duration":
		absolute := math.Abs(value)
		switch {
		case absolute < 1:
			return formatPrometheusDecimal(value*1000) + " ms"
		case absolute < 60:
			return formatPrometheusDecimal(value) + " s"
		case absolute < 3600:
			return formatPrometheusDecimal(value/60) + " m"
		case absolute < 86400:
			return formatPrometheusDecimal(value/3600) + " h"
		default:
			return formatPrometheusDecimal(value/86400) + " d"
		}
	case "percent":
		return formatPrometheusDecimal(value) + "%"
	default:
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
}

type prometheusScale struct {
	Divisor float64
	Suffix  string
}

func formatPrometheusScaled(value float64, scales []prometheusScale) string {
	absolute := math.Abs(value)
	for _, scale := range scales {
		if absolute >= scale.Divisor {
			return formatPrometheusDecimal(value/scale.Divisor) + scale.Suffix
		}
	}
	return formatPrometheusDecimal(value)
}

func formatPrometheusDecimal(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatPrometheusRangeLabel(value time.Duration) string {
	switch {
	case value%(24*time.Hour) == 0:
		return strconv.FormatInt(int64(value/(24*time.Hour)), 10) + "d"
	case value%time.Hour == 0:
		return strconv.FormatInt(int64(value/time.Hour), 10) + "h"
	case value%time.Minute == 0:
		return strconv.FormatInt(int64(value/time.Minute), 10) + "m"
	default:
		return strconv.FormatInt(int64(value/time.Second), 10) + "s"
	}
}

func (widget *prometheusWidget) setDefaultAllowInsecure(value bool) {
	widget.AllowInsecure = value
}

func (widget *prometheusWidget) setDefaultTimeout(value durationField) {
	widget.Timeout = value
}

func (widget *prometheusWidget) setDefaultHeaders(value map[string]string) {
	widget.Headers = mergeStringMaps(value, widget.Headers)
}
