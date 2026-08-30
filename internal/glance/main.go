package glance

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"golang.org/x/crypto/bcrypt"
)

var buildVersion = "dev"

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
		// remove in v0.10.0
		if serveUpdateNoticeIfConfigLocationNotMigrated(options.configPath) {
			return 1
		}

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
	// TODO: refactor if this gets any more complex, the current implementation is
	// difficult to reason about due to all of the callbacks and simultaneous operations,
	// use a single goroutine and a channel to initiate synchronous changes to the server
	exitChannel := make(chan error, 1)
	hadValidConfigOnStartup := false
	var stopServer func() error

	onChange := func(newParsed *parsedYAMLConfig) {
		isReload := stopServer != nil

		if isReload {
			slog.Info("Configuration changed, reloading")
		}

		config, err := newConfigFromParsedYAML(newParsed)
		if err != nil {
			if isReload {
				logConfigDiagnostic(
					slog.LevelWarn,
					"Configuration reload rejected; keeping existing application",
					err,
				)
			} else {
				logConfigDiagnostic(slog.LevelError, "Configuration is invalid", err)
			}

			if !hadValidConfigOnStartup {
				reportExitError(exitChannel, fmt.Errorf("validating config file: %w", err))
			}

			return
		}

		app, err := newApplication(config)
		if err != nil {
			if isReload {
				slog.Warn(
					"Application reload rejected; keeping existing application",
					"error", err,
				)
			} else {
				slog.Error("Failed to create application", "error", err)
			}

			if !hadValidConfigOnStartup {
				reportExitError(exitChannel, fmt.Errorf("creating application: %w", err))
			}

			return
		}

		if !hadValidConfigOnStartup {
			hadValidConfigOnStartup = true
		}

		if stopServer != nil {
			if err := stopServer(); err != nil {
				slog.Error("Failed to stop server during configuration reload", "error", err)
			}
		}

		var startServer func() error
		startServer, stopServer = app.server()
		go startServerAndReport(startServer, exitChannel)

		if isReload {
			slog.Info("Configuration reload accepted")
		} else {
			slog.Info("Application configuration loaded successfully")
		}
	}

	onErr := func(err error) {
		slog.Error("Error watching configuration files", "error", err)
	}

	parsedConfig, err := parseYAMLIncludesWithSources(configPath)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	stopWatching, err := configFilesWatcherWithSources(
		configPath,
		parsedConfig,
		onChange,
		onErr,
	)
	if err == nil {
		defer stopWatching()
	} else {
		slog.Warn(
			"Failed to start configuration file watcher; configuration changes require a manual restart",
			"error", err,
		)

		config, err := newConfigFromParsedYAML(parsedConfig)
		if err != nil {
			return fmt.Errorf("validating config file: %w", err)
		}

		app, err := newApplication(config)
		if err != nil {
			return fmt.Errorf("creating application: %w", err)
		}

		slog.Info("Application configuration loaded successfully")

		startServer, _ := app.server()
		if err := startServer(); err != nil {
			return fmt.Errorf("starting server: %w", err)
		}
	}

	return <-exitChannel
}

func startServerAndReport(startServer func() error, exitChannel chan<- error) {
	if err := startServer(); err != nil {
		slog.Error("Failed to start server", "error", err)
		reportExitError(exitChannel, fmt.Errorf("starting server: %w", err))
	}
}

func reportExitError(exitChannel chan<- error, err error) {
	select {
	case exitChannel <- err:
	default:
	}
}

func serveUpdateNoticeServer(handler http.Handler) error {
	server := http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serving configuration migration notice: %w", err)
	}

	return nil
}

func serveUpdateNoticeIfConfigLocationNotMigrated(configPath string) bool {
	if !isRunningInsideDockerContainer() {
		return false
	}

	if _, err := os.Stat(configPath); err == nil {
		return false
	}

	// glance.yml wasn't mounted to begin with or was incorrectly mounted as a directory
	if stat, err := os.Stat("glance.yml"); err != nil || stat.IsDir() {
		return false
	}

	templateFile, _ := templateFS.Open("v0.7-update-notice-page.html")
	bodyContents, _ := io.ReadAll(templateFile)

	fmt.Println("!!! WARNING !!!")
	fmt.Println("The default location of glance.yml in the Docker image has changed starting from v0.7.0.")
	fmt.Println("Please see https://github.com/glanceapp/glance/blob/main/docs/v0.7.0-upgrade.md for more information.")

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(bodyContents))
	})

	if err := serveUpdateNoticeServer(mux); err != nil {
		fmt.Printf("Failed to serve configuration migration notice: %v\n", err)
	}

	return true
}
