package glance

import (
	"html/template"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestComprehensivePercentChange(t *testing.T) {
	tests := []struct{ current, previous, want float64 }{{0, 0, 0}, {10, 0, 100}, {110, 100, 10}, {50, 100, -50}, {100, 100, 0}}
	for _, tt := range tests {
		if got := percentChange(tt.current, tt.previous); math.Abs(got-tt.want) > 0.0001 {
			t.Fatalf("percentChange(%v,%v)=%v want %v", tt.current, tt.previous, got, tt.want)
		}
	}
}

func TestComprehensiveURLAndStringUtilities(t *testing.T) {
	if got := extractDomainFromUrl("HTTPS://WWW.Example.COM/path"); got != "example.com" {
		t.Fatalf("domain=%q", got)
	}
	if extractDomainFromUrl("") != "" {
		t.Fatal("empty URL should produce empty domain")
	}
	if stripURLScheme("https://example.com") != "example.com" || stripURLScheme("HTTP://example.com") != "HTTP://example.com" {
		t.Fatal("stripURLScheme behavior changed")
	}
	if prefixStringLines("> ", "a\nb") != "> a\n> b" {
		t.Fatal("prefixStringLines mismatch")
	}
	if titleToSlug("  Hello   World  ") != "hello-world" {
		t.Fatal("titleToSlug mismatch")
	}
	if normalizeVersionFormat(" 1.2.3 ") != "v1.2.3" || normalizeVersionFormat("V2.0") != "v2.0" || normalizeVersionFormat("") != "" {
		t.Fatal("normalizeVersionFormat mismatch")
	}
	if !stringToBool("true") || !stringToBool("yes") || stringToBool("TRUE") || stringToBool("1") {
		t.Fatal("stringToBool mismatch")
	}
}

func TestComprehensiveSliceAndLengthUtilities(t *testing.T) {
	original := []int{1, 2, 3}
	got := maybeCopySliceWithoutZeroValues(original)
	if len(got) != 3 || &got[0] != &original[0] {
		t.Fatal("zero-free slice should be returned unchanged")
	}
	got = maybeCopySliceWithoutZeroValues([]int{0, 1, 0, 2})
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("filtered=%v", got)
	}
	empty := maybeCopySliceWithoutZeroValues([]float64{})
	if len(empty) != 0 {
		t.Fatal("empty slice changed")
	}
	if v, truncated := limitStringLength("abcdef", 3); v != "abc" || !truncated {
		t.Fatal("ASCII truncation failed")
	}
	if v, truncated := limitStringLength("éclair", 2); v != "éc" || !truncated {
		t.Fatalf("rune truncation=%q", v)
	}
	if v, truncated := limitStringLength("abc", 3); v != "abc" || truncated {
		t.Fatal("equal length should not truncate")
	}
	if itemAtIndexOrDefault([]string{"a"}, 0, "x") != "a" || itemAtIndexOrDefault([]string{"a"}, 2, "x") != "x" {
		t.Fatal("itemAtIndexOrDefault mismatch")
	}
	if ternary(true, "a", "b") != "a" || ternary(false, "a", "b") != "b" {
		t.Fatal("ternary mismatch")
	}
}

func TestComprehensivePolylineAndColorUtilities(t *testing.T) {
	if svgPolylineCoordsFromYValues(100, 50, nil) != "" || svgPolylineCoordsFromYValues(100, 50, []float64{1}) != "" {
		t.Fatal("short polyline should be empty")
	}
	coords := svgPolylineCoordsFromYValues(100, 50, []float64{0, 5, 10})
	if !strings.Contains(coords, "0.00,") || !strings.Contains(coords, "50.00,") || !strings.Contains(coords, "100.00,") {
		t.Fatalf("coords=%q", coords)
	}
	tests := map[string][3]float64{"#000000": {0, 0, 0}, "#ffffff": {0, 0, 100}, "#ff0000": {0, 100, 50}, "#00ff00": {120, 100, 50}, "#0000ff": {240, 100, 50}}
	for want, v := range tests {
		if got := hslToHex(v[0], v[1], v[2]); got != want {
			t.Fatalf("hsl %v = %s want %s", v, got, want)
		}
	}
}

func TestComprehensiveTemplateUtilities(t *testing.T) {
	tests := map[int]string{0: "0", 999: "999", 1000: "1.0k", 9999: "10.0k", 10000: "10k", 999999: "999k", 1000000: "1.0m", 2500000: "2.5m"}
	for in, want := range tests {
		if got := formatApproxNumber(in); got != want {
			t.Fatalf("formatApproxNumber(%d)=%q want %q", in, got, want)
		}
	}
	tm := time.Unix(12345, 0)
	if got := string(dynamicRelativeTimeAttrs(tm)); got != `data-dynamic-relative-time="12345"` {
		t.Fatalf("attrs=%q", got)
	}
	tpl := template.Must(template.New("x").Parse("Hello {{.}}"))
	if got, err := executeTemplateToString(tpl, "World"); err != nil || got != "Hello World" {
		t.Fatalf("template got=%q err=%v", got, err)
	}
	bad := template.Must(template.New("bad").Parse("{{.Missing}}"))
	if _, err := executeTemplateToString(bad, struct{}{}); err == nil {
		t.Fatal("expected execution error")
	}
}

func TestComprehensiveFileServerWithCache(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	h := fileServerWithCache(http.Dir(dir), 90*time.Second)
	req := httptest.NewRequest(http.MethodGet, "/hello.txt", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "hello" || rr.Header().Get("Cache-Control") != "public, max-age=90" {
		t.Fatalf("status=%d body=%q cache=%q", rr.Code, rr.Body.String(), rr.Header().Get("Cache-Control"))
	}
}

func TestComprehensiveParseRFC3339Time(t *testing.T) {
	want := time.Date(2026, 8, 30, 12, 34, 56, 0, time.UTC)
	if got := parseRFC3339Time("2026-08-30T12:34:56Z"); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	before := time.Now().Add(-time.Second)
	got := parseRFC3339Time("bad")
	after := time.Now().Add(time.Second)
	if got.Before(before) || got.After(after) {
		t.Fatalf("invalid parse fallback=%v", got)
	}
}
