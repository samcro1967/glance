package glance

import (
	"context"
	"html/template"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestPriorityCustomAPIOptionsTypedDefaultsAndJSON(t *testing.T) {
	options := customAPIOptions{
		"string": "value",
		"int":    7,
		"float":  2.5,
		"bool":   true,
		"object": map[string]any{"name": "glance"},
	}

	if got := options.StringOr("string", "fallback"); got != "value" {
		t.Fatalf("StringOr = %q", got)
	}
	if got := options.StringOr("int", "fallback"); got != "fallback" {
		t.Fatalf("StringOr type mismatch = %q", got)
	}
	if got := options.IntOr("int", 9); got != 7 {
		t.Fatalf("IntOr = %d", got)
	}
	if got := options.FloatOr("float", 9); got != 2.5 {
		t.Fatalf("FloatOr = %v", got)
	}
	if got := options.BoolOr("bool", false); !got {
		t.Fatal("BoolOr = false, want true")
	}
	if got := options.JSON("object"); got != `{"name":"glance"}` {
		t.Fatalf("JSON = %q", got)
	}
}

func TestPriorityCustomAPIOptionsJSONMissingKeyPanics(t *testing.T) {
	options := customAPIOptions{}
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("JSON missing key did not panic")
		}
	}()
	_ = options.JSON("missing")
}

func TestPriorityCustomAPIRequestInitializeBodyModes(t *testing.T) {
	tests := []struct {
		name        string
		req         CustomAPIRequest
		wantMethod  string
		wantType    string
		wantErrText string
	}{
		{name: "default get", req: CustomAPIRequest{URL: "https://example.com"}, wantMethod: http.MethodGet},
		{name: "default json post", req: CustomAPIRequest{URL: "https://example.com", Body: map[string]any{"ok": true}}, wantMethod: http.MethodPost, wantType: "application/json"},
		{name: "string post", req: CustomAPIRequest{URL: "https://example.com", BodyType: "string", Body: "payload"}, wantMethod: http.MethodPost},
		{name: "invalid body type", req: CustomAPIRequest{URL: "https://example.com", BodyType: "xml", Body: "payload"}, wantErrText: "invalid body type"},
		{name: "string requires string", req: CustomAPIRequest{URL: "https://example.com", BodyType: "string", Body: 123}, wantErrText: "body must be a string"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.initialize()
			if tc.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("initialize error = %v", err)
			}
			if tc.req.httpRequest.Method != tc.wantMethod {
				t.Fatalf("method = %q, want %q", tc.req.httpRequest.Method, tc.wantMethod)
			}
			if got := tc.req.httpRequest.Header.Get("Content-Type"); got != tc.wantType {
				t.Fatalf("Content-Type = %q, want %q", got, tc.wantType)
			}
		})
	}
}

func TestPriorityDecoratedGJSONResultAccessors(t *testing.T) {
	result := decoratedGJSONResult{gjson.Parse(`{"name":"glance","count":3,"ratio":1.5,"ok":true,"items":[{"id":1},{"id":2}]}`)}

	if !result.Exists("name") {
		t.Fatal("name should exist")
	}
	if result.Exists("missing") {
		t.Fatal("missing should not exist")
	}
	if got := result.String("name"); got != "glance" {
		t.Fatalf("String = %q", got)
	}
	if got := result.Int("count"); got != 3 {
		t.Fatalf("Int = %d", got)
	}
	if got := result.Float("ratio"); got != 1.5 {
		t.Fatalf("Float = %v", got)
	}
	if got := result.Bool("ok"); !got {
		t.Fatal("Bool = false")
	}
	if got := result.Get("name").String(""); got != "glance" {
		t.Fatalf("Get/String = %q", got)
	}
	items := result.Array("items")
	if len(items) != 2 || items[1].Int("id") != 2 {
		t.Fatalf("Array = %#v", items)
	}
}

func TestPriorityCustomAPIMathOperations(t *testing.T) {
	if got := customAPIDoMathOp(6, 2, "add"); got != 8 {
		t.Fatalf("add = %d", got)
	}
	if got := customAPIDoMathOp(6, 2, "sub"); got != 4 {
		t.Fatalf("sub = %d", got)
	}
	if got := customAPIDoMathOp(6, 2, "mul"); got != 12 {
		t.Fatalf("mul = %d", got)
	}
	if got := customAPIDoMathOp(6, 2, "div"); got != 3 {
		t.Fatalf("div = %d", got)
	}
	if got := customAPIDoMathOp(6, 0, "div"); got != 0 {
		t.Fatalf("division by zero = %d", got)
	}
	if got := customAPIDoMathOp(6, 2, "unknown"); got != 0 {
		t.Fatalf("unknown op = %d", got)
	}

	add := customAPITemplateFuncs["add"].(func(any, any) any)
	if got := add("bad", 2).(float64); !math.IsNaN(got) {
		t.Fatalf("invalid mixed math = %v, want NaN", got)
	}
}

func TestPriorityCustomAPIJSONLinesAndSubrequest(t *testing.T) {
	data := customAPITemplateData{
		customAPIResponseData: &customAPIResponseData{JSON: decoratedGJSONResult{gjson.Parse("{\"id\":1}\n{\"id\":2}\n")}},
		subrequests:           map[string]*customAPIResponseData{"secondary": {JSON: decoratedGJSONResult{gjson.Parse(`{"ok":true}`)}}},
	}

	lines := data.JSONLines()
	if len(lines) != 2 || lines[0].Int("id") != 1 || lines[1].Int("id") != 2 {
		t.Fatalf("JSONLines = %#v", lines)
	}
	if !data.Subrequest("secondary").JSON.Bool("ok") {
		t.Fatal("subrequest data not returned")
	}
}

func TestPriorityCustomAPIFetchWhitespaceAndInvalidJSON(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
		skip       bool
		wantErr    string
	}{
		{name: "empty success", body: "   \n", status: http.StatusOK},
		{name: "invalid success json", body: "not-json", status: http.StatusOK, wantErr: "invalid response JSON"},
		{name: "skip validation", body: "not-json", status: http.StatusOK, skip: true},
		{name: "invalid error json classified by status", body: "not-json", status: http.StatusBadGateway, wantErr: "502 Bad Gateway"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			req := &CustomAPIRequest{URL: server.URL, SkipJSONValidation: tc.skip}
			if err := req.initialize(); err != nil {
				t.Fatal(err)
			}
			data, err := fetchCustomAPIResponse(context.Background(), req)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if data == nil || data.Response.StatusCode != tc.status {
				t.Fatalf("data = %#v", data)
			}
		})
	}
}

func TestPriorityCustomAPITimeHelpers(t *testing.T) {
	value := time.Date(2026, 8, 30, 13, 45, 0, 0, time.UTC)
	if got := customAPIFuncFormatTime("2006-01-02 15:04", value); got != "2026-08-30 13:45" {
		t.Fatalf("format = %q", got)
	}
	parsed := customAPIFuncParseTimeInLocation("2006-01-02 15:04", "2026-08-30 13:45", time.UTC)
	if !parsed.Equal(value) {
		t.Fatalf("parsed = %v, want %v", parsed, value)
	}
}

func TestPriorityCustomAPITemplateSubrequestMissingReturnsExecutionError(t *testing.T) {
	tmpl := template.Must(template.New("priority").Parse(`{{ (.Subrequest "missing").JSON.String "name" }}`))
	_, err := fetchAndRenderCustomAPIRequest(context.Background(), nil, nil, nil, tmpl)
	if err == nil || !strings.Contains(err.Error(), `subrequest with key "missing" has not been defined`) {
		t.Fatalf("error = %v", err)
	}
}
