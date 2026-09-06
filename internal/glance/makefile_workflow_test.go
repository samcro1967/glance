package glance

import (
	"os"
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

func TestMakefilePRWatchRefreshesHeadDuringPolling(t *testing.T) {
	makefile := readRepositoryMakefile(t)
	recipe := makeTargetRecipe(t, makefile, "pr-watch")

	loop := strings.Index(recipe, `for i in $$(seq 1 "$(CI_RUN_RETRIES)"); do`)
	refresh := strings.Index(recipe, `current_revision="$$(gh pr view "$(PR)" --repo "$(REPO)" --json headRefOid`)
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
