package glance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const startupTestConfig = "server:\n  port: 18081\n  base-url: /dashboard\npages:\n  - name: Home\n    columns:\n      - size: full\n"

func TestStartupConfigDiagnostic(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		file    string
		line    int
		summary string
	}{
		{"missing include", fmt.Errorf("include failed: %w", &os.PathError{Op: "open", Path: "/config/page.yml", Err: fs.ErrNotExist}), "/config/page.yml", 0, "not found"},
		{"permission", &os.PathError{Op: "open", Path: "/config/glance.yml", Err: fs.ErrPermission}, "/config/glance.yml", 0, "Permission denied"},
		{"source diagnostic", fmt.Errorf("invalid: %w", &configDiagnostic{File: "/config/page.yml", Line: 17, Message: "SECRET_VALUE"}), "/config/page.yml", 17, "Invalid YAML"},
		{"unlocated", errors.New("SECRET_VALUE"), "/config/glance.yml", 0, "could not be loaded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := startupConfigDiagnostic("/config/glance.yml", test.err)
			if got.File != test.file || got.Line != test.line || !strings.Contains(got.Summary, test.summary) {
				t.Fatalf("unexpected diagnostic: %+v", got)
			}
			if strings.Contains(got.Summary, "SECRET_VALUE") {
				t.Fatal("diagnostic exposed a configuration value")
			}
		})
	}
}

func TestStartupConfigPage(t *testing.T) {
	oldREADME := docsREADME
	docsREADME = []byte("# Installation\n\nCreate config/glance.yml.\n")
	t.Cleanup(func() { docsREADME = oldREADME })
	page := &startupConfigPage{
		configPath: "/config/glance.yml",
		issue:      startupConfigIssue{File: "<script>alert(1)</script>.yml", Line: 17, Summary: "Invalid YAML"},
	}
	for _, test := range []struct {
		method   string
		url      string
		status   int
		contains string
	}{
		{"GET", "/", 503, "Line: <strong>17</strong>"},
		{"HEAD", "/", 503, ""},
		{"GET", "/docs/README", 200, "Create config/glance.yml"},
		{"GET", "/docs/configuration", 200, "Configuring Glance"},
		{"GET", "/docs/assets/glance.yml", 200, "pages:"},
		{"GET", "/docs/assets/../../glance.yml", 404, ""},
		{"GET", "/docs/assets/embed.go", 404, ""},
		{"GET", "/docs/missing", 404, ""},
		{"GET", "/api/config", 404, ""},
		{"POST", "/", 405, ""},
	} {
		t.Run(test.method+test.url, func(t *testing.T) {
			response := httptest.NewRecorder()
			page.ServeHTTP(response, httptest.NewRequest(test.method, test.url, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "<script>") {
				t.Fatal("unescaped diagnostic filename")
			}
			if test.method == "HEAD" && response.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing recovery response security headers")
			}
		})
	}
}

func TestStartupConfigLoad(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "glance.yml")
	if _, _, err := readStartupConfig(mainPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("pages:\n  $include: pages.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	includePath := filepath.Join(dir, "pages.yml")
	if err := os.WriteFile(includePath, []byte("- name: Home\n  columns: SECRET_VALUE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := readStartupConfig(mainPath)
	issue := startupConfigDiagnostic(mainPath, err)
	if err == nil || issue.File != includePath || issue.Line != 2 || strings.Contains(issue.Summary, "SECRET_VALUE") {
		t.Fatalf("include diagnostic = %+v, error = %v", issue, err)
	}
	if err := os.WriteFile(mainPath, []byte(startupTestConfig), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, candidate, err := loadStartupConfig(context.Background(), mainPath)
	if err != nil || parsed == nil || candidate == nil {
		t.Fatalf("valid configuration: %v", err)
	}
	if candidate.Server.Port != 18081 || candidate.Server.BaseURL != "/dashboard" {
		t.Fatal("configured server settings were lost")
	}
}

func TestStartupConfigRecovery(t *testing.T) {
	for _, cancelRecovery := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelRecovery), func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "missing-parent", "glance.yml")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			addresses := make(chan string, 1)
			type result struct {
				candidate *config
				err       error
			}
			results := make(chan result, 1)
			go func() {
				_, candidate, err := recoverStartupConfig(ctx, configPath, fs.ErrNotExist, 10*time.Millisecond, func(handler http.Handler) (*processServer, error) {
					server, err := newProcessServer("127.0.0.1", 0, handler)
					if err == nil {
						addresses <- server.listener.Addr().String()
					}
					return server, err
				})
				results <- result{candidate, err}
			}()
			var address string
			select {
			case address = <-addresses:
			case <-ctx.Done():
				t.Fatal("recovery listener did not start")
			}
			request, err := http.NewRequestWithContext(ctx, "GET", "http://"+address+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(response.Body)
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if readErr != nil || response.StatusCode != 503 || !strings.Contains(string(body), "not found") {
				t.Fatalf("recovery response: %d %s %v", response.StatusCode, body, readErr)
			}
			if cancelRecovery {
				cancel()
			} else {
				if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, []byte("pages:\n  - name: Home\n    columns: SECRET_VALUE\n"), 0600); err != nil {
					t.Fatal(err)
				}
				// A new validation failure must replace the missing-file message
				// while the same HTTP listener stays available.
				for {
					response, err := http.DefaultClient.Do(request.Clone(ctx))
					if err != nil {
						t.Fatal(err)
					}
					body, readErr := io.ReadAll(response.Body)
					if err := response.Body.Close(); err != nil {
						t.Fatal(err)
					}
					if readErr != nil || response.StatusCode != 503 || strings.Contains(string(body), "SECRET_VALUE") {
						t.Fatalf("updated recovery response: %d %s %v", response.StatusCode, body, readErr)
					}
					if strings.Contains(string(body), "Line: <strong>3</strong>") {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("recovery diagnostic did not update")
					case <-time.After(10 * time.Millisecond):
					}
				}
				if err := os.WriteFile(configPath, []byte(startupTestConfig), 0600); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case got := <-results:
				if cancelRecovery {
					if !errors.Is(got.err, context.Canceled) {
						t.Fatalf("cancel error = %v", got.err)
					}
				} else if got.err != nil || got.candidate == nil || got.candidate.Server.Port != 18081 {
					t.Fatalf("recovery result = %+v", got)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("recovery did not finish")
			}
			// Recovery must release its listener before normal startup can bind.
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("recovery listener was not released: %v", err)
			}
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStartupConfigRecoveryBindFailure(t *testing.T) {
	bindErr := errors.New("address already in use")
	_, _, err := recoverStartupConfig(context.Background(), "/config/glance.yml", fs.ErrNotExist, time.Second, func(http.Handler) (*processServer, error) {
		return nil, bindErr
	})
	if !errors.Is(err, bindErr) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("bind failure lost diagnostics: %v", err)
	}
}
