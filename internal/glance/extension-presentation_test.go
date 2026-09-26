package glance

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newExtensionTestServer(t *testing.T, headers map[string]string, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, value := range headers {
			w.Header().Set(key, value)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRenderExtensionPresentationValidBlocks(t *testing.T) {
	content := []byte(`{"blocks":[
        {"type":"text","text":"<b>escaped</b>","style":"secondary"},
        {"type":"metrics","items":[{"label":"Requests","value":"12,402"}]},
        {"type":"status","text":"Healthy","variant":"positive"},
        {"type":"key-values","items":[{"label":"Version","value":"1.2.3"}]},
        {"type":"progress","label":"Storage","value":67,"text":"67%"},
        {"type":"state","text":"Partial data","variant":"degraded"},
        {"type":"list","items":[{"title":"Service","meta":"42 ms","url":"/service"}]},
        {"type":"cards","items":[{"title":"API","text":"Primary","status":{"text":"Healthy","variant":"positive"}}]},
        {"type":"table","columns":[{"key":"service","label":"Service"},{"key":"latency","label":"Latency","type":"number","priority":1}],"rows":[{"service":"API","latency":42}]},
        {"type":"chart","chart-type":"line","data":{"labels":["Mon","Tue"],"series":[{"label":"CPU","values":[42,55]}]}}
    ]}`)
	rendered, err := renderExtensionPresentation(content)
	if err != nil {
		t.Fatalf("renderExtensionPresentation() error = %v", err)
	}
	html := string(rendered)
	for _, expected := range []string{"&lt;b&gt;escaped&lt;/b&gt;", "glance-metric", "glance-status-positive", "glance-key-values", "glance-progress", "glance-state-degraded", "glance-list", "glance-card", "data-glance-table=\"extension-table-8\"", "data-glance-chart=\"extension-chart-9\"", "data-glance-presentation-config"} {
		if !strings.Contains(html, expected) {
			t.Errorf("rendered presentation missing %q: %s", expected, html)
		}
	}
	if strings.Contains(html, "<b>escaped</b>") {
		t.Fatal("presentation text was not escaped")
	}
}

func TestRenderExtensionPresentationRequiresBlocks(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{}`))
	if err == nil {
		t.Fatal("expected missing blocks to be rejected")
	}
}

func TestRenderExtensionPresentationRejectsUnknownEnvelopeField(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{"blocks":[],"extra":true}`))
	if err == nil {
		t.Fatal("expected unknown envelope field to be rejected")
	}
}

func TestRenderExtensionPresentationRejectsUnknownBlockField(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{"blocks":[{"type":"badge","text":"x","extra":true}]}`))
	if err == nil {
		t.Fatal("expected unknown block field to be rejected")
	}
}

func TestRenderExtensionPresentationRejectsUnknownBlockType(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{"blocks":[{"type":"button","text":"x"}]}`))
	if err == nil {
		t.Fatal("expected unknown block type to be rejected")
	}
}

func TestRenderExtensionPresentationRejectsInvalidProgress(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{"blocks":[{"type":"progress","value":101}]}`))
	if err == nil {
		t.Fatal("expected invalid progress to be rejected")
	}
}

func TestRenderExtensionPresentationRejectsUnsafeListURL(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{"blocks":[{"type":"list","items":[{"title":"Bad","url":"javascript:alert(1)"}]}]}`))
	if err == nil {
		t.Fatal("expected unsafe list URL to be rejected")
	}
}

func TestRenderExtensionPresentationRejectsUnknownTableRowKey(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{"blocks":[{"type":"table","columns":[{"key":"service","label":"Service"}],"rows":[{"service":"API","extra":"x"}]}]}`))
	if err == nil {
		t.Fatal("expected unknown table row key to be rejected")
	}
}

func TestRenderExtensionPresentationRejectsInvalidChartType(t *testing.T) {
	_, err := renderExtensionPresentation([]byte(`{"blocks":[{"type":"chart","chart-type":"scatter","data":{}}]}`))
	if err == nil {
		t.Fatal("expected invalid chart type to be rejected")
	}
}

func TestFetchExtensionPresentationV1DoesNotRequireHTMLTrust(t *testing.T) {
	server := newExtensionTestServer(t, map[string]string{extensionHeaderContentType: "presentation-v1"}, `{"blocks":[{"type":"badge","text":"Native"}]}`)
	result, err := fetchExtension(t.Context(), extensionRequestOptions{URL: server.URL, AllowHtml: false})
	if err != nil {
		t.Fatalf("fetchExtension() error = %v", err)
	}
	if !strings.Contains(string(result.Content), "glance-badge") {
		t.Fatalf("presentation content was not rendered: %s", result.Content)
	}
}

func TestFetchExtensionMalformedPresentationPreservesNoContentIdentity(t *testing.T) {
	server := newExtensionTestServer(t, map[string]string{extensionHeaderContentType: "presentation-v1"}, `{"blocks":[{"type":"progress","value":200}]}`)
	_, err := fetchExtension(t.Context(), extensionRequestOptions{URL: server.URL})
	if err == nil {
		t.Fatal("expected malformed presentation to fail")
	}
	if !errors.Is(err, errNoContent) {
		t.Fatalf("errNoContent identity was not preserved: %v", err)
	}
}
