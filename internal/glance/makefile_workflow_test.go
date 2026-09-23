package glance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func readRepositoryMakefile(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "..", "Makefile")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}

	return string(data)
}

func makeTargetRecipe(t *testing.T, makefile, target string) string {
	t.Helper()

	marker := target + ":"
	start := strings.Index(makefile, marker)
	if start == -1 {
		t.Fatalf("target %q not found in Makefile", target)
	}

	rest := makefile[start+len(marker):]
	lines := strings.Split(rest, "\n")
	var recipe []string

	for _, line := range lines {
		if line == "" {
			if len(recipe) > 0 {
				break
			}
			continue
		}

		if !strings.HasPrefix(line, "\t") {
			break
		}

		recipe = append(recipe, line)
	}

	if len(recipe) == 0 {
		t.Fatalf("target %q has no recipe", target)
	}

	return strings.Join(recipe, "\n")
}

func requireRecipeFragmentsInOrder(
	t *testing.T,
	recipe string,
	fragments ...string,
) {
	t.Helper()

	position := 0
	for _, fragment := range fragments {
		index := strings.Index(recipe[position:], fragment)
		if index == -1 {
			t.Fatalf(
				"recipe does not contain %q after previous required fragments:\n%s",
				fragment,
				recipe,
			)
		}

		position += index + len(fragment)
	}
}

func TestMakefileWorkflowFinishTargets(t *testing.T) {
	makefile := readRepositoryMakefile(t)

	tests := []struct {
		name      string
		target    string
		fragments []string
	}{
		{
			name:   "feature to dev",
			target: "pr-finish",
			fragments: []string{
				"set -euo pipefail",
				`base" != "$(DEV_BRANCH)"`,
				`head" = "$(DEV_BRANCH)"`,
				`head" = "$(STABLE_BRANCH)"`,
				`$(MAKE) pr-watch`,
				`$(MAKE) pr-merge`,
				`$(MAKE) post-merge`,
				`$(MAKE) image-watch`,
				`$(MAKE) status`,
			},
		},
		{
			name:   "dev to main",
			target: "promote-finish",
			fragments: []string{
				"set -euo pipefail",
				`head" != "$(DEV_BRANCH)"`,
				`base" != "$(STABLE_BRANCH)"`,
				`$(MAKE) pr-watch`,
				`$(MAKE) pr-merge`,
				`$(MAKE) post-merge`,
				`$(MAKE) verify-main`,
			},
		},
		{
			name:   "main to dev",
			target: "sync-finish",
			fragments: []string{
				"set -euo pipefail",
				`head" != "$(STABLE_BRANCH)"`,
				`base" != "$(DEV_BRANCH)"`,
				`$(MAKE) pr-watch`,
				`$(MAKE) pr-merge`,
				`$(MAKE) post-merge`,
				`$(MAKE) image-watch`,
				`$(MAKE) status`,
			},
		},
		{
			name:   "formal release",
			target: "release-finish",
			fragments: []string{
				"set -euo pipefail",
				`branch" != "$(STABLE_BRANCH)"`,
				`release_tag="$$(git tag --points-at HEAD`,
				`$(MAKE) release`,
				`$(MAKE) release-retry`,
				`$(MAKE) release-watch`,
				`$(MAKE) release-status`,
				`$(MAKE) deploy-status`,
				"Production was NOT deployed.",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe := makeTargetRecipe(t, makefile, test.target)
			requireRecipeFragmentsInOrder(t, recipe, test.fragments...)

			if strings.Contains(recipe, "$(MAKE) deploy-dev") {
				t.Fatalf("%s must not deploy a dev image", test.target)
			}

			for _, line := range strings.Split(recipe, "\n") {
				line = strings.TrimSpace(line)
				if line == `@$(MAKE) deploy` ||
					line == `$(MAKE) deploy` ||
					strings.HasPrefix(line, `@$(MAKE) deploy `) ||
					strings.HasPrefix(line, `$(MAKE) deploy `) {
					t.Fatalf("%s must not deploy production", test.target)
				}
			}
		})
	}
}

func TestMakefileReleaseRecoveryContract(t *testing.T) {
	makefile := readRepositoryMakefile(t)
	retry := makeTargetRecipe(t, makefile, "release-retry")

	requireRecipeFragmentsInOrder(
		t,
		retry,
		"set -euo pipefail",
		`branch" != "$(STABLE_BRANCH)"`,
		`git status --porcelain`,
		`git fetch origin --prune --tags`,
		`revision="$$(git rev-parse HEAD)"`,
		`origin_revision="$$(git rev-parse origin/$(STABLE_BRANCH))"`,
		`release_tag="$$(git tag --points-at HEAD`,
		`run_id="$$(gh run list`,
		`.headSha ==`,
		`$$revision`,
		`.headBranch ==`,
		`$$release_tag`,
		`status="$$(gh run view`,
		`if [ "$$status" != "completed" ]; then`,
		`elif [ "$$conclusion" = "success" ]; then`,
		`gh run rerun "$$run_id"`,
	)

	if strings.Count(retry, `gh run rerun "$$run_id"`) != 1 {
		t.Fatal("release-retry must contain exactly one workflow rerun operation")
	}

	finish := makeTargetRecipe(t, makefile, "release-finish")
	requireRecipeFragmentsInOrder(
		t,
		finish,
		`release_tag="$$(git tag --points-at HEAD`,
		`if [ -z "$$release_tag" ]; then`,
		`$(MAKE) release`,
		`$(MAKE) release-retry`,
		`$(MAKE) release-watch`,
	)

	if strings.Count(finish, "$(MAKE) release-retry") != 1 {
		t.Fatal("release-finish must contain exactly one recovery invocation")
	}
}

func TestMakefileImageVulnerabilityVisibility(t *testing.T) {
	makefile := readRepositoryMakefile(t)

	imageVuln := makeTargetRecipe(t, makefile, "image-vuln")
	for _, fragment := range []string{
		`--only-fixed`,
		`WARNING: CONTAINER VULNERABILITY SCAN UNAVAILABLE`,
		`WARNING: CONTAINER VULNERABILITY SCAN FAILED`,
		`Pipeline will continue`,
	} {
		if !strings.Contains(imageVuln, fragment) {
			t.Fatalf("image-vuln missing informational scan contract %q", fragment)
		}
	}

	prFinish := makeTargetRecipe(t, makefile, "pr-finish")
	requireRecipeFragmentsInOrder(
		t,
		prFinish,
		`$(MAKE) image-watch`,
		`$(MAKE) image-vuln IMAGE="$(DEPLOY_DEV_IMAGE)"`,
	)

	releaseFinish := makeTargetRecipe(t, makefile, "release-finish")
	requireRecipeFragmentsInOrder(
		t,
		releaseFinish,
		`$(MAKE) release-watch`,
		`$(MAKE) image-vuln IMAGE="ghcr.io/$(REPO):$$release_tag"`,
		`$(MAKE) release-status`,
	)

	nonRuntime := makeTargetRecipe(t, makefile, "ship-nonruntime")
	if strings.Contains(nonRuntime, "image-vuln") {
		t.Fatal("ship-nonruntime must not invoke container vulnerability scanning")
	}
}

func TestMakefilePRAutoResolution(t *testing.T) {
	makefile := readRepositoryMakefile(t)

	tests := []struct {
		target string
		head   string
		base   string
	}{
		{"pr-finish", `--head "$$head"`, `--base "$(DEV_BRANCH)"`},
		{"promote-finish", `--head "$(DEV_BRANCH)"`, `--base "$(STABLE_BRANCH)"`},
		{"sync-finish", `--head "$(STABLE_BRANCH)"`, `--base "$(DEV_BRANCH)"`},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			recipe := makeTargetRecipe(t, makefile, test.target)

			if !strings.Contains(recipe, `pr="$(PR)"`) {
				t.Fatalf("%s must preserve explicit PR override", test.target)
			}
			if !strings.Contains(recipe, `python3 scripts/resolve_pr.py`) {
				t.Fatalf("%s must auto-resolve PR when PR is omitted", test.target)
			}
			if !strings.Contains(recipe, test.head) {
				t.Fatalf("%s missing expected resolver head %q", test.target, test.head)
			}
			if !strings.Contains(recipe, test.base) {
				t.Fatalf("%s missing expected resolver base %q", test.target, test.base)
			}
		})
	}

	watch := makeTargetRecipe(t, makefile, "pr-watch")
	if !strings.Contains(watch, `pr="$(PR)"`) {
		t.Fatal("pr-watch must preserve explicit PR override")
	}
	if !strings.Contains(watch, `python3 scripts/resolve_pr.py`) {
		t.Fatal("pr-watch must auto-resolve PR when PR is omitted")
	}
}

func TestMakefileDevFinishTargetsRequireCurrentDev(t *testing.T) {
	makefile := readRepositoryMakefile(t)

	for _, target := range []string{"promote-finish", "sync-finish"} {
		t.Run(target, func(t *testing.T) {
			recipe := makeTargetRecipe(t, makefile, target)

			requireRecipeFragmentsInOrder(
				t,
				recipe,
				`git fetch origin --prune`,
				`local_revision="$$(git rev-parse $(DEV_BRANCH))"`,
				`origin_revision="$$(git rev-parse origin/$(DEV_BRANCH))"`,
				`if [ "$$local_revision" != "$$origin_revision" ]; then`,
				`pr="$(PR)"`,
			)
		})
	}
}

func TestMakefilePRWatchRefreshesHeadDuringPolling(t *testing.T) {
	makefile := readRepositoryMakefile(t)
	recipe := makeTargetRecipe(t, makefile, "pr-watch")

	loop := strings.Index(recipe, `for i in $$(seq 1 "$(CI_RUN_RETRIES)"); do`)
	refresh := strings.Index(recipe, `current_revision="$$(gh pr view "$$pr" --repo "$(REPO)" --json headRefOid`)
	runLookup := strings.Index(recipe, `run_id="$$(gh run list`)

	if loop == -1 {
		t.Fatal("pr-watch missing retry loop")
	}
	if refresh == -1 {
		t.Fatal("pr-watch missing headRefOid refresh")
	}
	if runLookup == -1 {
		t.Fatal("pr-watch missing validation run lookup")
	}
	if refresh < loop || refresh > runLookup {
		t.Fatal("pr-watch must refresh headRefOid inside the retry loop before looking up the validation run")
	}
}

func TestMakefileParkFirstShippingWorkflow(t *testing.T) {
	makefile := readRepositoryMakefile(t)

	for _, target := range []string{"ship", "ship-nonruntime"} {
		t.Run(target, func(t *testing.T) {
			recipe := makeTargetRecipe(t, makefile, target)

			requireRecipeFragmentsInOrder(
				t,
				recipe,
				`current="$$(git branch --show-current)"`,
				`if [ "$$current" != "$(DEV_BRANCH)" ]; then`,
				`git status --porcelain`,
				`git fetch origin --prune`,
				`dev_revision="$$(git rev-parse $(DEV_BRANCH))"`,
				`origin_revision="$$(git rev-parse origin/$(DEV_BRANCH))"`,
				`git merge-base --is-ancestor "$$origin_revision" "$$dev_revision"`,
				`parked="$$(git rev-list --count "$$origin_revision..$$dev_revision")"`,
				`if [ "$$parked" -eq 0 ]; then`,
				`feature="ship/$$(git rev-parse --short=12 "$$dev_revision")"`,
				`git switch -c "$$feature" "$$dev_revision"`,
				`$(MAKE) push`,
				`$(MAKE) pr-finish PR="$$feature_pr"`,
			)
		})
	}
}

func TestMakefileNonRuntimeShippingWorkflow(t *testing.T) {
	makefile := readRepositoryMakefile(t)
	ship := makeTargetRecipe(t, makefile, "ship-nonruntime")

	if strings.Contains(makefile, "ship-docs") {
		t.Fatal("obsolete ship-docs workflow must not remain")
	}

	requireRecipeFragmentsInOrder(
		t,
		ship,
		`git diff --name-only "$$origin_revision...$$dev_revision"`,
		`python3 scripts/check_nonruntime_changes.py`,
		`feature="ship/$$(git rev-parse --short=12 "$$dev_revision")"`,
		`$(MAKE) push`,
		`$(MAKE) pr-finish PR="$$feature_pr" SKIP_IMAGE_WATCH=1`,
		`$(MAKE) promote-finish PR="$$promotion_pr"`,
		`$(MAKE) sync-finish PR="$$sync_pr" SKIP_IMAGE_WATCH=1`,
	)

	for _, forbidden := range []string{
		"release-finish",
		"deploy-finish",
		"$(MAKE) release ",
		"$(MAKE) deploy ",
	} {
		if strings.Contains(ship, forbidden) {
			t.Fatalf("ship-nonruntime must not invoke runtime release/deployment operation %q", forbidden)
		}
	}
}

func TestNonRuntimeChangeClassifier(t *testing.T) {
	t.Run("accepts non-runtime paths", func(t *testing.T) {
		input := strings.Join([]string{
			"Makefile",
			"README.md",
			"CONTRIBUTING.md",
			"CODE_OF_CONDUCT.md",
			"LICENSE",
			".github/workflows/ci.yml",
			".golangci.yml",
			"test-instance.yml",
			"scripts/check_docs.py",
			"testdata/visual/run.sh",
			"internal/glance/widget_test.go",
			"pkg/sysinfo/sysinfo_test.go",
		}, "\n") + "\n"

		cmd := exec.Command("python3", "../../scripts/check_nonruntime_changes.py")
		cmd.Stdin = strings.NewReader(input)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("non-runtime classifier rejected approved paths: %v\n%s", err, output)
		}
	})

	t.Run("rejects runtime and unknown paths", func(t *testing.T) {
		input := strings.Join([]string{
			"internal/glance/widget.go",
			"internal/glance/static/js/main.js",
			"internal/glance/templates/page.html",
			"pkg/sysinfo/sysinfo.go",
			"main.go",
			"Dockerfile",
			".dockerignore",
			"Dockerfile.goreleaser",
			".goreleaser.yaml",
			"go.mod",
			"go.sum",
			"unknown.future.path",
		}, "\n") + "\n"

		cmd := exec.Command("python3", "../../scripts/check_nonruntime_changes.py")
		cmd.Stdin = strings.NewReader(input)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("non-runtime classifier accepted runtime paths:\n%s", output)
		}

		for _, path := range strings.Split(strings.TrimSpace(input), "\n") {
			if !strings.Contains(string(output), path) {
				t.Fatalf("classifier rejection did not report %q:\n%s", path, output)
			}
		}
	})

	t.Run("rejects empty input", func(t *testing.T) {
		cmd := exec.Command("python3", "../../scripts/check_nonruntime_changes.py")
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("non-runtime classifier accepted empty input:\n%s", output)
		}
	})
}

func TestMakefileCanonicalTestEnvironments(t *testing.T) {
	makefile := readRepositoryMakefile(t)

	for _, contract := range []string{
		"TEST_CONFIG ?= test-instance.yml",
		"TEST_RUNTIME_CONTAINER ?= $(DEPLOY_CONTAINER)",
		"TEST_PROD_CONFIG_OVERRIDE ?= true",
		"TEST_PROD_CONFIG_APPEND_FILE ?= test-prod.yml",
		"test-instance = deterministic current source",
		"test-prod     = current source + production runtime/integrations",
		"test-container= published dev artifact",
		"test-prod.yml is local/ignored and must never be committed",
		"TEST_PROD_EXTRA_ENV ?= GLANCE_OIDC_CLIENT_ID GLANCE_OIDC_CLIENT_SECRET",
		"OIDC test credentials are loaded from .env.test;",
		"for key in $(TEST_PROD_EXTRA_ENV)",
	} {
		if !strings.Contains(makefile, contract) {
			t.Fatalf("Makefile missing canonical test-environment contract %q", contract)
		}
	}

	for _, obsolete := range []string{
		"test-external-start",
		"test-external-status",
		"test-external-stop",
		"TEST_EXTERNAL_URL",
		"TEST_EXTERNAL_ANALYTICS_ENDPOINT",
	} {
		if strings.Contains(makefile, obsolete) {
			t.Fatalf("obsolete test workflow must not remain: %s", obsolete)
		}
	}

	start := makeTargetRecipe(t, makefile, "test-prod-start")
	refresh := makeTargetRecipe(t, makefile, "test-prod-config-refresh")

	for _, recipe := range []struct {
		name string
		body string
	}{
		{name: "test-prod-start", body: start},
		{name: "test-prod-config-refresh", body: refresh},
	} {
		for _, contract := range []string{
			"python3 scripts/prepare_test_prod_config.py",
			"--frontend-diagnostics \"$(TEST_FRONTEND_DIAGNOSTICS)\"",
			"--https \"$(TEST_PROD_HTTPS)\"",
			"--resource-proxy-origins \"$${TEST_PROD_RESOURCE_PROXY_ORIGINS:-}\"",
		} {
			if !strings.Contains(recipe.body, contract) {
				t.Errorf("%s missing centralized config-preparation contract %q", recipe.name, contract)
			}
		}
	}

	for _, obsolete := range []string{
		"diagnostics_count=",
		"sed -i \"/^server:",
		"cat \"$(TEST_PROD_CONFIG_APPEND_FILE)\"",
	} {
		if strings.Contains(start, obsolete) || strings.Contains(refresh, obsolete) {
			t.Errorf("test-prod config preparation must not restore inline mutation logic %q", obsolete)
		}
	}
}

func TestCanonicalTestConfigurationPrivacy(t *testing.T) {
	root := filepath.Join("..", "..")

	instancePath := filepath.Join(root, "test-instance.yml")
	if _, err := os.Stat(instancePath); err != nil {
		t.Fatalf("canonical deterministic fixture missing: %v", err)
	}

	cmd := exec.Command("git", "check-ignore", "-q", "test-prod.yml")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatal("test-prod.yml must remain explicitly ignored")
	}

	cmd = exec.Command("git", "ls-files", "--error-unmatch", "test-prod.yml")
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		t.Fatal("test-prod.yml must never be tracked")
	}
}

func TestPrepareTestProdConfig(t *testing.T) {
	root := filepath.Join("..", "..")
	temp := t.TempDir()

	source := filepath.Join(temp, "source.yml")
	overlay := filepath.Join(temp, "overlay.yml")
	destination := filepath.Join(temp, "result.yml")

	if err := os.WriteFile(source, []byte(`server:
  assets-path: /app/assets
  frontend-diagnostics: false
branding:
  app-name: Test
document:
  head: |
    https: untouched
`), 0o600); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	if err := os.WriteFile(overlay, []byte(`auth:
  oidc:
    issuer: https://accounts.example.test
analytics:
  provider: goatcounter
  endpoint: https://analytics.example.test
`), 0o600); err != nil {
		t.Fatalf("write overlay config: %v", err)
	}

	cmd := exec.Command(
		"python3",
		filepath.Join(root, "scripts", "prepare_test_prod_config.py"),
		"--source", source,
		"--destination", destination,
		"--overlay", overlay,
		"--frontend-diagnostics", "true",
		"--https", "false",
		"--resource-proxy-origins",
		"http://osu.plex:32400 http://osu.sonarr:8079 http://osu.radarr:8095",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare test-prod config failed: %v\n%s", err, output)
	}

	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read prepared config: %v", err)
	}
	config := string(data)

	for _, expected := range []string{
		"  assets-path: /app/assets",
		"  frontend-diagnostics: true",
		"    https: untouched",
		"  resource-proxy:",
		"    allowed-origins:",
		"      - http://osu.plex:32400",
		"      - http://osu.sonarr:8079",
		"      - http://osu.radarr:8095",
		"auth:",
		"analytics:",
	} {
		if !strings.Contains(config, expected) {
			t.Errorf("prepared config missing %q:\n%s", expected, config)
		}
	}

	if strings.Contains(config, "  https: true") ||
		strings.Contains(config, "  https: false") {
		t.Errorf("disabled test HTTPS unexpectedly changed server config:\n%s", config)
	}

	t.Run("rejects existing resource proxy", func(t *testing.T) {
		source := filepath.Join(temp, "existing-resource-proxy.yml")
		destination := filepath.Join(temp, "existing-resource-proxy-result.yml")

		if err := os.WriteFile(source, []byte(`server:
  resource-proxy:
    allowed-origins:
      - http://existing.test
`), 0o600); err != nil {
			t.Fatalf("write source config: %v", err)
		}

		cmd := exec.Command(
			"python3",
			filepath.Join(root, "scripts", "prepare_test_prod_config.py"),
			"--source", source,
			"--destination", destination,
			"--resource-proxy-origins", "http://new.test",
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("existing resource proxy was accepted:\n%s", output)
		}
		if !strings.Contains(string(output), "production config already contains server resource-proxy") {
			t.Fatalf("unexpected existing resource-proxy error:\n%s", output)
		}
	})

	t.Run("rejects duplicate server mappings", func(t *testing.T) {
		source := filepath.Join(temp, "duplicate-server.yml")
		destination := filepath.Join(temp, "duplicate-server-result.yml")

		if err := os.WriteFile(source, []byte(`server:
  port: 8080
server:
  port: 8081
`), 0o600); err != nil {
			t.Fatalf("write source config: %v", err)
		}

		cmd := exec.Command(
			"python3",
			filepath.Join(root, "scripts", "prepare_test_prod_config.py"),
			"--source", source,
			"--destination", destination,
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("duplicate server mappings were accepted:\n%s", output)
		}
		if !strings.Contains(string(output), "expected exactly one top-level server mapping; found 2") {
			t.Fatalf("unexpected duplicate-server error:\n%s", output)
		}
	})
}

func TestMakefileTestContainerUsesPublishedDevArtifact(t *testing.T) {
	makefile := readRepositoryMakefile(t)
	start := makeTargetRecipe(t, makefile, "test-container-start")
	stop := makeTargetRecipe(t, makefile, "test-container-stop")

	if !strings.Contains(makefile, "TEST_CONTAINER_IMAGE ?= $(DEPLOY_DEV_IMAGE)") {
		t.Fatal("test container must use the published development image")
	}

	requireRecipeFragmentsInOrder(
		t,
		start,
		`git fetch origin --prune`,
		`dev_revision="$$(git rev-parse origin/$(DEV_BRANCH))"`,
		`run_id="$$(gh run list`,
		`docker pull "$(TEST_CONTAINER_IMAGE)"`,
		`image_version="$$(docker run --rm --entrypoint /app/glance "$(TEST_CONTAINER_IMAGE)" --version)"`,
		`-p "$(TEST_CONTAINER_PORT):8080"`,
		`"$(TEST_CONTAINER_IMAGE)" >/dev/null`,
		`short_revision="$${dev_revision:0:7}"`,
		`grep -Fq "revision=$$short_revision"`,
	)

	if strings.Contains(start, "docker build") {
		t.Fatal("test-container-start must not rebuild source locally")
	}

	if strings.Contains(stop, "docker image rm") {
		t.Fatal("test-container-stop must preserve the shared development image")
	}
}
