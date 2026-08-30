package glance

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestComprehensiveHSLColorField(t *testing.T) {
	tests := []struct {
		name, input         string
		wantH, wantS, wantL float64
		wantErr             bool
	}{
		{"space", "120 50 25", 120, 50, 25, false}, {"comma", "hsl(240, 100%, 50%)", 240, 100, 50, false},
		{"hsla accepted", "hsla(360, 0%, 100%)", 360, 0, 100, false}, {"bad format", "red", 0, 0, 0, true},
		{"hue high", "361 1 1", 0, 0, 0, true}, {"sat high", "1 101 1", 0, 0, 0, true}, {"light high", "1 1 101", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got hslColorField
			err := yaml.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.H != tt.wantH || got.S != tt.wantS || got.L != tt.wantL {
				t.Fatalf("got %#v", got)
			}
			if got.String() == "" || got.ToHex() == "" {
				t.Fatal("expected string and hex output")
			}
		})
	}
	var nil1, nil2 *hslColorField
	if !nil1.SameAs(nil2) {
		t.Fatal("nil colors should match")
	}
	c1 := &hslColorField{H: 1, S: 2, L: 3}
	c2 := &hslColorField{H: 1, S: 2, L: 3}
	if !c1.SameAs(c2) || c1.SameAs(nil) {
		t.Fatal("SameAs mismatch")
	}
	c2.L = 4
	if c1.SameAs(c2) {
		t.Fatal("different colors matched")
	}
}

func TestComprehensiveDurationField(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"5s", 5 * time.Second, false}, {"7m", 7 * time.Minute, false}, {"2h", 2 * time.Hour, false}, {"3d", 72 * time.Hour, false},
		{"1ms", 0, true}, {"-1h", 0, true}, {"hour", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d durationField
			err := yaml.Unmarshal([]byte(tt.input), &d)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if time.Duration(d) != tt.want {
				t.Fatalf("got %s want %s", time.Duration(d), tt.want)
			}
		})
	}
}

func TestComprehensiveCustomIconField(t *testing.T) {
	tests := []struct {
		input, contains string
		invert          bool
	}{
		{"https://example.invalid/icon.svg", "example.invalid", false}, {"auto-invert /icon.png", "/icon.png", true},
		{"si:github", "simple-icons", true}, {"di:plex.png", "dashboard-icons/png/plex.png", false},
		{"di:plex.bad", "dashboard-icons/svg/plex.svg", false}, {"mdi:home", "@mdi/svg", true}, {"sh:glance.png", "selfhst/icons/png/glance.png", false},
		{"unknown:value", "unknown:value", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := newCustomIconField(tt.input)
			if !strings.Contains(string(got.URL), tt.contains) {
				t.Fatalf("URL=%q", got.URL)
			}
			if got.AutoInvert != tt.invert {
				t.Fatalf("invert=%v", got.AutoInvert)
			}
		})
	}
	var got customIconField
	if err := yaml.Unmarshal([]byte("si:github"), &got); err != nil {
		t.Fatal(err)
	}
	if !got.AutoInvert {
		t.Fatal("yaml icon should auto invert")
	}
}

func TestComprehensiveProxyOptionsField(t *testing.T) {
	var empty proxyOptionsField
	if err := yaml.Unmarshal([]byte("{}"), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.client != nil {
		t.Fatal("empty proxy should not create client")
	}

	var scalar proxyOptionsField
	if err := yaml.Unmarshal([]byte("http://proxy.example.invalid:8080"), &scalar); err != nil {
		t.Fatal(err)
	}
	if scalar.client == nil || scalar.client.Timeout != defaultClientTimeout {
		t.Fatal("scalar proxy client defaults not set")
	}
	tr, ok := scalar.client.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatal("proxy transport not configured")
	}

	var mapping proxyOptionsField
	if err := yaml.Unmarshal([]byte("url: https://proxy.example.invalid\nallow-insecure: true\ntimeout: 9s\n"), &mapping); err != nil {
		t.Fatal(err)
	}
	if mapping.client == nil || mapping.client.Timeout != 9*time.Second {
		t.Fatalf("timeout=%v", mapping.client)
	}
	tr = mapping.client.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("allow-insecure not applied")
	}
}

func TestComprehensiveQueryParametersField(t *testing.T) {
	var q queryParametersField
	input := "s: text\ni: 42\nf: 1.5\nb: true\nlist: [one, 2, false]\n"
	if err := yaml.Unmarshal([]byte(input), &q); err != nil {
		t.Fatal(err)
	}
	query := q.toQueryString()
	for _, part := range []string{"s=text", "i=42", "f=1.5", "b=true", "list=one", "list=2", "list=false"} {
		if !strings.Contains(query, part) {
			t.Fatalf("query %q missing %q", query, part)
		}
	}
	var bad queryParametersField
	if err := yaml.Unmarshal([]byte("bad:\n  nested: value\n"), &bad); err == nil {
		t.Fatal("expected invalid nested query value error")
	}
}
