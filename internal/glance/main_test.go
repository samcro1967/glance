package glance

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSwappableHandlerRoutesNewRequestsToNewGeneration(t *testing.T) {
	oldHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "old")
	})
	newHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "new")
	})

	handler := newSwappableHandler(oldHandler)

	before := httptest.NewRecorder()
	handler.ServeHTTP(before, httptest.NewRequest(http.MethodGet, "/", nil))
	if before.Body.String() != "old" {
		t.Fatalf("response before swap = %q, want old", before.Body.String())
	}

	handler.swap(newHandler)

	after := httptest.NewRecorder()
	handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/", nil))
	if after.Body.String() != "new" {
		t.Fatalf("response after swap = %q, want new", after.Body.String())
	}
}

func TestSwappableHandlerAllowsInflightOldRequestToFinish(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan string, 1)

	oldHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "old")
	})
	newHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "new")
	})

	handler := newSwappableHandler(oldHandler)
	go func() {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		finished <- recorder.Body.String()
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("old request did not start")
	}

	handler.swap(newHandler)

	newRecorder := httptest.NewRecorder()
	handler.ServeHTTP(newRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if newRecorder.Body.String() != "new" {
		t.Fatalf("new request response = %q, want new", newRecorder.Body.String())
	}

	close(release)

	select {
	case response := <-finished:
		if response != "old" {
			t.Fatalf("in-flight old response = %q, want old", response)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight old request did not finish")
	}
}

func TestProcessServerBindFailureDoesNotCreateServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	_, err = newProcessServer("127.0.0.1", port, http.NotFoundHandler())
	if err == nil {
		t.Fatal("expected bind failure")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("bind failure = %q, want address already in use", err)
	}
}

func TestProcessServerSwapDoesNotRebindListener(t *testing.T) {
	server, err := newProcessServer(
		"127.0.0.1",
		0,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "old")
		}),
	)
	if err != nil {
		t.Fatalf("create process server: %v", err)
	}

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.serve()
	}()

	client := &http.Client{Timeout: time.Second}
	url := "http://" + server.listener.Addr().String()

	response, err := client.Get(url)
	if err != nil {
		t.Fatalf("request old generation: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read old generation: %v", err)
	}
	if string(body) != "old" {
		t.Fatalf("old generation response = %q, want old", body)
	}

	addressBefore := server.listener.Addr().String()
	server.swap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "new")
	}))
	if server.listener.Addr().String() != addressBefore {
		t.Fatal("handler swap changed listener address")
	}

	response, err = client.Get(url)
	if err != nil {
		t.Fatalf("request new generation: %v", err)
	}
	body, err = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read new generation: %v", err)
	}
	if string(body) != "new" {
		t.Fatalf("new generation response = %q, want new", body)
	}

	if err := server.shutdown(); err != nil {
		t.Fatalf("shutdown process server: %v", err)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve returned after shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not return after shutdown")
	}
}

func TestApplicationRuntimeStopCancelsScheduler(t *testing.T) {
	widget := newServerLifecycleTestWidget()
	app := newServerLifecycleTestApplication(t, 0, widget)

	runtime := app.startRuntime()

	select {
	case <-widget.started:
	case <-time.After(time.Second):
		t.Fatal("widget refresh scheduler did not start")
	}

	runtime.stop()

	select {
	case <-widget.cancelled:
	case <-time.After(time.Second):
		t.Fatal("runtime stop did not cancel widget refresh scheduler")
	}
}

func TestProcessServerGracefulShutdownAllowsInflightRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var completed atomic.Bool

	server, err := newProcessServer(
		"127.0.0.1",
		0,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			<-release
			completed.Store(true)
			_, _ = io.WriteString(w, "done")
		}),
	)
	if err != nil {
		t.Fatalf("create process server: %v", err)
	}

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.serve()
	}()

	requestDone := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"http://"+server.listener.Addr().String(),
			nil,
		)
		if err != nil {
			requestDone <- err
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		requestDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- server.shutdown()
	}()

	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-requestDone:
		if err != nil {
			t.Fatalf("request failed during graceful shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}

	if !completed.Load() {
		t.Fatal("in-flight request did not complete")
	}

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("graceful shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("graceful shutdown did not finish")
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve returned after shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not return")
	}
}

func TestRuntimeGenerationRejectsListenerChangeWithoutDisturbingCurrentGeneration(t *testing.T) {
	oldWidget := newServerLifecycleTestWidget()
	oldApp := newServerLifecycleTestApplication(t, 0, oldWidget)
	diagnostics := newConfigRuntimeDiagnostics("glance.yml")
	oldApp.configDiagnostics = diagnostics

	server, err := newProcessServer(
		"127.0.0.1",
		0,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "old")
		}),
	)
	if err != nil {
		t.Fatalf("create process server: %v", err)
	}
	defer func() { _ = server.shutdown() }()
	server.setActiveApplication(oldApp)

	oldRuntime := oldApp.startRuntime()
	defer oldRuntime.stop()

	select {
	case <-oldWidget.started:
	case <-time.After(time.Second):
		t.Fatal("old scheduler did not start")
	}

	generation := &runtimeGeneration{
		runtime: oldRuntime,
		config:  &oldApp.Config,
	}

	candidate := oldApp.Config
	candidate.Server.Port = 65534

	_, err = generation.reload(server, &candidate, diagnostics, nil)
	if err == nil {
		t.Fatal("expected listener change to be rejected")
	}
	if !strings.Contains(err.Error(), "require an application restart") {
		t.Fatalf("listener rejection = %q, want restart-required message", err)
	}
	if generation.runtime != oldRuntime {
		t.Fatal("rejected reload replaced current runtime")
	}
	if server.activeApplication.Load() != oldApp {
		t.Fatal("rejected reload replaced active application")
	}

	select {
	case <-oldWidget.cancelled:
		t.Fatal("rejected reload cancelled current scheduler")
	default:
	}
}

func TestRuntimeGenerationSuccessfulReloadCommitsNewGenerationAndRetiresOld(t *testing.T) {
	oldWidget := newServerLifecycleTestWidget()
	oldApp := newServerLifecycleTestApplication(t, 0, oldWidget)
	diagnostics := newConfigRuntimeDiagnostics("glance.yml")
	oldApp.configDiagnostics = diagnostics

	server, err := newProcessServer(
		"127.0.0.1",
		0,
		oldApp.router(),
	)
	if err != nil {
		t.Fatalf("create process server: %v", err)
	}
	defer func() { _ = server.shutdown() }()
	server.setActiveApplication(oldApp)

	oldSubscription, unsubscribeOld := oldApp.liveUpdates.subscribe(nil)
	defer unsubscribeOld()

	oldRuntime := oldApp.startRuntime()

	select {
	case <-oldWidget.started:
	case <-time.After(time.Second):
		t.Fatal("old scheduler did not start")
	}

	generation := &runtimeGeneration{
		runtime: oldRuntime,
		config:  &oldApp.Config,
	}

	candidateApp := newGlanceTestApplication(t, `
server:
  host: 127.0.0.1
  port: 0
  frontend-diagnostics: true

branding:
  app-name: Reloaded Glance

pages:
  - name: Home
    columns:
      - size: full
        widgets: []
`)
	candidate := candidateApp.Config

	previousRuntime, err := generation.reload(server, &candidate, diagnostics, nil)
	if err != nil {
		t.Fatalf("reload generation: %v", err)
	}
	defer generation.runtime.stop()

	if previousRuntime != oldRuntime {
		t.Fatal("successful reload did not return previous runtime for retirement")
	}
	if generation.runtime == oldRuntime {
		t.Fatal("successful reload retained old runtime")
	}
	if server.activeApplication.Load() != generation.runtime.app {
		t.Fatal("successful reload did not install new active application")
	}
	if server.activeApplication.Load() == oldApp {
		t.Fatal("successful reload retained old active application")
	}

	newSubscription, unsubscribeNew := generation.runtime.app.liveUpdates.subscribe(nil)
	defer unsubscribeNew()

	recorder := httptest.NewRecorder()
	server.processDiagnosticProfileHandler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/debug/frontend-diagnostics/runtime-state", nil),
	)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("post-reload diagnostic status = %d, want %d; body = %q", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}

	newCommands := newSubscription.takeDiagnosticCommands()
	if len(newCommands) != 1 {
		t.Fatalf("new application received %d post-reload commands, want 1", len(newCommands))
	}
	if newCommands[0].Command != "runtime_state" {
		t.Fatalf("post-reload command = %q, want runtime_state", newCommands[0].Command)
	}
	if oldCommands := oldSubscription.takeDiagnosticCommands(); len(oldCommands) != 0 {
		t.Fatalf("old application received %d post-reload commands, want 0", len(oldCommands))
	}

	if generation.config.Branding.AppName != "Reloaded Glance" {
		t.Fatalf(
			"new generation app name = %q, want Reloaded Glance",
			generation.config.Branding.AppName,
		)
	}

	select {
	case <-oldWidget.cancelled:
		t.Fatal("successful reload retired old scheduler before caller committed acceptance")
	default:
	}

	previousRuntime.stop()

	select {
	case <-oldWidget.cancelled:
	case <-time.After(time.Second):
		t.Fatal("old scheduler was not retired after accepted reload")
	}
}

func TestRuntimeGenerationApplicationFailurePreservesCurrentGeneration(t *testing.T) {
	oldWidget := newServerLifecycleTestWidget()
	oldApp := newServerLifecycleTestApplication(t, 0, oldWidget)
	diagnostics := newConfigRuntimeDiagnostics("glance.yml")
	oldApp.configDiagnostics = diagnostics

	server, err := newProcessServer("127.0.0.1", 0, oldApp.router())
	if err != nil {
		t.Fatalf("create process server: %v", err)
	}
	defer func() { _ = server.shutdown() }()
	server.setActiveApplication(oldApp)

	oldRuntime := oldApp.startRuntime()
	defer oldRuntime.stop()

	select {
	case <-oldWidget.started:
	case <-time.After(time.Second):
		t.Fatal("old scheduler did not start")
	}

	generation := &runtimeGeneration{
		runtime: oldRuntime,
		config:  &oldApp.Config,
	}

	candidate := oldApp.Config
	candidate.Auth.Users = map[string]*user{
		"broken": {
			Password: "password",
		},
	}
	candidate.Auth.SecretKey = "not-valid-base64"

	_, err = generation.reload(server, &candidate, diagnostics, nil)
	if err == nil {
		t.Fatal("expected candidate application construction to fail")
	}
	if generation.runtime != oldRuntime {
		t.Fatal("failed candidate replaced current runtime")
	}
	if server.activeApplication.Load() != oldApp {
		t.Fatal("failed candidate replaced active application")
	}

	select {
	case <-oldWidget.cancelled:
		t.Fatal("failed candidate cancelled current scheduler")
	default:
	}
}

func TestInternalFrontendDiagnosticCommandRoutes(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		command string
	}{
		{
			name:    "performance snapshot",
			path:    "/debug/frontend-diagnostics/performance-snapshot",
			command: "performance_snapshot",
		},
		{
			name:    "long task capture",
			path:    "/debug/frontend-diagnostics/long-task-capture",
			command: "long_task_capture",
		},
		{
			name:    "runtime state",
			path:    "/debug/frontend-diagnostics/runtime-state",
			command: "runtime_state",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := newFrontendDiagnosticsTestApplication(true)
			app.liveUpdates = newLiveUpdateBroker()

			subscription, unsubscribe := app.liveUpdates.subscribe(nil)
			defer unsubscribe()

			server := &processServer{}
			server.setActiveApplication(app)
			handler := server.processDiagnosticProfileHandler()

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(
				recorder,
				httptest.NewRequest(http.MethodPost, test.path, nil),
			)

			if recorder.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusAccepted, recorder.Body.String())
			}

			commands := subscription.takeDiagnosticCommands()
			if len(commands) != 1 {
				t.Fatalf("published commands = %d, want 1", len(commands))
			}
			if commands[0].ID == 0 {
				t.Fatal("command ID must be nonzero")
			}
			if commands[0].Command != test.command {
				t.Fatalf("command = %q, want %q", commands[0].Command, test.command)
			}
		})
	}

	t.Run("disabled", func(t *testing.T) {
		app := newFrontendDiagnosticsTestApplication(false)
		app.liveUpdates = newLiveUpdateBroker()

		subscription, unsubscribe := app.liveUpdates.subscribe(nil)
		defer unsubscribe()

		server := &processServer{}
		server.setActiveApplication(app)

		recorder := httptest.NewRecorder()
		server.processDiagnosticProfileHandler().ServeHTTP(
			recorder,
			httptest.NewRequest(http.MethodPost, "/debug/frontend-diagnostics/performance-snapshot", nil),
		)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
		if commands := subscription.takeDiagnosticCommands(); len(commands) != 0 {
			t.Fatalf("disabled diagnostics published %d commands", len(commands))
		}
	})

	t.Run("method restricted", func(t *testing.T) {
		app := newFrontendDiagnosticsTestApplication(true)
		app.liveUpdates = newLiveUpdateBroker()

		server := &processServer{}
		server.setActiveApplication(app)

		recorder := httptest.NewRecorder()
		server.processDiagnosticProfileHandler().ServeHTTP(
			recorder,
			httptest.NewRequest(http.MethodGet, "/debug/frontend-diagnostics/performance-snapshot", nil),
		)

		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("arbitrary command unavailable", func(t *testing.T) {
		app := newFrontendDiagnosticsTestApplication(true)
		app.liveUpdates = newLiveUpdateBroker()

		server := &processServer{}
		server.setActiveApplication(app)

		recorder := httptest.NewRecorder()
		server.processDiagnosticProfileHandler().ServeHTTP(
			recorder,
			httptest.NewRequest(http.MethodPost, "/debug/frontend-diagnostics/arbitrary", nil),
		)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
	})
}

func TestContentionProfilingCanBeEnabledAndDisabled(t *testing.T) {
	originalMutexFraction := runtime.SetMutexProfileFraction(0)
	t.Cleanup(func() {
		runtime.SetMutexProfileFraction(originalMutexFraction)
		runtime.SetBlockProfileRate(0)
	})

	enableContentionProfiling()
	if got := runtime.SetMutexProfileFraction(mutexProfileFraction); got != mutexProfileFraction {
		t.Fatalf("mutex profile fraction after enable = %d, want %d", got, mutexProfileFraction)
	}

	disableContentionProfiling()
	if got := runtime.SetMutexProfileFraction(0); got != 0 {
		t.Fatalf("mutex profile fraction after disable = %d, want 0", got)
	}
}

func TestProcessServerProfilingCanBeReconciledWithoutMainServerImpact(t *testing.T) {
	server, err := newProcessServer("127.0.0.1", 0, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("create process server: %v", err)
	}
	defer func() { _ = server.shutdown() }()

	diagnostics := newProfilingRuntimeDiagnostics()
	server.profileDiagnostics = diagnostics

	server.reconcileProfiling(false)
	got := diagnostics.snapshot()
	if got.Requested || got.Running {
		t.Fatalf("disabled profiling diagnostics = %+v, want not requested and not running", got)
	}

	server.profileMu.Lock()
	profileServer := server.profileServer
	server.profileMu.Unlock()
	if profileServer != nil {
		t.Fatal("profiling server started while disabled")
	}

	server.reconcileProfiling(true)
	server.profileMu.Lock()
	profileServer = server.profileServer
	server.profileMu.Unlock()
	if profileServer == nil {
		t.Fatal("profiling server did not start when enabled")
	}

	got = diagnostics.snapshot()
	if !got.Requested || !got.Running {
		t.Fatalf("enabled profiling diagnostics = %+v, want requested and running", got)
	}

	server.reconcileProfiling(false)
	server.profileMu.Lock()
	profileServer = server.profileServer
	server.profileMu.Unlock()
	if profileServer != nil {
		t.Fatal("profiling server remained configured after disable")
	}

	got = diagnostics.snapshot()
	if got.Requested || got.Running {
		t.Fatalf("disabled profiling diagnostics = %+v, want not requested and not running", got)
	}
}

func TestProcessServerProfilingBindFailureIsNonfatal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:6060")
	if err != nil {
		t.Skipf("cannot reserve profiling port: %v", err)
	}

	server, err := newProcessServer("127.0.0.1", 0, http.NotFoundHandler())
	if err != nil {
		_ = listener.Close()
		t.Fatalf("create process server: %v", err)
	}
	defer func() { _ = server.shutdown() }()

	diagnostics := newProfilingRuntimeDiagnostics()
	server.profileDiagnostics = diagnostics

	server.reconcileProfiling(true)

	deadline := time.Now().Add(time.Second)
	for {
		server.profileMu.Lock()
		profileServer := server.profileServer
		server.profileMu.Unlock()
		if profileServer == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = listener.Close()
			t.Fatal("profiling server did not clear failed bind state")
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := diagnostics.snapshot()
	if !got.Requested || got.Running {
		t.Fatalf("failed profiling diagnostics = %+v, want requested and not running", got)
	}
	if got.LastFailureAt.IsZero() || got.LastFailure == "" {
		t.Fatalf("failed profiling diagnostics = %+v, want recorded failure", got)
	}
	if !strings.Contains(got.LastFailure, "address already in use") {
		t.Fatalf("profiling failure = %q, want address already in use", got.LastFailure)
	}

	recorder := httptest.NewRecorder()
	server.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if recorder.Code != http.StatusNotFound {
		_ = listener.Close()
		t.Fatalf("main handler status after profiling bind failure = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("release profiling port: %v", err)
	}

	server.reconcileProfiling(true)
	server.profileMu.Lock()
	profileServer := server.profileServer
	server.profileMu.Unlock()
	if profileServer == nil {
		t.Fatal("profiling server did not retry after failed bind was cleared")
	}

	got = diagnostics.snapshot()
	if !got.Requested || !got.Running {
		t.Fatalf("retried profiling diagnostics = %+v, want requested and running", got)
	}
	if !got.LastFailureAt.IsZero() || got.LastFailure != "" {
		t.Fatalf("retried profiling diagnostics = %+v, want stale failure cleared", got)
	}
}
