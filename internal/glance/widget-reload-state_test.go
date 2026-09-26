package glance

import (
	"html/template"
	"testing"
	"time"
)

func reloadStateTestApplication(t *testing.T, title string) *application {
	t.Helper()

	return newGlanceTestApplication(t, `
server:
  base-url: /glance
  resource-proxy:
    allowed-origins:
      - http://example.test
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: monitor
            title: `+title+`
            sites:
              - title: Example
                url: https://example.com
`)
}

func TestWidgetReloadReusePreservesUnchangedWidgetInstanceAndRuntimeState(t *testing.T) {
	previous := reloadStateTestApplication(t, "Services")
	candidate := reloadStateTestApplication(t, "Services")

	oldWidget := previous.refreshWidgets[0]
	oldBase, ok := widgetBaseOf(oldWidget)
	if !ok {
		t.Fatal("previous widget does not expose widgetBase")
	}
	oldBase.setNextUpdateTime(time.Now().Add(time.Hour))
	oldBase.renderSnapshot = template.HTML("cached-render")
	oldBase.renderSnapshotReady = true
	oldBase.refreshAttempts = 7

	newWidget := candidate.refreshWidgets[0]
	if oldWidget == newWidget {
		t.Fatal("independently constructed applications unexpectedly share widget instance")
	}

	plan, err := prepareWidgetReloadReusePlan(previous, candidate)
	if err != nil {
		t.Fatalf("prepareWidgetReloadReusePlan() error = %v", err)
	}
	candidate.applyWidgetReloadReusePlan(plan)

	if candidate.refreshWidgets[0] != oldWidget {
		t.Fatal("unchanged widget instance was not reused")
	}
	if _, exists := candidate.widgetByID[oldWidget.GetID()]; !exists {
		t.Fatalf("reused widget ID %d is not registered", oldWidget.GetID())
	}

	reusedBase, _ := widgetBaseOf(candidate.refreshWidgets[0])
	if reusedBase.nextUpdateTime() != oldBase.nextUpdateTime() {
		t.Fatal("reused widget did not preserve next refresh time")
	}
	if reusedBase.renderSnapshot != template.HTML("cached-render") || !reusedBase.renderSnapshotReady {
		t.Fatal("reused widget did not preserve rendered snapshot")
	}
	if reusedBase.refreshAttempts != 7 {
		t.Fatalf("reused widget refresh attempts = %d, want 7", reusedBase.refreshAttempts)
	}
}

func TestWidgetReloadReuseDoesNotReuseChangedWidget(t *testing.T) {
	previous := reloadStateTestApplication(t, "Services")
	candidate := reloadStateTestApplication(t, "Changed Services")

	oldWidget := previous.refreshWidgets[0]
	newWidget := candidate.refreshWidgets[0]

	plan, err := prepareWidgetReloadReusePlan(previous, candidate)
	if err != nil {
		t.Fatalf("prepareWidgetReloadReusePlan() error = %v", err)
	}
	candidate.applyWidgetReloadReusePlan(plan)

	if candidate.refreshWidgets[0] != newWidget {
		t.Fatal("changed widget was unexpectedly replaced")
	}
	if candidate.refreshWidgets[0] == oldWidget {
		t.Fatal("changed widget unexpectedly reused previous runtime state")
	}
}

func TestWidgetReloadReuseDisabledWhenProviderInputsChange(t *testing.T) {
	previous := reloadStateTestApplication(t, "Services")
	candidate := reloadStateTestApplication(t, "Services")
	candidate.Config.Server.BaseURL = "/different"

	plan, err := prepareWidgetReloadReusePlan(previous, candidate)
	if err != nil {
		t.Fatalf("prepareWidgetReloadReusePlan() error = %v", err)
	}
	if len(plan) != 0 {
		t.Fatalf("reuse plan contains %d widgets after provider inputs changed, want 0", len(plan))
	}
}

func TestWidgetReloadReuseMatchesUnchangedWidgetsAfterInsertion(t *testing.T) {
	previous := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: monitor
            title: First
            sites:
              - title: First
                url: https://first.example.com
          - type: monitor
            title: Second
            sites:
              - title: Second
                url: https://second.example.com
`)
	candidate := newGlanceTestApplication(t, `
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: monitor
            title: Inserted
            sites:
              - title: Inserted
                url: https://inserted.example.com
          - type: monitor
            title: First
            sites:
              - title: First
                url: https://first.example.com
          - type: monitor
            title: Second
            sites:
              - title: Second
                url: https://second.example.com
`)

	first := previous.refreshWidgets[0]
	second := previous.refreshWidgets[1]
	inserted := candidate.refreshWidgets[0]

	plan, err := prepareWidgetReloadReusePlan(previous, candidate)
	if err != nil {
		t.Fatalf("prepareWidgetReloadReusePlan() error = %v", err)
	}
	candidate.applyWidgetReloadReusePlan(plan)

	if candidate.refreshWidgets[0] != inserted {
		t.Fatal("newly inserted widget was unexpectedly replaced")
	}
	if candidate.refreshWidgets[1] != first {
		t.Fatal("first unchanged widget was not matched across insertion")
	}
	if candidate.refreshWidgets[2] != second {
		t.Fatal("second unchanged widget was not matched across insertion")
	}
}

func TestWidgetReloadReuseSkipsWidgetsWithoutFingerprint(t *testing.T) {
	t.Run("previous widget", func(t *testing.T) {
		previous := reloadStateTestApplication(t, "Services")
		candidate := reloadStateTestApplication(t, "Services")

		delete(previous.widgetReloadFingerprints, previous.refreshWidgets[0])

		plan, err := prepareWidgetReloadReusePlan(previous, candidate)
		if err != nil {
			t.Fatalf("prepareWidgetReloadReusePlan() error = %v", err)
		}
		if len(plan) != 0 {
			t.Fatalf("reuse plan contains %d widgets with missing previous fingerprint, want 0", len(plan))
		}
	})

	t.Run("candidate widget", func(t *testing.T) {
		previous := reloadStateTestApplication(t, "Services")
		candidate := reloadStateTestApplication(t, "Services")

		delete(candidate.widgetReloadFingerprints, candidate.refreshWidgets[0])

		plan, err := prepareWidgetReloadReusePlan(previous, candidate)
		if err != nil {
			t.Fatalf("prepareWidgetReloadReusePlan() error = %v", err)
		}
		if len(plan) != 0 {
			t.Fatalf("reuse plan contains %d widgets with missing candidate fingerprint, want 0", len(plan))
		}
	})
}
