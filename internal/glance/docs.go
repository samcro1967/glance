package glance

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	glancedocs "github.com/samcro1967/glance/docs"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func cleanDocsDocumentPath(requestPath string) (string, bool) {
	if requestPath == "" || strings.HasPrefix(requestPath, "/") || strings.Contains(requestPath, "\\\\") {
		return "", false
	}

	cleaned := path.Clean(requestPath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != requestPath {
		return "", false
	}

	if strings.HasSuffix(cleaned, ".md") {
		cleaned = strings.TrimSuffix(cleaned, ".md")
	}

	if path.Ext(cleaned) != "" {
		return "", false
	}

	return cleaned + ".md", true
}

func readDocsDocument(requestPath string) ([]byte, string, error) {
	if requestPath == "README" || requestPath == "README.md" {
		if len(docsREADME) == 0 {
			return nil, "", fs.ErrNotExist
		}
		return docsREADME, "README.md", nil
	}

	if requestPath == "CONTRIBUTING" || requestPath == "CONTRIBUTING.md" || requestPath == "contributing" {
		if len(docsContributing) == 0 {
			return nil, "", fs.ErrNotExist
		}
		return docsContributing, "CONTRIBUTING.md", nil
	}

	filePath, ok := cleanDocsDocumentPath(requestPath)
	if !ok {
		return nil, "", fs.ErrNotExist
	}

	source, err := fs.ReadFile(glancedocs.Files, filePath)
	if err != nil {
		return nil, "", err
	}

	return source, filePath, nil
}

var trustedReadmeHTMLDestinationPattern = regexp.MustCompile(`(href|src)="([^"]+)"`)

func rewriteTrustedReadmeHTMLDestinations(source []byte, baseURL string) []byte {
	return trustedReadmeHTMLDestinationPattern.ReplaceAllFunc(source, func(attribute []byte) []byte {
		parts := trustedReadmeHTMLDestinationPattern.FindSubmatch(attribute)
		if len(parts) != 3 {
			return attribute
		}

		name := string(parts[1])
		target := string(parts[2])

		// The README navigation links Configuration to the detailed reference
		// for GitHub. In the in-app README, keep that top navigation on the
		// README itself, matching the existing Installation anchor behavior.
		if name == "href" &&
			target == "docs/configuration.md#configuring-glance" {
			return []byte(name + `="` + baseURL + `/docs#configuration"`)
		}

		rewritten := docsURLForTarget("README.md", target, baseURL, name == "src")

		return []byte(name + `="` + rewritten + `"`)
	})
}

func docsURLForTarget(currentDocument, target, baseURL string, image bool) string {
	parsed, err := url.Parse(target)
	if err != nil ||
		parsed.Scheme != "" ||
		parsed.Host != "" ||
		strings.HasPrefix(target, "//") {
		return target
	}

	if parsed.Path == "" {
		return target
	}

	currentRepositoryPath := currentDocument
	if currentDocument != "README.md" &&
		currentDocument != "CONTRIBUTING.md" {
		currentRepositoryPath = path.Join("docs", currentDocument)
	}

	resolved := path.Clean(
		path.Join(path.Dir(currentRepositoryPath), parsed.Path),
	)
	if resolved == "." ||
		resolved == ".." ||
		strings.HasPrefix(resolved, "../") {
		return target
	}

	var rewritten string

	switch resolved {
	case "README.md":
		rewritten = baseURL + "/docs"
	case "CONTRIBUTING.md":
		rewritten = baseURL + "/docs/contributing"
	default:
		if !strings.HasPrefix(resolved, "docs/") {
			return target
		}

		docsPath := strings.TrimPrefix(resolved, "docs/")
		if !image && strings.HasSuffix(docsPath, ".md") {
			rewritten = baseURL + "/docs/" +
				strings.TrimSuffix(docsPath, ".md")
		} else {
			rewritten = baseURL + "/docs/assets/" + docsPath
		}
	}

	if parsed.RawQuery != "" {
		rewritten += "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		rewritten += "#" + parsed.Fragment
	}

	return rewritten
}

func rewriteDocsDestinations(document ast.Node, currentDocument, baseURL string) error {
	return ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch typed := node.(type) {
		case *ast.Link:
			typed.Destination = []byte(
				docsURLForTarget(currentDocument, string(typed.Destination), baseURL, false),
			)
		case *ast.Image:
			typed.Destination = []byte(
				docsURLForTarget(currentDocument, string(typed.Destination), baseURL, true),
			)
		}

		return ast.WalkContinue, nil
	})
}

func renderDocsMarkdown(
	markdown goldmark.Markdown,
	source []byte,
	documentPath string,
	baseURL string,
) (template.HTML, error) {
	if documentPath == "README.md" {
		source = rewriteTrustedReadmeHTMLDestinations(source, baseURL)
	}

	reader := text.NewReader(source)
	document := markdown.Parser().Parse(reader)

	if err := rewriteDocsDestinations(document, documentPath, baseURL); err != nil {
		return "", fmt.Errorf("rewriting documentation links: %w", err)
	}

	var output bytes.Buffer
	if err := markdown.Renderer().Render(&output, source, document); err != nil {
		return "", fmt.Errorf("rendering documentation markdown: %w", err)
	}

	return template.HTML(output.String()), nil
}

func (a *application) handleDocsRequest(w http.ResponseWriter, r *http.Request) {
	session, authenticated := a.authorizeSession(w, r)
	if !authenticated {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
		return
	}

	requestPath := strings.TrimPrefix(r.PathValue("path"), "/")
	if requestPath == "" {
		requestPath = "README"
	}

	source, documentPath, err := readDocsDocument(requestPath)
	if err != nil {
		a.renderNotFound(w, r, session, nil, nil, "")
		return
	}

	markdown := newDocumentationMarkdownRenderer()

	if documentPath == "README.md" {

		markdown = newTrustedDocumentationMarkdownRenderer()

	}

	content, err := renderDocsMarkdown(
		markdown,
		source,
		documentPath,
		a.Config.Server.BaseURL,
	)
	if err != nil {
		writeInternalServerError(w, "Failed to render documentation", err)
		return
	}

	generalDocs, exampleDocs, widgetDocs, err := buildDocsNavigation()
	if err != nil {
		writeInternalServerError(w, "Failed to build documentation navigation", err)
		return
	}

	a.setAuthenticatedDynamicResponseHeaders(w)

	data := templateData{
		App: a,
		Dashboards: a.authorization.authorizedDashboards(
			session.AuthorizationIdentity,
			a.dashboards,
		),
		Docs: &docsPageData{
			Title:       docsTitle(documentPath, source),
			Path:        strings.TrimSuffix(documentPath, ".md"),
			Content:     content,
			GeneralDocs: generalDocs,
			ExampleDocs: exampleDocs,
			WidgetDocs:  widgetDocs,
		},
	}
	a.populateTemplateRequestData(&data.Request, r, nil)

	if a.RequiresAuth && session.DisplayName != "" {
		data.Request.AuthDisplayName = session.DisplayName
		switch session.Method {
		case authMethodLocal:
			data.Request.AuthDescription = "Signed in locally"
		case authMethodOIDC:
			data.Request.AuthDescription = "Signed in with " + a.OIDCProviderName()
		}
	}

	var response bytes.Buffer
	if err := docsTemplate.Execute(&response, data); err != nil {
		writeInternalServerError(w, "Failed to render documentation page", err)
		return
	}

	_, _ = w.Write(response.Bytes())
}

func (a *application) handleDocsAssetRequest(w http.ResponseWriter, r *http.Request) {
	session, authenticated := a.authorizeSession(w, r)
	if !authenticated {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
		return
	}
	_ = session

	assetPath := strings.TrimPrefix(r.PathValue("path"), "/")
	if assetPath == "" ||
		assetPath == "." ||
		strings.HasPrefix(assetPath, "/") ||
		strings.Contains(assetPath, "\\\\") {
		http.NotFound(w, r)
		return
	}

	cleaned := path.Clean(assetPath)
	if cleaned != assetPath ||
		cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") {
		http.NotFound(w, r)
		return
	}

	asset, err := fs.ReadFile(glancedocs.Files, cleaned)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch strings.ToLower(path.Ext(cleaned)) {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(asset)
}

type docsNavigationItem struct {
	Title string
	Path  string
	Level int
}

type docsPageData struct {
	Title       string
	Path        string
	Content     template.HTML
	GeneralDocs []docsNavigationItem
	ExampleDocs []docsNavigationItem
	WidgetDocs  []docsNavigationItem
}

func docsTitle(filePath string, source []byte) string {
	for _, line := range strings.Split(string(source), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}

	name := strings.TrimSuffix(path.Base(filePath), ".md")
	return strings.ReplaceAll(name, "-", " ")
}

func buildDocsNavigation() ([]docsNavigationItem, []docsNavigationItem, []docsNavigationItem, error) {
	general := make([]docsNavigationItem, 0)
	examples := make([]docsNavigationItem, 0)
	widgets := make([]docsNavigationItem, 0)

	err := fs.WalkDir(glancedocs.Files, ".", func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(filePath, ".md") {
			return nil
		}

		source, err := fs.ReadFile(glancedocs.Files, filePath)
		if err != nil {
			return err
		}

		item := docsNavigationItem{
			Title: docsTitle(filePath, source),
			Path:  strings.TrimSuffix(filePath, ".md"),
		}

		switch {
		case filePath == "examples.md":
			examples = append(examples, item)

		case strings.HasPrefix(filePath, "examples/"):
			item.Level = 1
			if strings.Contains(filePath, "/integrations/") {
				item.Level = 2
			}
			examples = append(examples, item)

		case strings.HasPrefix(filePath, "widgets/"):
			widgets = append(widgets, item)

		default:
			general = append(general, item)
		}

		return nil
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("building documentation navigation: %w", err)
	}

	return general, examples, widgets, nil
}
