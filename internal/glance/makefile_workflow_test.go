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
				`$(MAKE) release`,
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

func TestMakefileNonRuntimeShippingWorkflow(t *testing.T) {
	makefile := readRepositoryMakefile(t)
	ship := makeTargetRecipe(t, makefile, "ship-nonruntime")

	if strings.Contains(makefile, "ship-docs") {
		t.Fatal("obsolete ship-docs workflow must not remain")
	}

	requireRecipeFragmentsInOrder(
		t,
		ship,
		`git diff --name-only origin/$(DEV_BRANCH)...HEAD`,
		`python3 scripts/check_nonruntime_changes.py`,
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
			"glance-test.yml",
			"glance-test-auth.yml",
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
