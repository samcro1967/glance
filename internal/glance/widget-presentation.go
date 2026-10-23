package glance

import (
	"html/template"
	"log/slog"
	"strings"
)

// Templates are initialized after the shared function map. Keeping this pointer
// uninitialized at declaration avoids a function-map/template initialization cycle.
var widgetStateTemplate *template.Template

var compactStateTemplate = template.Must(template.New("compact-state").Parse(`<span class="footer-micro-item widget-state" data-widget-id="{{.ID}}" data-widget-state="{{if .Loading}}loading{{else}}error{{end}}"{{if .Loading}} role="status" aria-busy="true"{{end}}>{{if .Loading}}<span class="loading-icon widget-loading-icon" aria-hidden="true"></span><span>Loading…</span>{{else}}<span class="color-negative" title="{{.Error}}">Unavailable</span>{{end}}</span>`))

func init() {
	widgetStateTemplate = mustParseTemplate("widget-base.html")
}

// IsLoading describes only the first pending result. A real error stays visible
// during retries, cancellation does not finish initial loading, and successful
// empty responses count as content through the existing outcome handler.
// Call while holding refreshMu, or during single-threaded initialization.
func (w *widgetBase) IsLoading() bool {
	return w.cacheType != cacheTypeInfinite && !w.initialRefreshCompleted && !w.ContentAvailable && w.Error == nil
}

func renderCompactState(base *widgetBase) template.HTML {
	var buffer strings.Builder
	data := struct {
		ID      uint64
		Loading bool
		Error   error
	}{base.ID, base.IsLoading(), base.Error}
	// This fixed template only formats scalar fields into an in-memory builder.
	if err := compactStateTemplate.Execute(&buffer, data); err != nil {
		slog.Error("Failed to render compact widget state", "widget_id", base.ID, "error", err)
		return ""
	}
	return template.HTML(buffer.String())
}

// renderCommittedWidget is called under the widget refresh lock. Provider data
// is never read while a fetch is mutating it. Static/container rendering stays
// with its existing implementation; shared states also cover cached-HTML widgets.
func renderCommittedWidget(candidate widget) template.HTML {
	base, ok := widgetBaseOf(candidate)
	if !ok {
		return candidate.Render()
	}
	if base.IsLoading() || (!base.ContentAvailable && base.Error != nil) {
		if _, micro := candidate.(microWidget); micro {
			return renderCompactState(base)
		}
		var buffer strings.Builder
		if err := widgetStateTemplate.Execute(&buffer, base); err != nil {
			slog.Error("Failed to render widget state", "widget_id", base.ID, "error", err)
			return ""
		}
		return template.HTML(buffer.String())
	}
	return candidate.Render()
}

func compactItemsLocked(candidate microSource, open bool) []statusBarCompactItem {
	base, ok := widgetBaseOf(candidate)
	if ok && base.IsLoading() {
		return []statusBarCompactItem{{Kind: "loading", ErrorTitle: base.Title}}
	}
	if ok && !base.ContentAvailable && base.Error != nil {
		return compactError(base.Title, base.Error)
	}
	return candidate.MicroItems(open)
}

// Status bars use the same child lock and committed snapshot boundary as cards.
// Copy the slice before changing navigation flags; snapshots remain immutable.
func renderCompactItems(candidate microSource, open bool) []statusBarCompactItem {
	base, ok := widgetBaseOf(candidate)
	if !ok {
		return candidate.MicroItems(open)
	}
	if !candidate.tryLockRefresh() {
		base.renderSnapshotMu.RLock()
		items := append([]statusBarCompactItem(nil), base.compactSnapshot...)
		base.renderSnapshotMu.RUnlock()
		for i := range items {
			items[i].OpenLinksInNewTab = open
		}
		return items
	}
	defer candidate.unlockRefresh()
	return compactItemsLocked(candidate, open)
}

func expandedContentLocked(expanded expandedWidget, base *widgetBase) template.HTML {
	if base.IsLoading() || !base.ContentAvailable {
		return renderCompactState(base)
	}
	return expanded.RenderExpanded()
}

func renderExpandedWidget(candidate widget, expanded expandedWidget, base *widgetBase) template.HTML {
	if !candidate.tryLockRefresh() {
		base.renderSnapshotMu.RLock()
		content := base.expandedSnapshot
		base.renderSnapshotMu.RUnlock()
		return content
	}
	defer candidate.unlockRefresh()
	return expandedContentLocked(expanded, base)
}

// Capture the previous committed model before network work begins. This also
// creates a loading snapshot before the very first fetch, so first-page rendering
// never has to wait for that fetch. Do not copy widgetBase: it owns mutexes.
func captureWidgetPresentation(candidate widget, base *widgetBase) {
	base.renderSnapshotMu.RLock()
	current := base.renderSnapshotCurrent
	base.renderSnapshotMu.RUnlock()
	var normal template.HTML
	if !current {
		normal = renderCommittedWidget(candidate)
	}
	var compact []statusBarCompactItem
	if micro, ok := candidate.(microSource); ok {
		compact = compactItemsLocked(micro, true)
	}
	var expandedHTML template.HTML
	if expanded, ok := candidate.(expandedWidget); ok && base.HasExpandedView() {
		expandedHTML = expandedContentLocked(expanded, base)
	}
	base.renderSnapshotMu.Lock()
	if !current {
		base.renderSnapshot = normal
		base.renderSnapshotReady = true
		base.renderSnapshotCurrent = true
	}
	base.compactSnapshot = append([]statusBarCompactItem(nil), compact...)
	base.expandedSnapshot = expandedHTML
	base.snapshotLoading = base.IsLoading()
	base.renderSnapshotMu.Unlock()
}

// Group headers inspect children independently of the child renderer. Never read
// mutable lifecycle fields without the child lock while its provider is running.
func widgetPresentationLoading(candidate widget) bool {
	base, ok := widgetBaseOf(candidate)
	if !ok {
		return false
	}
	if candidate.tryLockRefresh() {
		defer candidate.unlockRefresh()
		return base.IsLoading()
	}
	base.renderSnapshotMu.RLock()
	defer base.renderSnapshotMu.RUnlock()
	return base.snapshotLoading
}

// Bars own compact child layout, while only their leaves own scheduled fetching.
// Register render/authorization targets separately, including after widget reuse.
func (a *application) registerStatusBarTargets() {
	targets := make(map[uint64][]uint64)
	var visit func(*page, widgets)
	visit = func(page *page, source widgets) {
		for _, candidate := range source {
			if bar, ok := candidate.(*statusBarWidget); ok {
				a.widgetByID[bar.ID] = bar
				a.widgetPages[bar.ID] = append(a.widgetPages[bar.ID], page)
				for _, child := range collectRefreshWidgets(bar.childWidgets()) {
					targets[child.GetID()] = append(targets[child.GetID()], bar.ID)
				}
			}
			if container, ok := candidate.(widgetContainer); ok {
				visit(page, container.childWidgets())
			}
		}
	}
	for i := range a.Config.Pages {
		page := &a.Config.Pages[i]
		visit(page, page.HeadWidgets)
		for _, column := range page.Columns {
			visit(page, column.Widgets)
		}
		visit(page, page.BottomWidgets)
	}
	a.liveUpdates.mu.Lock()
	a.liveUpdates.renderTargets = targets
	a.liveUpdates.mu.Unlock()
}

var widgetNoticeTemplate = template.Must(template.New("widget-notice").Parse(`<span class="notice-icon {{if .Error}}notice-icon-major{{else}}notice-icon-minor{{end}}" title="{{if .Error}}{{.Error}}{{else}}{{.Notice}}{{end}}"></span>`))

// Shared notice slot for specialized footer templates. The fixed template escapes
// error text and leaves the existing root element and widget ID intact.
func renderWidgetNotice(candidate widget) template.HTML {
	base, ok := widgetBaseOf(candidate)
	if !ok || (base.Error == nil && base.Notice == nil) {
		return ""
	}
	var buffer strings.Builder
	if err := widgetNoticeTemplate.Execute(&buffer, base); err != nil {
		slog.Error("Failed to render widget notice", "widget_id", base.ID, "error", err)
		return ""
	}
	return template.HTML(buffer.String())
}
