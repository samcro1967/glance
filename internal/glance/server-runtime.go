package glance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	serverShutdownTimeout = 10 * time.Second

	// Contention profiling is diagnostic-only because both profiles add
	// runtime sampling overhead.
	mutexProfileFraction = 5
	blockProfileRate     = 1_000_000
)

func enableContentionProfiling() {
	runtime.SetMutexProfileFraction(mutexProfileFraction)
	runtime.SetBlockProfileRate(blockProfileRate)
}

func disableContentionProfiling() {
	runtime.SetMutexProfileFraction(0)
	runtime.SetBlockProfileRate(0)
}

type swappableHandler struct {
	active atomic.Value
}

func newSwappableHandler(handler http.Handler) *swappableHandler {
	h := &swappableHandler{}
	h.active.Store(handler)
	return h
}

func (h *swappableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.active.Load().(http.Handler).ServeHTTP(w, r)
}

func (h *swappableHandler) swap(handler http.Handler) {
	h.active.Store(handler)
}

type applicationRuntime struct {
	app       *application
	handler   http.Handler
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
}

func (a *application) newRuntime() *applicationRuntime {
	ctx, cancel := context.WithCancel(context.Background())
	return &applicationRuntime{
		app:     a,
		handler: a.router(),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (r *applicationRuntime) start() {
	if r == nil {
		return
	}

	r.startOnce.Do(func() {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			runWidgetRefreshScheduler(
				r.ctx,
				r.app.refreshWidgets,
				widgetRefreshScanInterval,
				widgetRefreshConcurrency,
				r.app.liveUpdates,
			)
		}()

		if r.app.shouldCheckForkReleaseStatus() {
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				runForkReleaseStatusChecker(r.ctx, r.app.Version, &r.app.releaseStatus)
			}()
		}
	})
}

func (a *application) startRuntime() *applicationRuntime {
	runtime := a.newRuntime()
	runtime.start()
	return runtime
}

func (r *applicationRuntime) stop() {
	if r == nil {
		return
	}

	r.stopOnce.Do(func() {
		r.app.liveUpdates.close()
		r.cancel()
		r.wg.Wait()
	})
}

type runtimeGeneration struct {
	runtime *applicationRuntime
	config  *config
}

func (g *runtimeGeneration) reload(
	server *processServer,
	candidateConfig *config,
	diagnostics *configRuntimeDiagnostics,
	profilingDiagnostics *profilingRuntimeDiagnostics,
) (*applicationRuntime, error) {
	if candidateConfig.Server.Host != g.config.Server.Host ||
		candidateConfig.Server.Port != g.config.Server.Port {
		return nil, fmt.Errorf(
			"server.host and server.port require an application restart (running %s:%d, requested %s:%d)",
			g.config.Server.Host,
			g.config.Server.Port,
			candidateConfig.Server.Host,
			candidateConfig.Server.Port,
		)
	}

	var reusableOIDC *oidcRuntime
	if g.runtime != nil && g.runtime.app != nil {
		reusableOIDC = g.runtime.app.oidc
	}

	candidateApp, err := newApplicationWithOIDCRuntime(candidateConfig, reusableOIDC)
	if err != nil {
		return nil, err
	}

	candidateApp.configDiagnostics = diagnostics
	candidateApp.profilingDiagnostics = profilingDiagnostics

	if g.runtime != nil && g.runtime.app != nil &&
		g.runtime.app.personalState != nil &&
		candidateApp.personalState != nil &&
		g.runtime.app.personalState.path == candidateApp.personalState.path {
		candidateApp.personalState = g.runtime.app.personalState
	}

	var reusePlan widgetReloadReusePlan
	if g.runtime != nil && g.runtime.app != nil {
		reusePlan, err = prepareWidgetReloadReusePlan(g.runtime.app, candidateApp)
		if err != nil {
			return nil, fmt.Errorf("preparing widget state reuse: %w", err)
		}
	}

	candidateApp.applyWidgetReloadReusePlan(reusePlan)
	// Prepare the candidate runtime without starting background work. The caller
	// retires the previous runtime after accepting the generation, then starts
	// this runtime. This gives reused widgets exactly one scheduler owner during
	// the reload handoff while still allowing the HTTP handler to switch first.
	candidateRuntime := candidateApp.newRuntime()

	previousRuntime := g.runtime

	server.swap(candidateRuntime.handler)
	server.setActiveApplication(candidateApp)
	g.runtime = candidateRuntime
	g.config = &candidateApp.Config
	server.reconcileProfiling(candidateApp.Config.Server.FrontendDiagnostics)

	return previousRuntime, nil
}

func (g *runtimeGeneration) completeReload(previousRuntime *applicationRuntime) {
	if previousRuntime != nil {
		previousRuntime.stop()
	}
	if g != nil && g.runtime != nil {
		g.runtime.start()
	}
}

type processServer struct {
	listener net.Listener
	server   *http.Server
	handler  *swappableHandler

	activeApplication atomic.Pointer[application]

	profileMu          sync.Mutex
	profileServer      *http.Server
	profileWG          sync.WaitGroup
	profileDiagnostics *profilingRuntimeDiagnostics
}

func newProcessServer(host string, port uint16, initial http.Handler) (*processServer, error) {
	address := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	handler := newSwappableHandler(initial)
	return &processServer{
		listener: listener,
		handler:  handler,
		server: &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
	}, nil
}

func (s *processServer) serve() error {
	err := s.server.Serve(s.listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *processServer) swap(handler http.Handler) {
	s.handler.swap(handler)
}

func (s *processServer) setActiveApplication(app *application) {
	s.activeApplication.Store(app)
}

func (s *processServer) reconcileProfiling(enabled bool) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	if enabled {
		if s.profileServer != nil {
			s.profileDiagnostics.recordRunning()
			return
		}

		profileServer := &http.Server{
			Addr:              "127.0.0.1:6060",
			Handler:           s.processDiagnosticProfileHandler(),
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		s.profileServer = profileServer
		enableContentionProfiling()
		s.profileDiagnostics.recordRunning()
		s.profileWG.Add(1)
		go func() {
			defer s.profileWG.Done()
			slog.Info("Performance profiling server starting", "address", profileServer.Addr)
			if err := profileServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("Performance profiling server stopped unexpectedly", "error", err)
				s.profileDiagnostics.recordFailure(err)
				s.profileMu.Lock()
				if s.profileServer == profileServer {
					s.profileServer = nil
					disableContentionProfiling()
				}
				s.profileMu.Unlock()
			}
		}()
		return
	}

	if s.profileServer == nil {
		disableContentionProfiling()
		s.profileDiagnostics.recordDisabled()
		return
	}

	profileServer := s.profileServer
	s.profileServer = nil
	disableContentionProfiling()
	s.profileDiagnostics.recordDisabled()
	if err := profileServer.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Warn("Failed to stop performance profiling server", "error", err)
	}
}

func (s *processServer) shutdown() error {
	s.profileMu.Lock()
	profileServer := s.profileServer
	s.profileServer = nil
	disableContentionProfiling()
	s.profileMu.Unlock()

	if profileServer != nil {
		if err := profileServer.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("Failed to stop performance profiling server", "error", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()

	err := s.server.Shutdown(ctx)
	if err != nil {
		slog.Warn("Graceful server shutdown did not complete; forcing close", "error", err)
		if closeErr := s.server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			return errors.Join(err, closeErr)
		}
	}

	s.profileWG.Wait()
	return err
}
