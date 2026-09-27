package glance

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func stateAuthBoundaryTestApplication(t *testing.T) (*application, *page, *page) {
	t.Helper()

	app := newAuthTestApplication(t)
	allowedPage := &page{Slug: "home"}
	deniedPage := &page{Slug: "admin"}
	defaultDashboard := &dashboard{Name: "Default", Pages: []*page{allowedPage}}
	adminDashboard := &dashboard{Name: "Admin", Slug: "admin", Pages: []*page{deniedPage}}
	app.dashboards = []*dashboard{defaultDashboard, adminDashboard}
	app.authorization = newAuthorizationPolicy(
		nil,
		authAccessConfig{Dashboards: map[string]authDashboardAccessConfig{
			"Default": {Users: []string{"test-user"}},
			"Admin":   {Users: []string{"operator"}},
		}},
		app.dashboards,
	)
	app.widgetByID = map[uint64]widget{
		1: &clockWidget{widgetBase: widgetBase{ID: 1, Type: "clock"}},
		2: &clockWidget{widgetBase: widgetBase{ID: 2, Type: "clock"}},
	}
	app.widgetPages = map[uint64][]*page{
		1: {allowedPage},
		2: {deniedPage},
	}
	app.globalWidgetIDs = make(map[uint64]struct{})
	app.liveUpdates = newLiveUpdateBroker()
	return app, allowedPage, deniedPage
}

func addStateAuthBoundarySession(t *testing.T, app *application, request *http.Request) {
	t.Helper()
	request.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: authTestSessionToken(t, app, "test-user", time.Now()),
	})
}

func TestWidgetContentRequestRejectsWidgetOutsideAuthorizedPages(t *testing.T) {
	app, _, _ := stateAuthBoundaryTestApplication(t)
	request := httptest.NewRequest(http.MethodGet, "/api/widgets/2/content/", nil)
	request.SetPathValue("widget", "2")
	addStateAuthBoundarySession(t, app, request)
	recorder := httptest.NewRecorder()

	app.handleWidgetContentRequest(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestRuntimeDiagnosticsRequireOperatorWideDashboardAccess(t *testing.T) {
	app, _, _ := stateAuthBoundaryTestApplication(t)
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	addStateAuthBoundarySession(t, app, request)
	recorder := httptest.NewRecorder()

	app.handleRuntimeDiagnosticsRequest(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestLiveUpdatesUnfilteredSubscriptionIsRestrictedToAuthorizedWidgets(t *testing.T) {
	app, _, _ := stateAuthBoundaryTestApplication(t)
	writer := newLiveUpdateDeadlineTestWriter()
	request := httptest.NewRequest(http.MethodGet, "/api/live-updates", nil)
	addStateAuthBoundarySession(t, app, request)

	done := make(chan struct{})
	go func() {
		app.handleLiveUpdatesRequest(writer, request)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		app.liveUpdates.mu.Lock()
		var subscription *liveUpdateSubscription
		for candidate := range app.liveUpdates.subscribers {
			subscription = candidate
			break
		}
		app.liveUpdates.mu.Unlock()

		if subscription != nil {
			subscription.mu.Lock()
			_, allowed := subscription.widgetIDs[1]
			_, denied := subscription.widgetIDs[2]
			subscription.mu.Unlock()
			if !allowed || denied {
				t.Fatalf("subscription filter allowed=%t denied=%t, want true/false", allowed, denied)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for live-update subscription")
		}
		time.Sleep(time.Millisecond)
	}

	app.liveUpdates.close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("live-updates handler did not stop after broker close")
	}
}
