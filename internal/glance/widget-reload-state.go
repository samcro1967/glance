package glance

import (
	"crypto/sha256"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

type widgetReloadFingerprint [sha256.Size]byte

type widgetReloadReusePlan map[widget]widget

func calculateWidgetReloadFingerprint(candidate widget) (widgetReloadFingerprint, error) {
	serialized, err := yaml.Marshal(candidate)
	if err != nil {
		return widgetReloadFingerprint{}, fmt.Errorf("serializing %s widget configuration: %w", candidate.GetType(), err)
	}

	effective := struct {
		Serialized          []byte
		OpenLinksInNewTab   bool
		DockerDefaultNewTab *bool
	}{
		Serialized: serialized,
	}

	if base, ok := widgetBaseOf(candidate); ok {
		effective.OpenLinksInNewTab = base.OpenLinksInNewTab
	}
	if dockerWidget, ok := candidate.(*dockerContainersWidget); ok && dockerWidget.DefaultNewTab != nil {
		value := *dockerWidget.DefaultNewTab
		effective.DockerDefaultNewTab = &value
	}

	effectiveSerialized, err := yaml.Marshal(effective)
	if err != nil {
		return widgetReloadFingerprint{}, fmt.Errorf("serializing %s widget effective configuration: %w", candidate.GetType(), err)
	}

	return widgetReloadFingerprint(sha256.Sum256(effectiveSerialized)), nil
}

func captureWidgetReloadFingerprints(source []widget) (map[widget]widgetReloadFingerprint, error) {
	fingerprints := make(map[widget]widgetReloadFingerprint, len(source))
	for _, candidate := range source {
		fingerprint, err := calculateWidgetReloadFingerprint(candidate)
		if err != nil {
			return nil, err
		}
		fingerprints[candidate] = fingerprint
	}
	return fingerprints, nil
}

func prepareWidgetReloadReusePlan(previous, candidate *application) (widgetReloadReusePlan, error) {
	plan := make(widgetReloadReusePlan)
	if previous == nil || candidate == nil {
		return plan, nil
	}

	// Provider inputs participate in runtime behavior and generated URLs. Keep
	// reuse conservative when those inputs change; reused widgets are rebound to
	// the candidate generation providers when the plan is applied.
	if previous.Config.Server.BaseURL != candidate.Config.Server.BaseURL ||
		!reflect.DeepEqual(
			previous.Config.Server.ResourceProxy.AllowedOrigins,
			candidate.Config.Server.ResourceProxy.AllowedOrigins,
		) {
		return plan, nil
	}

	type reuseKey struct {
		widgetType  string
		fingerprint widgetReloadFingerprint
	}

	available := make(map[reuseKey][]widget)
	for _, previousWidget := range previous.refreshWidgets {
		fingerprint, ok := previous.widgetReloadFingerprints[previousWidget]
		if !ok {
			continue
		}
		key := reuseKey{widgetType: previousWidget.GetType(), fingerprint: fingerprint}
		available[key] = append(available[key], previousWidget)
	}

	for _, candidateWidget := range candidate.refreshWidgets {
		fingerprint, ok := candidate.widgetReloadFingerprints[candidateWidget]
		if !ok {
			continue
		}
		key := reuseKey{widgetType: candidateWidget.GetType(), fingerprint: fingerprint}
		matches := available[key]
		if len(matches) == 0 {
			continue
		}

		plan[candidateWidget] = matches[0]
		available[key] = matches[1:]
	}

	return plan, nil
}

func replaceReloadWidget(candidate widget, plan widgetReloadReusePlan) widget {
	if replacement, ok := plan[candidate]; ok {
		return replacement
	}

	container, ok := candidate.(widgetContainer)
	if !ok {
		return candidate
	}

	children := container.childWidgets()
	for i := range children {
		children[i] = replaceReloadWidget(children[i], plan)
	}
	return candidate
}

func (a *application) applyWidgetReloadReusePlan(plan widgetReloadReusePlan) {
	if a == nil || len(plan) == 0 {
		return
	}

	replaceMicroWidgets := func(source microWidgets) {
		for i, candidate := range source {
			dynamic, ok := candidate.(dynamicMicroWidget)
			if !ok {
				continue
			}
			replacement, ok := plan[dynamic]
			if !ok {
				continue
			}
			source[i] = replacement.(dynamicMicroWidget)
		}
	}
	replaceMicroWidgets(a.Config.FooterMicroWidgets.Left)
	replaceMicroWidgets(a.Config.FooterMicroWidgets.Right)

	if a.widgetProviders != nil {
		for _, dynamic := range a.Config.FooterMicroWidgets.dynamicWidgets() {
			dynamic.setProviders(a.widgetProviders)
		}
	}

	for p := range a.Config.Pages {
		page := &a.Config.Pages[p]
		for i := range page.HeadWidgets {
			page.HeadWidgets[i] = replaceReloadWidget(page.HeadWidgets[i], plan)
			if a.widgetProviders != nil {
				page.HeadWidgets[i].setProviders(a.widgetProviders)
			}
		}
		for c := range page.Columns {
			for i := range page.Columns[c].Widgets {
				page.Columns[c].Widgets[i] = replaceReloadWidget(page.Columns[c].Widgets[i], plan)
				if a.widgetProviders != nil {
					page.Columns[c].Widgets[i].setProviders(a.widgetProviders)
				}
			}
		}
		for i := range page.BottomWidgets {
			page.BottomWidgets[i] = replaceReloadWidget(page.BottomWidgets[i], plan)
			if a.widgetProviders != nil {
				page.BottomWidgets[i].setProviders(a.widgetProviders)
			}
		}
	}

	a.widgetByID = make(map[uint64]widget)
	a.widgetPages = make(map[uint64][]*page)
	a.globalWidgetIDs = make(map[uint64]struct{})
	refreshSources := make(widgets, 0)
	footerDynamicWidgets := a.Config.FooterMicroWidgets.dynamicWidgets()
	for _, candidate := range collectRefreshWidgets(footerDynamicWidgets) {
		a.widgetByID[candidate.GetID()] = candidate
		a.globalWidgetIDs[candidate.GetID()] = struct{}{}
	}
	refreshSources = append(refreshSources, footerDynamicWidgets...)

	recordPageWidgets := func(page *page, source widgets) {
		for _, candidate := range collectRefreshWidgets(source) {
			a.widgetByID[candidate.GetID()] = candidate
			a.widgetPages[candidate.GetID()] = append(a.widgetPages[candidate.GetID()], page)
		}
	}

	for p := range a.Config.Pages {
		page := &a.Config.Pages[p]
		recordPageWidgets(page, page.HeadWidgets)
		refreshSources = append(refreshSources, page.HeadWidgets...)

		for c := range page.Columns {
			recordPageWidgets(page, page.Columns[c].Widgets)
			refreshSources = append(refreshSources, page.Columns[c].Widgets...)
		}

		recordPageWidgets(page, page.BottomWidgets)
		refreshSources = append(refreshSources, page.BottomWidgets...)
	}

	a.refreshWidgets = collectRefreshWidgets(refreshSources)
	reusedFingerprints := make(map[widget]widgetReloadFingerprint, len(a.refreshWidgets))
	for _, candidate := range a.refreshWidgets {
		a.widgetByID[candidate.GetID()] = candidate
		if fingerprint, ok := a.widgetReloadFingerprints[candidate]; ok {
			reusedFingerprints[candidate] = fingerprint
			continue
		}
		for replaced, replacement := range plan {
			if replacement == candidate {
				reusedFingerprints[candidate] = a.widgetReloadFingerprints[replaced]
				break
			}
		}
	}
	a.widgetReloadFingerprints = reusedFingerprints
}
