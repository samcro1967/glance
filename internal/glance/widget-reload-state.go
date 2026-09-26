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

	return widgetReloadFingerprint(sha256.Sum256(serialized)), nil
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

	// Reused widgets retain their existing provider closures. Only reuse them
	// when the application-level inputs to those providers are unchanged.
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

	for p := range a.Config.Pages {
		page := &a.Config.Pages[p]
		for i := range page.HeadWidgets {
			page.HeadWidgets[i] = replaceReloadWidget(page.HeadWidgets[i], plan)
		}
		for c := range page.Columns {
			for i := range page.Columns[c].Widgets {
				page.Columns[c].Widgets[i] = replaceReloadWidget(page.Columns[c].Widgets[i], plan)
			}
		}
		for i := range page.BottomWidgets {
			page.BottomWidgets[i] = replaceReloadWidget(page.BottomWidgets[i], plan)
		}
	}

	a.widgetByID = make(map[uint64]widget)
	refreshSources := make(widgets, 0)
	footerDynamicWidgets := a.Config.FooterMicroWidgets.dynamicWidgets()
	for _, candidate := range footerDynamicWidgets {
		a.widgetByID[candidate.GetID()] = candidate
	}
	refreshSources = append(refreshSources, footerDynamicWidgets...)

	for p := range a.Config.Pages {
		page := &a.Config.Pages[p]
		for _, candidate := range page.HeadWidgets {
			a.widgetByID[candidate.GetID()] = candidate
		}
		refreshSources = append(refreshSources, page.HeadWidgets...)

		for c := range page.Columns {
			for _, candidate := range page.Columns[c].Widgets {
				a.widgetByID[candidate.GetID()] = candidate
			}
			refreshSources = append(refreshSources, page.Columns[c].Widgets...)
		}

		for _, candidate := range page.BottomWidgets {
			a.widgetByID[candidate.GetID()] = candidate
		}
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
