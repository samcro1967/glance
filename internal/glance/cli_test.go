package glance

import (
	"os"
	"strings"
	"testing"
)

func withCLIArgs(t *testing.T, args ...string) {
	t.Helper()

	originalArgs := os.Args
	os.Args = append([]string{"glance"}, args...)

	t.Cleanup(func() {
		os.Args = originalArgs
	})
}

func TestParseCliOptionsVersion(t *testing.T) {
	for _, arg := range []string{"--version", "-v", "version"} {
		t.Run(arg, func(t *testing.T) {
			withCLIArgs(t, arg)

			options, err := parseCliOptions()
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if options.intent != cliIntentVersionPrint {
				t.Fatalf(
					"intent = %d, want %d",
					options.intent,
					cliIntentVersionPrint,
				)
			}
		})
	}
}

func TestParseCliOptionsServeDefaults(t *testing.T) {
	withCLIArgs(t)

	options, err := parseCliOptions()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if options.intent != cliIntentServe {
		t.Fatalf("intent = %d, want %d", options.intent, cliIntentServe)
	}

	if options.configPath != "glance.yml" {
		t.Fatalf("config path = %q, want %q", options.configPath, "glance.yml")
	}

	if len(options.args) != 0 {
		t.Fatalf("args = %#v, want no positional arguments", options.args)
	}
}

func TestParseCliOptionsCustomConfigPath(t *testing.T) {
	withCLIArgs(t, "--config", "/tmp/config.yml")

	options, err := parseCliOptions()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if options.intent != cliIntentServe {
		t.Fatalf("intent = %d, want %d", options.intent, cliIntentServe)
	}

	if options.configPath != "/tmp/config.yml" {
		t.Fatalf(
			"config path = %q, want %q",
			options.configPath,
			"/tmp/config.yml",
		)
	}
}

func TestParseCliOptionsSingleArgumentCommands(t *testing.T) {
	tests := []struct {
		command string
		intent  cliIntent
	}{
		{
			command: "config:validate",
			intent:  cliIntentConfigValidate,
		},
		{
			command: "config:print",
			intent:  cliIntentConfigPrint,
		},
		{
			command: "sensors:print",
			intent:  cliIntentSensorsPrint,
		},
		{
			command: "diagnose",
			intent:  cliIntentDiagnose,
		},
		{
			command: "secret:make",
			intent:  cliIntentSecretMake,
		},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			withCLIArgs(t, tt.command)

			options, err := parseCliOptions()
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if options.intent != tt.intent {
				t.Fatalf("intent = %d, want %d", options.intent, tt.intent)
			}

			if len(options.args) != 1 || options.args[0] != tt.command {
				t.Fatalf(
					"args = %#v, want [%q]",
					options.args,
					tt.command,
				)
			}
		})
	}
}

func TestParseCliOptionsPasswordHash(t *testing.T) {
	withCLIArgs(t, "password:hash", "example-password")

	options, err := parseCliOptions()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if options.intent != cliIntentPasswordHash {
		t.Fatalf(
			"intent = %d, want %d",
			options.intent,
			cliIntentPasswordHash,
		)
	}

	if len(options.args) != 2 {
		t.Fatalf("args = %#v, want two positional arguments", options.args)
	}

	if options.args[0] != "password:hash" {
		t.Fatalf("args[0] = %q, want %q", options.args[0], "password:hash")
	}

	if options.args[1] != "example-password" {
		t.Fatalf(
			"args[1] = %q, want %q",
			options.args[1],
			"example-password",
		)
	}
}

func TestParseCliOptionsMountpointInfo(t *testing.T) {
	const mountpoint = "/mnt/storage/media"

	withCLIArgs(t, "mountpoint:info", mountpoint)

	options, err := parseCliOptions()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if options.intent != cliIntentMountpointInfo {
		t.Fatalf(
			"intent = %d, want %d",
			options.intent,
			cliIntentMountpointInfo,
		)
	}

	if len(options.args) != 2 {
		t.Fatalf(
			"args = %#v, want mountpoint command and path",
			options.args,
		)
	}

	if options.args[0] != "mountpoint:info" {
		t.Fatalf(
			"args[0] = %q, want %q",
			options.args[0],
			"mountpoint:info",
		)
	}

	if options.args[1] != mountpoint {
		t.Fatalf("args[1] = %q, want %q", options.args[1], mountpoint)
	}
}

func TestParseCliOptionsMountpointInfoWithConfigFlag(t *testing.T) {
	const (
		configPath = "/tmp/glance.yml"
		mountpoint = "/mnt/storage"
	)

	withCLIArgs(
		t,
		"--config",
		configPath,
		"mountpoint:info",
		mountpoint,
	)

	options, err := parseCliOptions()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if options.intent != cliIntentMountpointInfo {
		t.Fatalf(
			"intent = %d, want %d",
			options.intent,
			cliIntentMountpointInfo,
		)
	}

	if options.configPath != configPath {
		t.Fatalf(
			"config path = %q, want %q",
			options.configPath,
			configPath,
		)
	}

	if len(options.args) != 2 || options.args[1] != mountpoint {
		t.Fatalf(
			"args = %#v, want mountpoint command followed by %q",
			options.args,
			mountpoint,
		)
	}
}

func TestParseCliOptionsRejectsIncompleteTwoArgumentCommands(t *testing.T) {
	for _, command := range []string{"password:hash", "mountpoint:info"} {
		t.Run(command, func(t *testing.T) {
			withCLIArgs(t, command)

			_, err := parseCliOptions()
			if err == nil {
				t.Fatal("expected incomplete command error")
			}

			want := "unknown command: " + command
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err, want)
			}
		})
	}
}

func TestParseCliOptionsRejectsUnknownCommand(t *testing.T) {
	withCLIArgs(t, "unknown:command")

	_, err := parseCliOptions()
	if err == nil {
		t.Fatal("expected unknown command error")
	}

	if err.Error() != "unknown command: unknown:command" {
		t.Fatalf(
			"error = %q, want %q",
			err,
			"unknown command: unknown:command",
		)
	}
}

func TestParseCliOptionsRejectsUnexpectedArguments(t *testing.T) {
	withCLIArgs(t, "diagnose", "unexpected")

	_, err := parseCliOptions()
	if err == nil {
		t.Fatal("expected unexpected arguments error")
	}

	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %q, want unknown command diagnostic", err)
	}

	if !strings.Contains(err.Error(), "diagnose unexpected") {
		t.Fatalf("error = %q, want rejected arguments in diagnostic", err)
	}
}
