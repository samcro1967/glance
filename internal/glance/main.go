package glance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var buildVersion = "dev"
var buildRevision = ""

func Main() int {
	options, err := parseCliOptions()
	if err != nil {
		fmt.Println(err)
		return 1
	}

	switch options.intent {
	case cliIntentVersionPrint:
		fmt.Println(buildVersion)
	case cliIntentServe:
		if err := serveApp(options.configPath); err != nil {
			fmt.Println(err)
			return 1
		}
	case cliIntentConfigValidate:
		parsed, err := parseYAMLIncludesWithSources(options.configPath)
		if err != nil {
			fmt.Printf("Could not parse config file: %v\n", err)
			return 1
		}

		if _, err := newConfigFromParsedYAML(parsed); err != nil {
			printConfigValidationError(err)
			return 1
		}
	case cliIntentConfigPrint:
		parsed, err := parseYAMLIncludesWithSources(options.configPath)
		if err != nil {
			fmt.Printf("Could not parse config file: %v\n", err)
			return 1
		}

		fmt.Println(string(parsed.Contents))
	case cliIntentSensorsPrint:
		return cliSensorsPrint()
	case cliIntentMountpointInfo:
		return cliMountpointInfo(options.args[1])
	case cliIntentDiagnose:
		runDiagnostic()
	case cliIntentSecretMake:
		key, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
		if err != nil {
			fmt.Printf("Failed to make secret key: %v\n", err)
			return 1
		}

		fmt.Println(key)
	case cliIntentPasswordHash:
		password := options.args[1]

		if password == "" {
			fmt.Println("Password cannot be empty")
			return 1
		}

		if len(password) < 6 {
			fmt.Println("Password must be at least 6 characters long")
			return 1
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			fmt.Printf("Failed to hash password: %v\n", err)
			return 1
		}

		fmt.Println(string(hashedPassword))
	}

	return 0
}

func printConfigValidationError(err error) {
	var diagnostic *configDiagnostic
	if errors.As(err, &diagnostic) {
		fmt.Println("Configuration is invalid:")
		if diagnostic.File != "" {
			fmt.Printf("  file: %s\n", diagnostic.File)
		}
		if diagnostic.Line > 0 {
			fmt.Printf("  line: %d\n", diagnostic.Line)
		}
		fmt.Printf("  error: %s\n", diagnostic.Message)
		return
	}

	fmt.Printf("Config file is invalid: %v\n", err)
}

func logConfigDiagnostic(level slog.Level, message string, err error) {
	var diagnostic *configDiagnostic
	attrs := make([]any, 0, 6)

	if errors.As(err, &diagnostic) {
		if diagnostic.File != "" {
			attrs = append(attrs, "file", diagnostic.File)
		}
		if diagnostic.Line > 0 {
			attrs = append(attrs, "line", diagnostic.Line)
		}
		attrs = append(attrs, "error", diagnostic.Message)
	} else {
		attrs = append(attrs, "error", err)
	}

	switch {
	case level >= slog.LevelError:
		slog.Error(message, attrs...)
	case level >= slog.LevelWarn:
		slog.Warn(message, attrs...)
	case level >= slog.LevelInfo:
		slog.Info(message, attrs...)
	default:
		slog.Debug(message, attrs...)
	}
}

func serveApp(configPath string) error {
	slog.Info(
		"Application starting",
		"version", buildVersion,
		"revision", shortBuildRevision(buildRevision),
		"repository", "https://github.com/samcro1967/glance",
		"config", configPath,
	)

	parsedConfig, err := parseYAMLIncludesWithSources(configPath)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	initialConfig, err := newConfigFromParsedYAML(parsedConfig)
	if err != nil {
		return fmt.Errorf("validating config file: %w", err)
	}

	initialApp, err := newApplication(initialConfig)
	if err != nil {
		return fmt.Errorf("creating application: %w", err)
	}

	configDiagnostics := newConfigRuntimeDiagnostics(configPath)
	profilingDiagnostics := newProfilingRuntimeDiagnostics()
	initialApp.configDiagnostics = configDiagnostics
	initialApp.profilingDiagnostics = profilingDiagnostics

	initialHandler := initialApp.router()
	server, err := newProcessServer(
		initialApp.Config.Server.Host,
		initialApp.Config.Server.Port,
		initialHandler,
	)
	if err != nil {
		return fmt.Errorf("starting server: %w", err)
	}
	server.profileDiagnostics = profilingDiagnostics
	server.setActiveApplication(initialApp)

	initialRuntime := initialApp.startRuntime()
	generation := &runtimeGeneration{
		runtime: initialRuntime,
		config:  &initialApp.Config,
	}
	var reloadMu sync.Mutex

	server.reconcileProfiling(initialApp.Config.Server.FrontendDiagnostics)
	configDiagnostics.recordLoaded(time.Now())

	var absAssetsPath string
	if initialApp.Config.Server.AssetsPath != "" {
		absAssetsPath, _ = filepath.Abs(initialApp.Config.Server.AssetsPath)
	}

	slog.Info(
		"Server starting",
		"host", initialApp.Config.Server.Host,
		"port", initialApp.Config.Server.Port,
		"base_url", initialApp.Config.Server.BaseURL,
		"assets_path", absAssetsPath,
	)
	slog.Info("Application configuration loaded successfully")

	exitChannel := make(chan error, 1)
	go func() {
		if err := server.serve(); err != nil {
			slog.Error("Server stopped unexpectedly", "error", err)
			reportExitError(exitChannel, fmt.Errorf("serving application: %w", err))
			return
		}
		reportExitError(exitChannel, nil)
	}()

	onChange := func(newParsed *parsedYAMLConfig) {
		reloadMu.Lock()
		defer reloadMu.Unlock()

		configDiagnostics.recordReloadAttempt(time.Now())
		slog.Info("Configuration changed, reloading")

		candidateConfig, err := newConfigFromParsedYAML(newParsed)
		if err != nil {
			configDiagnostics.recordReloadRejected(err)
			logConfigDiagnostic(
				slog.LevelWarn,
				"Configuration reload rejected; keeping existing application",
				err,
			)
			return
		}

		previousRuntime, err := generation.reload(server, candidateConfig, configDiagnostics, profilingDiagnostics)
		if err != nil {
			configDiagnostics.recordReloadRejected(err)
			slog.Warn(
				"Application reload rejected; keeping existing application",
				"error", err,
			)
			return
		}

		configDiagnostics.recordReloadAccepted(time.Now())
		slog.Info("Configuration reload accepted")
		previousRuntime.stop()
	}

	onErr := func(err error) {
		slog.Error("Error watching configuration files", "error", err)
	}

	stopWatching, watchErr := configFilesWatcherWithSources(
		configPath,
		parsedConfig,
		onChange,
		onErr,
	)
	if watchErr == nil {
		defer func() {
			if err := stopWatching(); err != nil {
				slog.Warn("Failed to stop configuration file watcher", "error", err)
			}
		}()
	} else {
		slog.Warn(
			"Failed to start configuration file watcher; configuration changes require a manual restart",
			"error", watchErr,
		)
	}

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	var serveErr error
	select {
	case serveErr = <-exitChannel:
	case <-signalCtx.Done():
		slog.Info("Shutdown signal received")
	}

	reloadMu.Lock()
	generation.runtime.stop()
	reloadMu.Unlock()

	if err := server.shutdown(); err != nil {
		if serveErr != nil {
			return errors.Join(serveErr, err)
		}
		return fmt.Errorf("shutting down server: %w", err)
	}

	if serveErr != nil {
		return serveErr
	}

	slog.Info("Server stopped")
	return nil
}

func reportExitError(exitChannel chan<- error, err error) {
	select {
	case exitChannel <- err:
	default:
	}
}
