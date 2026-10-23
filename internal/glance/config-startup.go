package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	glancedocs "github.com/samcro1967/glance/docs"
)

// Configuration recovery runs before an application or its authentication can
// exist. Only safe diagnostics and bundled documentation are exposed here.
// Never render err.Error(): YAML and semantic errors can contain secret values.
type startupConfigIssue struct {
	File    string
	Line    int
	Summary string
}

func startupConfigDiagnostic(configPath string, err error) startupConfigIssue {
	issue := startupConfigIssue{File: configPath, Summary: "The configuration could not be loaded. Check the container logs for details."}
	var diagnostic *configDiagnostic
	if errors.As(err, &diagnostic) {
		if diagnostic.File != "" {
			issue.File = diagnostic.File
		}
		issue.Line = diagnostic.Line
		issue.Summary = "Invalid YAML or configuration value at this location. Check indentation, value types, required settings, and the container logs."
	}
	var fileError *os.PathError
	if errors.As(err, &fileError) {
		issue.File = fileError.Path
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		issue.Summary = "A configuration file was not found. Create it or correct the volume mount. Check included files too."
	case errors.Is(err, fs.ErrPermission):
		issue.Summary = "Permission denied reading the configuration. Ensure the container user can read the file and traverse its parent directories."
	}
	return issue
}

func readStartupConfig(configPath string) (*parsedYAMLConfig, *config, error) {
	parsed, err := parseYAMLIncludesWithSources(configPath)
	if err != nil {
		return nil, nil, err
	}
	candidate, err := newConfigFromParsedYAML(parsed)
	return parsed, candidate, err
}

func loadStartupConfig(ctx context.Context, configPath string) (*parsedYAMLConfig, *config, error) {
	parsed, candidate, err := readStartupConfig(configPath)
	if err == nil {
		return parsed, candidate, nil
	}
	return recoverStartupConfig(ctx, configPath, err, 2*time.Second, func(handler http.Handler) (*processServer, error) {
		return newProcessServer("", 8080, handler)
	})
}

// Polling also handles absent parent directories and replaced Docker mounts;
// the normal file watcher cannot watch a configuration that does not exist yet.
// The fallback listener is fully stopped before the configured listener starts,
// including when both use port 8080. Normal reload behavior remains unchanged.
func recoverStartupConfig(ctx context.Context, configPath string, initialErr error, interval time.Duration, open func(http.Handler) (*processServer, error)) (_ *parsedYAMLConfig, _ *config, resultErr error) {
	page := &startupConfigPage{configPath: configPath, issue: startupConfigDiagnostic(configPath, initialErr)}
	server, err := open(page)
	if err != nil {
		return nil, nil, fmt.Errorf("configuration failed (%w); starting recovery server on port 8080: %w", initialErr, err)
	}
	done := make(chan struct{})
	var serveErr error
	go func() {
		defer close(done)
		serveErr = server.serve()
	}()
	defer func() {
		resultErr = errors.Join(resultErr, server.shutdown())
		<-done
	}()
	logConfigDiagnostic(slog.LevelError, "Configuration failed; recovery page available on port 8080; retrying every 2 seconds", initialErr)
	previousError := initialErr.Error()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-done:
			if serveErr == nil {
				serveErr = errors.New("recovery server stopped unexpectedly")
			}
			return nil, nil, serveErr
		case <-ticker.C:
			parsed, candidate, err := readStartupConfig(configPath)
			if err == nil {
				slog.Info("Configuration recovered; starting configured application", "config", configPath, "host", candidate.Server.Host, "port", candidate.Server.Port, "base_url", candidate.Server.BaseURL)
				return parsed, candidate, nil
			}
			if err.Error() != previousError {
				previousError = err.Error()
				logConfigDiagnostic(slog.LevelError, "Configuration still invalid; recovery page remains available", err)
			}
			page.mu.Lock()
			page.issue = startupConfigDiagnostic(configPath, err)
			page.mu.Unlock()
		}
	}
}

type startupConfigPage struct {
	mu         sync.RWMutex
	configPath string
	issue      startupConfigIssue
}

var startupConfigTemplate = template.Must(template.New("config-startup").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
{{if .Issue}}<meta http-equiv="refresh" content="5">{{end}}
<title>Glance configuration help</title>
<style>body{font:17px/1.6 system-ui,sans-serif;background:#15171c;color:#e0e2e7;max-width:900px;margin:6vh auto;padding:24px}a{color:#a9c8ff}code,pre{background:#232730;padding:3px 6px;overflow:auto}pre{padding:16px}h1,h2{line-height:1.25}img{max-width:100%}.issue{border-left:4px solid #e8b56b;padding:0 20px}li{margin:8px 0}</style>
</head><body><h1>Glance configuration help</h1>
{{if .Issue}}<section class="issue"><h2>Configuration could not be loaded</h2>
<p>{{.Issue.Summary}}</p><p>File: <code>{{.Issue.File}}</code>{{if .Issue.Line}} · Line: <strong>{{.Issue.Line}}</strong>{{end}}</p></section>
<p>Expected main configuration: <code>{{.ConfigPath}}</code></p>
<ul><li>Ensure you created <code>glance.yml</code>; an empty directory is insufficient.</li>
<li>Check the volume mapping. With <code>./config:/app/config</code>, the host file must be <code>./config/glance.yml</code>. Check the Compose project directory and filename spelling.</li>
<li>Ensure the container user can read the file and access its parent directories. A read-only mount is supported.</li>
<li>For YAML or setting errors, inspect the reported file and line, including any included files. Full diagnostic details are in <code>docker compose logs glance</code>.</li></ul>
<p>Glance retries every two seconds. This page refreshes automatically. Once corrected, the dashboard starts with the configured host, port, base URL, and authentication. If these differ, open the configured address.</p>
<p>This unauthenticated recovery page uses port <strong>8080</strong>, without a base URL. Container port 8080 must be published or reachable through your proxy. It exposes only setup guidance and bundled docs.</p>
{{end}}<nav><a href="/docs/README#installation">Local installation instructions</a> · <a href="/docs/configuration">Local configuration reference</a> · <a href="/docs/assets/glance.yml">Starter configuration</a> · <a href="https://github.com/samcro1967/glance#installation">GitHub installation instructions</a> · <a href="/">Recovery status</a></nav>
{{.Document}}</body></html>`))

func (p *startupConfigPage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' https:; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/docs/assets/") {
		name := strings.TrimPrefix(r.URL.Path, "/docs/assets/")
		if !fs.ValidPath(name) {
			http.NotFound(w, r)
			return
		}
		switch path.Ext(name) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".yml", ".yaml":
		default:
			http.NotFound(w, r)
			return
		}
		contents, err := fs.ReadFile(glancedocs.Files, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType == "" || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			contentType = "text/plain; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
		if r.Method != http.MethodHead {
			_, _ = w.Write(contents)
		}
		return
	}
	data := struct {
		Issue      *startupConfigIssue
		ConfigPath string
		Document   template.HTML
	}{ConfigPath: p.configPath}
	status := http.StatusOK
	if r.URL.Path == "/" {
		p.mu.RLock()
		issue := p.issue
		p.mu.RUnlock()
		data.Issue = &issue
		status = http.StatusServiceUnavailable
		w.Header().Set("Retry-After", "2")
	} else if r.URL.Path == "/docs" || strings.HasPrefix(r.URL.Path, "/docs/") {
		name := strings.TrimPrefix(r.URL.Path, "/docs")
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			name = "README"
		}
		source, documentPath, err := readDocsDocument(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data.Document, err = renderDocsMarkdown(newDocumentationMarkdownRenderer(), source, documentPath, "")
		if err != nil {
			http.Error(w, "Documentation could not be rendered", http.StatusInternalServerError)
			return
		}
	} else {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		if err := startupConfigTemplate.Execute(w, data); err != nil {
			slog.Warn("Failed to render configuration recovery page", "error", err)
		}
	}
}
