SHELL := /bin/bash

export GIT_PAGER := cat
export PAGER := cat
export GH_PAGER := cat
export GIT_EDITOR := true
export GIT_MERGE_AUTOEDIT := no

.PHONY: help deps build goreleaser-check frontend-audit frontend-check test-instance-fixture-start test-instance-fixture-stop test-instance-start test-instance-status test-instance-stop test-prod-start test-prod-status test-prod-stop test test-race test-count test-race-count fmt-check diff-check staged-check docs-check check coverage vuln status staged-diff upstream-status upstream-dev-status branch push pr-create promote-create sync-dev-create pr-view pr-runs pr-watch pr-merge post-merge image-runs image-watch release-runs release-watch ci-watch ci-view verify-dev verify-main release-status release-check release deploy-status deploy-dev deploy pr-finish promote-finish sync-finish release-finish ship deploy-finish workflow-status visual-check visual-screenshots visual-docs visual-docs-promote visual-all visual-final

COUNT ?= 10
COVERAGE_FILE ?= coverage.out
BASE_REF ?= origin/dev

TEST_PORT := 18080
TEST_BINARY ?= .glance-test

TEST_ENV_FILE ?= .env.test

ifneq (,$(wildcard $(TEST_ENV_FILE)))
include $(TEST_ENV_FILE)
export
endif

TEST_CONFIG ?= glance-test.yml
TEST_PID_FILE ?= .glance-test.pid
TEST_LOG ?= .glance-test.log
TEST_URL ?= http://127.0.0.1:$(TEST_PORT)
TEST_CONTAINER ?= glance-test
TEST_CONTAINER_IMAGE ?= $(DEPLOY_DEV_IMAGE)
TEST_CONTAINER_PORT ?= 18080
TEST_CONTAINER_URL ?= http://127.0.0.1:$(TEST_CONTAINER_PORT)
TEST_RUNTIME_CONTAINER ?=
TEST_PROD_IMAGE ?= glance-prod-test:local
TEST_PROD_CONTAINER ?= glance-prod-test

CI_RUN_RETRIES ?= 12
CI_RUN_RETRY_DELAY ?= 5

REPO ?= samcro1967/glance
PR_WORKFLOW ?= 345456314
IMAGE_WORKFLOW ?= 344869583
RELEASE_WORKFLOW ?= 344853462
BRANCH ?= $(shell git branch --show-current 2>/dev/null)
NEW_BRANCH ?=

DEV_BRANCH ?= dev
STABLE_BRANCH ?= main
PR_BASE ?= $(DEV_BRANCH)

FORK_RELEASE_ID ?= samcro1967
FORK_RELEASE_WIDTH ?= 3
GORELEASER_VERSION ?= v2.18.1

DEPLOY_IMAGE ?= ghcr.io/samcro1967/glance:latest
DEPLOY_DEV_IMAGE ?= ghcr.io/samcro1967/glance:dev
DEPLOY_CONTAINER ?= glance
DEPLOY_SERVICE ?= glance
DEPLOY_DIR ?= ..
DEPLOY_COMPOSE_FILE ?= docker-compose.yml
DEPLOY_URL ?= http://127.0.0.1:8092/
DEPLOY_RETRIES ?= 6
DEPLOY_RETRY_DELAY ?= 2

help:
	@echo "GLANCE FORK WORKFLOW"
	@echo
	@echo "SAFE END-TO-END STAGES:"
	@echo "  make ship TITLE='Description' [BODY_FILE=file]"
	@echo "                                Feature -> dev -> main -> formal release; NEVER deploys"
	@echo "                                BODY_FILE optionally supplies the feature PR body"
	@echo "  make deploy-finish            Deploy formal release -> sync main back to dev -> final verification"
	@echo
	@echo "NORMAL WORKFLOW:"
	@echo "  make branch NEW_BRANCH=feature/name"
	@echo "                                Clean local dev may contain committed parked work"
	@echo "                                when origin/dev is its ancestor; parked commits are included"
	@echo "  ... edit, stage, commit ..."
	@echo "  make ship TITLE='Description' [BODY_FILE=file]"
	@echo "  make deploy-finish"
	@echo
	@echo "RECOVERY / INDIVIDUAL STAGES:"
	@echo "  make pr-finish [PR=55]        feature -> dev: auto-resolve PR, CI, merge, cleanup, dev image"
	@echo "  make promote-finish [PR=56]   dev -> main: auto-resolve PR, CI, merge, update/verify main"
	@echo "  make release-finish           main: validate, tag, push, watch formal release"
	@echo "                               DOES NOT deploy production"
	@echo "  make sync-finish [PR=57]      main -> dev: auto-resolve PR, CI, merge, update dev, dev image"
	@echo "  make workflow-status          Combined repository/release/CI/deployment status"
	@echo
	@echo "LIFECYCLE:"
	@echo "  feature -> dev PR -> PR CI -> merge -> dev image"
	@echo "  dev -> main PR -> PR CI -> merge -> formal release -> release image"
	@echo "  explicit production deploy"
	@echo "  main -> dev sync PR -> PR CI -> merge -> dev image"
	@echo
	@echo "SAFEGUARDS:"
	@echo "  Composite stages fail immediately when any required command fails."
	@echo "  PR finish targets verify feature/dev/main direction before merging."
	@echo "  PR CI watches match the exact PR head SHA, not merely the branch name."
	@echo "  Dev image watches match the exact current dev SHA."
	@echo "  Feature push refuses dev/main and requires a clean worktree."
	@echo "  Branch creation permits parked local dev commits only when origin/dev is an ancestor."
	@echo "  Feature post-merge reconciles parked dev history only after verifying it reached origin/dev."
	@echo "  Release requires clean/current main containing origin/dev and upstream/main."
	@echo "  Release refuses an existing release tag and runs full make check."
	@echo "  Deploy requires current main to have a formal release tag."
	@echo "  Deploy pulls latest and verifies its embedded version matches that tag."
	@echo "  Deploy verifies running container version, image ID, and HTTP readiness."
	@echo "  dev/main are preserved; merged feature branches are cleaned."
	@echo "  No composite PR/release stage deploys production."
	@echo
	@echo "DEVELOPMENT:"
	@echo "  make deps                     Download Go module dependencies"
	@echo "  make build                    Build all Go packages"
	@echo "  make test-instance-start      Build current source and run local binary with deterministic test config"
	@echo "  make test-instance-status     Show local deterministic test instance status"
	@echo "  make test-instance-stop       Stop local deterministic test instance and remove runtime artifacts"
	@echo "  make test-prod-start TEST_RUNTIME_CONTAINER=name"
	@echo "                                Build current source into an isolated test container using"
	@echo "                                the named production container as its runtime reference"
	@echo "                                Production is not modified or replaced"
	@echo "  make test-prod-status         Show isolated production-runtime test container status"
	@echo "  make test-prod-stop           Remove isolated production-runtime test container and local image"
	@echo "  make test-container-start TEST_RUNTIME_CONTAINER=name"
	@echo "                                Pull and start isolated published dev container"
	@echo "  make test-container-status    Show isolated container status"
	@echo "  make test-container-stop      Remove isolated container"
	@echo
	@echo "VISUAL QA / DOCUMENTATION:"
	@echo "  make visual-check             Validate visual QA and documentation contracts"
	@echo "  make visual-screenshots       Capture canonical QA pages and widgets"
	@echo "  make visual-docs              Stage documentation screenshots for review"
	@echo "                                NEVER modifies docs/images"
	@echo "  make visual-docs-promote      Promote approved staged images into docs/images"
	@echo "  make visual-all               Capture QA + stage documentation screenshots"
	@echo "                                NEVER promotes documentation images"
	@echo
	@echo "TESTING / VALIDATION:"
	@echo "  make test                     Go tests"
	@echo "  make test-race                Go tests with race detector"
	@echo "  make test-count COUNT=10      Repeated Go tests"
	@echo "  make test-race-count COUNT=10 Repeated race tests"
	@echo "  make coverage                 Generate test coverage"
	@echo "  make vuln                     Go vulnerability analysis"
	@echo "  make fmt-check                Verify changed Go files are formatted"
	@echo "  make diff-check               Working-tree whitespace validation"
	@echo "  make staged-check             Staged whitespace validation"
	@echo "  make check                    Tests + race + build + format + whitespace + docs"
	@echo "  make goreleaser-check         Validate formal-release configuration"
	@echo
	@echo "REPOSITORY:"
	@echo "  make status                   Branch, HEAD, worktree"
	@echo "  make staged-diff              Staged summary and diff"
	@echo "  make upstream-status          dev/main vs origin/upstream"
	@echo "  make upstream-dev-status      Upstream dev patches requiring review"
	@echo "  make verify-dev               Refresh origin and inspect dev"
	@echo "  make verify-main              Refresh origin/upstream and inspect main"
	@echo "  make branch NEW_BRANCH=name   Create feature branch; clean dev may include parked commits"
	@echo "  make push                     Push clean feature branch; refuses dev/main"
	@echo
	@echo "PULL REQUESTS:"
	@echo "  make pr-create TITLE=... BODY_FILE=file"
	@echo "                                Create feature -> dev PR"
	@echo "  make promote-create TITLE=... BODY_FILE=file"
	@echo "                                Create dev -> main promotion PR"
	@echo "  make sync-dev-create TITLE=... BODY_FILE=file"
	@echo "                                Create main -> dev synchronization PR"
	@echo "  make pr-view PR=55            Show PR identity/direction/state"
	@echo "  make pr-runs [BRANCH=name]    Recent PR validation runs"
	@echo "  make pr-watch [PR=55]         Auto-resolve current PR and watch exact head SHA"
	@echo "  make pr-merge PR=55           Merge; delete feature branches only"
	@echo "  make post-merge PR=55         Update base locally and clean feature branch"
	@echo
	@echo "GITHUB ACTIONS:"
	@echo "  make image-runs               Recent dev image builds"
	@echo "  make image-watch              Watch dev image for exact current dev SHA"
	@echo "  make release-runs             Recent formal release workflows"
	@echo "  make release-watch            Watch release for current tagged main SHA"
	@echo "  make ci-watch RUN=12345       Watch run; nonzero exit on workflow failure"
	@echo "  make ci-view RUN=12345        Show workflow result"
	@echo
	@echo "RELEASES:"
	@echo "  make release-status           Current upstream/fork release relationship"
	@echo "  make release-check            Full guarded formal-release validation"
	@echo "  make release                  Validate, tag, push next formal release"
	@echo "  make release-finish           Release + watch + status; NEVER deploys"
	@echo
	@echo "PRODUCTION -- EXPLICIT BOUNDARY:"
	@echo "  make deploy-status            Source/Compose/images/running production"
	@echo "  make deploy-dev               Explicit validated dev-image deployment"
	@echo "  make deploy                   Explicit formal production deployment"

deps:
	go mod download

goreleaser-check:
	@docker run --rm \
		-v "$$(pwd):/go/src/github.com/samcro1967/glance" \
		-w /go/src/github.com/samcro1967/glance \
		goreleaser/goreleaser:$(GORELEASER_VERSION) check

test:
	go test ./...

test-race:
	go test -race ./...

test-count:
	go test ./... -count=$(COUNT)

test-race-count:
	go test -race ./... -count=$(COUNT)

build:
	go build ./...

fmt-check:
	@files="$$(git diff --name-only --diff-filter=ACMR $(BASE_REF)...HEAD -- '*.go'; \
		git diff --name-only --diff-filter=ACMR -- '*.go'; \
		git ls-files --others --exclude-standard -- '*.go')"; \
	files="$$(printf '%s\n' "$$files" | sort -u | sed '/^$$/d')"; \
	if [ -n "$$files" ]; then \
		unformatted="$$(printf '%s\n' "$$files" | xargs gofmt -l)"; \
		if [ -n "$$unformatted" ]; then \
			echo "The following changed Go files require gofmt:"; \
			printf '%s\n' "$$unformatted"; \
			exit 1; \
		fi; \
	fi

diff-check:
	git diff --check

staged-check:
	git diff --cached --check

docs-check:
	python3 scripts/check_docs.py

check: test test-race build fmt-check diff-check staged-check docs-check frontend-audit

coverage:
	go test ./... -coverprofile=$(COVERAGE_FILE)
	go tool cover -func=$(COVERAGE_FILE)

vuln:
	govulncheck ./...

status:
	@echo "=== BRANCH ==="
	@git branch --show-current
	@echo
	@echo "=== LATEST COMMIT ==="
	@git log -1 --oneline --decorate
	@echo
	@echo "=== STATUS ==="
	@git status --short

staged-diff:
	@echo "=== STAGED STAT ==="
	@git diff --cached --stat
	@echo
	@echo "=== STAGED DIFF ==="
	@git diff --cached

upstream-status:
	@echo "=== REFRESH ORIGIN ==="
	@git fetch origin --prune
	@echo
	@echo "=== REFRESH UPSTREAM ==="
	@git fetch upstream --prune
	@echo
	@echo "=== DEV VS ORIGIN ==="
	@git rev-list --left-right --count origin/$(DEV_BRANCH)...$(DEV_BRANCH)
	@echo
	@echo "=== MAIN VS ORIGIN ==="
	@git rev-list --left-right --count origin/$(STABLE_BRANCH)...$(STABLE_BRANCH)
	@echo
	@echo "=== DEV VS MAIN ==="
	@git rev-list --left-right --count $(STABLE_BRANCH)...$(DEV_BRANCH)
	@echo
	@echo "=== MAIN VS UPSTREAM ==="
	@git rev-list --left-right --count upstream/main...$(STABLE_BRANCH)

upstream-dev-status:
	@echo "=== REFRESH UPSTREAM ==="
	@git fetch upstream --prune
	@echo
	@echo "=== FORK DEV VS UPSTREAM DEV ==="
	@git rev-list --left-right --count upstream/$(DEV_BRANCH)...$(DEV_BRANCH)
	@echo
	@echo "=== UPSTREAM DEV PATCH STATUS ==="
	@set -euo pipefail; \
	total=$$(git rev-list --count $(DEV_BRANCH)..upstream/$(DEV_BRANCH)); \
	incorporated=$$(git cherry $(DEV_BRANCH) upstream/$(DEV_BRANCH) | grep -c "^- " || true); \
	actionable=$$(git cherry $(DEV_BRANCH) upstream/$(DEV_BRANCH) | grep -c "^+ " || true); \
	echo "Upstream-only commits:     $$total"; \
	echo "Already incorporated:     $$incorporated"; \
	echo "Require review:           $$actionable"; \
	echo; \
	if [ "$$actionable" -eq 0 ]; then \
		echo "STATUS: No new upstream dev patches require review."; \
	else \
		echo "=== PATCHES REQUIRING REVIEW ==="; \
		git cherry -v $(DEV_BRANCH) upstream/$(DEV_BRANCH) | grep "^+ " || true; \
	fi
	@echo
	@echo "Inspection only; upstream/$(DEV_BRANCH) is not automatically merged into fork $(DEV_BRANCH)."

branch:
	@set -euo pipefail; \
	if [ -z "$(NEW_BRANCH)" ]; then \
		echo "NEW_BRANCH is required. Example: make branch NEW_BRANCH=feature/example"; \
		exit 2; \
	fi; \
	current="$$(git branch --show-current)"; \
	if [ "$$current" != "$(DEV_BRANCH)" ]; then \
		echo "Branch creation requires $(DEV_BRANCH); current branch is $$current."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Branch creation requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	if git show-ref --verify --quiet "refs/heads/$(NEW_BRANCH)"; then \
		echo "Local branch $(NEW_BRANCH) already exists."; \
		exit 1; \
	fi; \
	echo "Refreshing origin..."; \
	git fetch origin --prune; \
	local_revision="$$(git rev-parse $(DEV_BRANCH))"; \
	origin_revision="$$(git rev-parse origin/$(DEV_BRANCH))"; \
	if [ "$$local_revision" = "$$origin_revision" ]; then \
		echo "Local $(DEV_BRANCH) matches origin/$(DEV_BRANCH)."; \
	elif git merge-base --is-ancestor "$$origin_revision" "$$local_revision"; then \
		parked="$$(git rev-list --count "$$origin_revision..$$local_revision")"; \
		echo "Local $(DEV_BRANCH) contains $$parked parked commit(s) not yet in origin/$(DEV_BRANCH)."; \
		echo "The new feature branch will include those parked commits."; \
	else \
		echo "Refusing branch creation: local $(DEV_BRANCH) is behind or has diverged from origin/$(DEV_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	echo "Creating branch $(NEW_BRANCH) from $$local_revision..."; \
	git switch -c "$(NEW_BRANCH)"

push:
	@set -euo pipefail; \
	branch="$$(git branch --show-current)"; \
	if [ -z "$$branch" ]; then \
		echo "Unable to determine current branch."; \
		exit 1; \
	fi; \
	if [ "$$branch" = "$(DEV_BRANCH)" ] || [ "$$branch" = "$(STABLE_BRANCH)" ]; then \
		echo "Refusing to push protected branch $$branch with this target."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Push requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "Pushing $$branch to origin..."; \
	git push -u origin "$$branch"

pr-create:
	@set -euo pipefail; \
	if [ -z "$(TITLE)" ]; then \
		echo "TITLE is required. Example: make pr-create TITLE='Add feature' BODY_FILE=/tmp/pr-body.md"; \
		exit 2; \
	fi; \
	if [ -z "$(BODY_FILE)" ]; then \
		echo "BODY_FILE is required. Example: make pr-create TITLE='Add feature' BODY_FILE=/tmp/pr-body.md"; \
		exit 2; \
	fi; \
	if [ ! -f "$(BODY_FILE)" ]; then \
		echo "BODY_FILE does not exist: $(BODY_FILE)"; \
		exit 1; \
	fi; \
	branch="$$(git branch --show-current)"; \
	if [ -z "$$branch" ]; then \
		echo "Unable to determine current branch."; \
		exit 1; \
	fi; \
	if [ "$$branch" = "$(DEV_BRANCH)" ]; then \
		echo "Refusing normal PR creation from $(DEV_BRANCH). Use make promote-create for $(DEV_BRANCH) to $(STABLE_BRANCH)."; \
		exit 1; \
	fi; \
	if [ "$$branch" = "$(STABLE_BRANCH)" ]; then \
		echo "Refusing to create a pull request from $(STABLE_BRANCH)."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "PR creation requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	if ! git rev-parse --verify --quiet "@{upstream}" >/dev/null; then \
		echo "Current branch has no upstream. Run make push first."; \
		exit 1; \
	fi; \
	upstream="$$(git rev-parse --abbrev-ref '@{upstream}')"; \
	if [ "$$upstream" != "origin/$$branch" ]; then \
		echo "Current branch does not track origin/$$branch."; \
		echo "Tracking: $$upstream"; \
		exit 1; \
	fi; \
	local_revision="$$(git rev-parse HEAD)"; \
	remote_revision="$$(git rev-parse '@{upstream}')"; \
	if [ "$$local_revision" != "$$remote_revision" ]; then \
		echo "Current branch does not match its remote."; \
		echo "Local:  $$local_revision"; \
		echo "Remote: $$remote_revision"; \
		echo "Run make push first."; \
		exit 1; \
	fi; \
	echo "Creating PR from $$branch to $(PR_BASE)..."; \
	gh pr create \
		--repo "$(REPO)" \
		--base "$(PR_BASE)" \
		--head "$$branch" \
		--title "$(TITLE)" \
		--body-file "$(BODY_FILE)"

promote-create:
	@set -euo pipefail; \
	if [ -z "$(TITLE)" ]; then \
		echo "TITLE is required. Example: make promote-create TITLE='Promote dev to main' BODY_FILE=/tmp/pr-body.md"; \
		exit 2; \
	fi; \
	if [ -z "$(BODY_FILE)" ]; then \
		echo "BODY_FILE is required. Example: make promote-create TITLE='Promote dev to main' BODY_FILE=/tmp/pr-body.md"; \
		exit 2; \
	fi; \
	if [ ! -f "$(BODY_FILE)" ]; then \
		echo "BODY_FILE does not exist: $(BODY_FILE)"; \
		exit 1; \
	fi; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(DEV_BRANCH)" ]; then \
		echo "Promotion requires branch $(DEV_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Promotion requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "Refreshing origin..."; \
	git fetch origin --prune; \
	local_revision="$$(git rev-parse $(DEV_BRANCH))"; \
	remote_revision="$$(git rev-parse origin/$(DEV_BRANCH))"; \
	if [ "$$local_revision" != "$$remote_revision" ]; then \
		echo "Local $(DEV_BRANCH) does not match origin/$(DEV_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$remote_revision"; \
		exit 1; \
	fi; \
	if [ "$$(git rev-list --count origin/$(STABLE_BRANCH)..origin/$(DEV_BRANCH))" -eq 0 ]; then \
		echo "$(DEV_BRANCH) contains no commits to promote to $(STABLE_BRANCH)."; \
		exit 1; \
	fi; \
	echo "Creating promotion PR from $(DEV_BRANCH) to $(STABLE_BRANCH)..."; \
	gh pr create \
		--repo "$(REPO)" \
		--base "$(STABLE_BRANCH)" \
		--head "$(DEV_BRANCH)" \
		--title "$(TITLE)" \
		--body-file "$(BODY_FILE)"

sync-dev-create:
	@set -euo pipefail; \
	if [ -z "$(TITLE)" ]; then \
		echo "TITLE is required. Example: make sync-dev-create TITLE='Synchronize dev with released main' BODY_FILE=/tmp/pr-body.md"; \
		exit 2; \
	fi; \
	if [ -z "$(BODY_FILE)" ]; then \
		echo "BODY_FILE is required. Example: make sync-dev-create TITLE='Synchronize dev with released main' BODY_FILE=/tmp/pr-body.md"; \
		exit 2; \
	fi; \
	if [ ! -f "$(BODY_FILE)" ]; then \
		echo "BODY_FILE does not exist: $(BODY_FILE)"; \
		exit 1; \
	fi; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(DEV_BRANCH)" ]; then \
		echo "Dev synchronization requires branch $(DEV_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Dev synchronization requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "Refreshing origin..."; \
	git fetch origin --prune; \
	local_dev="$$(git rev-parse $(DEV_BRANCH))"; \
	origin_dev="$$(git rev-parse origin/$(DEV_BRANCH))"; \
	local_main="$$(git rev-parse $(STABLE_BRANCH))"; \
	origin_main="$$(git rev-parse origin/$(STABLE_BRANCH))"; \
	if [ "$$local_dev" != "$$origin_dev" ]; then \
		echo "Local $(DEV_BRANCH) does not match origin/$(DEV_BRANCH)."; \
		echo "Local:  $$local_dev"; \
		echo "Origin: $$origin_dev"; \
		exit 1; \
	fi; \
	if [ "$$local_main" != "$$origin_main" ]; then \
		echo "Local $(STABLE_BRANCH) does not match origin/$(STABLE_BRANCH)."; \
		echo "Local:  $$local_main"; \
		echo "Origin: $$origin_main"; \
		exit 1; \
	fi; \
	dev_ahead="$$(git rev-list --count origin/$(STABLE_BRANCH)..origin/$(DEV_BRANCH))"; \
	main_ahead="$$(git rev-list --count origin/$(DEV_BRANCH)..origin/$(STABLE_BRANCH))"; \
	if [ "$$dev_ahead" -ne 0 ]; then \
		echo "Refusing dev synchronization: $(DEV_BRANCH) contains commits not present in $(STABLE_BRANCH)."; \
		echo "Dev-only commits: $$dev_ahead"; \
		echo "Promote or otherwise reconcile $(DEV_BRANCH) before synchronizing from $(STABLE_BRANCH)."; \
		exit 1; \
	fi; \
	if [ "$$main_ahead" -eq 0 ]; then \
		echo "$(STABLE_BRANCH) contains no commits to synchronize to $(DEV_BRANCH)."; \
		exit 1; \
	fi; \
	echo "Dev revision:  $$origin_dev"; \
	echo "Main revision: $$origin_main"; \
	echo "Main ahead:    $$main_ahead"; \
	echo "Creating synchronization PR from $(STABLE_BRANCH) to $(DEV_BRANCH)..."; \
	gh pr create \
		--repo "$(REPO)" \
		--base "$(DEV_BRANCH)" \
		--head "$(STABLE_BRANCH)" \
		--title "$(TITLE)" \
		--body-file "$(BODY_FILE)"

pr-view:
	@if [ -z "$(PR)" ]; then \
		echo "PR is required. Example: make pr-view PR=55"; \
		exit 2; \
	fi
	@gh pr view "$(PR)" \
		--repo "$(REPO)" \
		--json number,title,url,state,headRefName,baseRefName,headRefOid,mergeCommit,mergedAt

pr-runs:
	@if [ -z "$(BRANCH)" ]; then \
		echo "BRANCH could not be determined. Example: make pr-runs BRANCH=dev"; \
		exit 2; \
	fi
	@gh run list \
		--repo "$(REPO)" \
		--workflow "$(PR_WORKFLOW)" \
		--branch "$(BRANCH)" \
		--limit 5 \
		--json databaseId,headSha,status,conclusion,createdAt,displayTitle

pr-watch:
	@set -euo pipefail; \
	pr="$(PR)"; \
	if [ -z "$$pr" ]; then \
		head="$$(git branch --show-current)"; \
		if [ "$$head" = "$(DEV_BRANCH)" ]; then base="$(STABLE_BRANCH)"; \
		elif [ "$$head" = "$(STABLE_BRANCH)" ]; then base="$(DEV_BRANCH)"; \
		else base="$(DEV_BRANCH)"; fi; \
		pr="$$(python3 scripts/resolve_pr.py --repo "$(REPO)" --head "$$head" --base "$$base")"; \
		echo "Resolved PR #$$pr for $$head -> $$base."; \
	fi; \
	pr_state="$$(gh pr view "$$pr" --repo "$(REPO)" --json state --jq '.state')"; \
	head_branch="$$(gh pr view "$$pr" --repo "$(REPO)" --json headRefName --jq '.headRefName')"; \
	revision="$$(gh pr view "$$pr" --repo "$(REPO)" --json headRefOid --jq '.headRefOid')"; \
	if [ "$$pr_state" != "OPEN" ]; then \
		echo "PR #$$pr is not open; current state is $$pr_state."; \
		exit 1; \
	fi; \
	echo "=== FIND PR VALIDATION RUN ==="; \
	echo "PR=$$pr"; \
	echo "Branch=$$head_branch"; \
	echo "Revision=$$revision"; \
	run_id=""; \
	for i in $$(seq 1 "$(CI_RUN_RETRIES)"); do \
		current_revision="$$(gh pr view "$$pr" --repo "$(REPO)" --json headRefOid --jq '.headRefOid')"; \
		if [ "$$current_revision" != "$$revision" ]; then \
			echo "PR head changed: $$revision -> $$current_revision"; \
			revision="$$current_revision"; \
		fi; \
		run_id="$$(gh run list \
			--repo "$(REPO)" \
			--workflow "$(PR_WORKFLOW)" \
			--branch "$$head_branch" \
			--event pull_request \
			--limit 20 \
			--json databaseId,headSha \
			--jq '.[] | select(.headSha == "'"$$revision"'") | .databaseId' \
			| head -1)"; \
		if [ -n "$$run_id" ]; then \
			break; \
		fi; \
		echo "Matching validation run not available yet; retrying ($$i/$(CI_RUN_RETRIES))..."; \
		sleep "$(CI_RUN_RETRY_DELAY)"; \
	done; \
	if [ -z "$$run_id" ]; then \
		echo "No pull-request validation run found for PR #$$pr at $$revision."; \
		exit 1; \
	fi; \
	echo "Run=$$run_id"; \
	echo; \
	echo "=== WATCH PR VALIDATION ==="; \
	gh run watch "$$run_id" \
		--repo "$(REPO)" \
		--exit-status; \
	echo; \
	echo "=== PR VALIDATION RESULT ==="; \
	gh run view "$$run_id" \
		--repo "$(REPO)" \
		--json status,conclusion,headSha,url


pr-merge:
	@set -euo pipefail; \
	if [ -z "$(PR)" ]; then \
		echo "PR is required. Example: make pr-merge PR=55"; \
		exit 2; \
	fi; \
	head_branch="$$(gh pr view "$(PR)" --repo "$(REPO)" --json headRefName --jq '.headRefName')"; \
	if [ "$$head_branch" = "$(DEV_BRANCH)" ] || [ "$$head_branch" = "$(STABLE_BRANCH)" ]; then \
		echo "Preserving long-lived branch $$head_branch."; \
		gh pr merge "$(PR)" \
			--repo "$(REPO)" \
			--merge; \
	else \
		gh pr merge "$(PR)" \
			--repo "$(REPO)" \
			--merge \
			--delete-branch; \
	fi

post-merge:
	@set -euo pipefail; \
	if [ -z "$(PR)" ]; then \
		echo "PR is required. Example: make post-merge PR=55"; \
		exit 2; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Post-merge cleanup requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "=== PR #$(PR) ==="; \
	pr_state="$$(gh pr view "$(PR)" --repo "$(REPO)" --json state --jq '.state')"; \
	head_branch="$$(gh pr view "$(PR)" --repo "$(REPO)" --json headRefName --jq '.headRefName')"; \
	base_branch="$$(gh pr view "$(PR)" --repo "$(REPO)" --json baseRefName --jq '.baseRefName')"; \
	if [ "$$pr_state" != "MERGED" ]; then \
		echo "PR #$(PR) is not merged; current state is $$pr_state."; \
		exit 1; \
	fi; \
	if [ "$$base_branch" != "$(DEV_BRANCH)" ] && [ "$$base_branch" != "$(STABLE_BRANCH)" ]; then \
		echo "Unexpected PR base branch: $$base_branch."; \
		exit 1; \
	fi; \
	echo "State=$$pr_state"; \
	echo "Head=$$head_branch"; \
	echo "Base=$$base_branch"; \
	echo; \
	echo "=== UPDATE LOCAL $$base_branch ==="; \
	git fetch origin --prune; \
	if git show-ref --verify --quiet "refs/heads/$$base_branch"; then \
		git switch "$$base_branch"; \
	else \
		git switch -c "$$base_branch" --track "origin/$$base_branch"; \
	fi; \
	local_revision="$$(git rev-parse "$$base_branch")"; \
	origin_revision="$$(git rev-parse "origin/$$base_branch")"; \
	if [ "$$local_revision" = "$$origin_revision" ]; then \
		echo "Local $$base_branch already matches origin/$$base_branch."; \
	elif git merge-base --is-ancestor "$$local_revision" "$$origin_revision"; then \
		echo "Fast-forwarding local $$base_branch to origin/$$base_branch..."; \
		git merge --ff-only "origin/$$base_branch"; \
	elif [ "$$base_branch" = "$(DEV_BRANCH)" ] && [ "$$head_branch" != "$(DEV_BRANCH)" ] && [ "$$head_branch" != "$(STABLE_BRANCH)" ]; then \
		if ! git show-ref --verify --quiet "refs/heads/$$head_branch"; then \
			echo "Refusing dev reconciliation: local feature branch $$head_branch is unavailable for ancestry verification."; \
			exit 1; \
		fi; \
		feature_revision="$$(git rev-parse "$$head_branch")"; \
		if ! git merge-base --is-ancestor "$$feature_revision" "$$origin_revision"; then \
			echo "Refusing dev reconciliation: merged feature revision is not contained in origin/$(DEV_BRANCH)."; \
			echo "Feature: $$feature_revision"; \
			echo "Origin:  $$origin_revision"; \
			exit 1; \
		fi; \
		if ! git merge-base --is-ancestor "$$local_revision" "$$feature_revision"; then \
			echo "Refusing dev reconciliation: local $(DEV_BRANCH) contains history not carried by the merged feature branch."; \
			echo "Local:   $$local_revision"; \
			echo "Feature: $$feature_revision"; \
			exit 1; \
		fi; \
		echo "Merged feature contains the parked local $(DEV_BRANCH) history."; \
		echo "Reconciling local $(DEV_BRANCH) to the verified merged origin/$(DEV_BRANCH)..."; \
		git reset --hard "$$origin_revision"; \
	else \
		echo "Refusing post-merge update: local $$base_branch cannot be safely fast-forwarded to origin/$$base_branch."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	echo; \
	echo "=== LOCAL BRANCH CLEANUP ==="; \
	if [ "$$head_branch" = "$(DEV_BRANCH)" ] || [ "$$head_branch" = "$(STABLE_BRANCH)" ]; then \
		echo "Preserving long-lived branch $$head_branch."; \
	elif git show-ref --verify --quiet "refs/heads/$$head_branch"; then \
		git branch -d "$$head_branch"; \
	else \
		echo "Local branch $$head_branch does not exist; nothing to delete."; \
	fi; \
	echo; \
	if [ "$$base_branch" = "$(DEV_BRANCH)" ]; then \
		$(MAKE) verify-dev; \
	else \
		$(MAKE) verify-main; \
	fi


pr-finish:
	@set -euo pipefail; \
	head="$$(git branch --show-current)"; \
	if [ "$$head" = "$(DEV_BRANCH)" ] || [ "$$head" = "$(STABLE_BRANCH)" ]; then \
		echo "Refusing pr-finish: current branch must be a feature branch; found $$head."; \
		exit 1; \
	fi; \
	pr="$(PR)"; \
	if [ -z "$$pr" ]; then \
		pr="$$(python3 scripts/resolve_pr.py --repo "$(REPO)" --head "$$head" --base "$(DEV_BRANCH)")"; \
		echo "Resolved feature PR #$$pr."; \
	fi; \
	base="$$(gh pr view "$$pr" --repo "$(REPO)" --json baseRefName --jq '.baseRefName')"; \
	pr_head="$$(gh pr view "$$pr" --repo "$(REPO)" --json headRefName --jq '.headRefName')"; \
	if [ "$$base" != "$(DEV_BRANCH)" ] || [ "$$pr_head" = "$(DEV_BRANCH)" ] || [ "$$pr_head" = "$(STABLE_BRANCH)" ]; then \
		echo "Refusing pr-finish: expected feature -> $(DEV_BRANCH); found $$pr_head -> $$base."; \
		exit 1; \
	fi; \
	if [ "$$pr_head" != "$$head" ]; then \
		echo "Refusing pr-finish: current branch $$head does not match PR head $$pr_head."; \
		exit 1; \
	fi; \
	echo "=== FEATURE PR #$$pr ==="; \
	$(MAKE) pr-watch PR="$$pr"; \
	$(MAKE) pr-merge PR="$$pr"; \
	$(MAKE) post-merge PR="$$pr"; \
	$(MAKE) image-watch; \
	$(MAKE) status

promote-finish:
	@set -euo pipefail; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(DEV_BRANCH)" ]; then \
		echo "Promotion finish requires branch $(DEV_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	git fetch origin --prune; \
	local_revision="$$(git rev-parse $(DEV_BRANCH))"; \
	origin_revision="$$(git rev-parse origin/$(DEV_BRANCH))"; \
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Refusing promote-finish: local $(DEV_BRANCH) does not match origin/$(DEV_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	pr="$(PR)"; \
	if [ -z "$$pr" ]; then \
		pr="$$(python3 scripts/resolve_pr.py --repo "$(REPO)" --head "$(DEV_BRANCH)" --base "$(STABLE_BRANCH)")"; \
		echo "Resolved promotion PR #$$pr."; \
	fi; \
	head="$$(gh pr view "$$pr" --repo "$(REPO)" --json headRefName --jq '.headRefName')"; \
	base="$$(gh pr view "$$pr" --repo "$(REPO)" --json baseRefName --jq '.baseRefName')"; \
	if [ "$$head" != "$(DEV_BRANCH)" ] || [ "$$base" != "$(STABLE_BRANCH)" ]; then \
		echo "Refusing promote-finish: expected $(DEV_BRANCH) -> $(STABLE_BRANCH); found $$head -> $$base."; \
		exit 1; \
	fi; \
	echo "=== PROMOTION PR #$$pr ==="; \
	$(MAKE) pr-watch PR="$$pr"; \
	$(MAKE) pr-merge PR="$$pr"; \
	$(MAKE) post-merge PR="$$pr"; \
	$(MAKE) verify-main

sync-finish:
	@set -euo pipefail; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(DEV_BRANCH)" ]; then \
		echo "Synchronization finish requires branch $(DEV_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	git fetch origin --prune; \
	local_revision="$$(git rev-parse $(DEV_BRANCH))"; \
	origin_revision="$$(git rev-parse origin/$(DEV_BRANCH))"; \
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Refusing sync-finish: local $(DEV_BRANCH) does not match origin/$(DEV_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	pr="$(PR)"; \
	if [ -z "$$pr" ]; then \
		pr="$$(python3 scripts/resolve_pr.py --repo "$(REPO)" --head "$(STABLE_BRANCH)" --base "$(DEV_BRANCH)")"; \
		echo "Resolved synchronization PR #$$pr."; \
	fi; \
	head="$$(gh pr view "$$pr" --repo "$(REPO)" --json headRefName --jq '.headRefName')"; \
	base="$$(gh pr view "$$pr" --repo "$(REPO)" --json baseRefName --jq '.baseRefName')"; \
	if [ "$$head" != "$(STABLE_BRANCH)" ] || [ "$$base" != "$(DEV_BRANCH)" ]; then \
		echo "Refusing sync-finish: expected $(STABLE_BRANCH) -> $(DEV_BRANCH); found $$head -> $$base."; \
		exit 1; \
	fi; \
	echo "=== SYNC PR #$$pr ==="; \
	$(MAKE) pr-watch PR="$$pr"; \
	$(MAKE) pr-merge PR="$$pr"; \
	$(MAKE) post-merge PR="$$pr"; \
	$(MAKE) image-watch; \
	$(MAKE) status


image-runs:
	@gh run list \
		--repo "$(REPO)" \
		--workflow "$(IMAGE_WORKFLOW)" \
		--branch "$(DEV_BRANCH)" \
		--limit 5 \
		--json databaseId,headSha,status,conclusion,createdAt,displayTitle

image-watch:
	@set -euo pipefail; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(DEV_BRANCH)" ]; then \
		echo "Image watch requires branch $(DEV_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	revision="$$(git rev-parse HEAD)"; \
	echo "=== FIND DEVELOPMENT IMAGE RUN ==="; \
	echo "Branch=$$branch"; \
	echo "Revision=$$revision"; \
	run_id=""; \
	for i in $$(seq 1 "$(CI_RUN_RETRIES)"); do \
		run_id="$$(gh run list \
			--repo "$(REPO)" \
			--workflow "$(IMAGE_WORKFLOW)" \
			--branch "$(DEV_BRANCH)" \
			--limit 20 \
			--json databaseId,headSha \
			--jq '.[] | select(.headSha == "'"$$revision"'") | .databaseId' \
			| head -1)"; \
		if [ -n "$$run_id" ]; then \
			break; \
		fi; \
		echo "Matching image run not available yet; retrying ($$i/$(CI_RUN_RETRIES))..."; \
		sleep "$(CI_RUN_RETRY_DELAY)"; \
	done; \
	if [ -z "$$run_id" ]; then \
		echo "No development image run found for $$revision."; \
		exit 1; \
	fi; \
	echo "Run=$$run_id"; \
	echo; \
	echo "=== WATCH DEVELOPMENT IMAGE ==="; \
	gh run watch "$$run_id" \
		--repo "$(REPO)" \
		--exit-status; \
	echo; \
	echo "=== DEVELOPMENT IMAGE RESULT ==="; \
	gh run view "$$run_id" \
		--repo "$(REPO)" \
		--json status,conclusion,headSha,url

release-runs:
	@gh run list \
		--repo "$(REPO)" \
		--workflow "$(RELEASE_WORKFLOW)" \
		--limit 5 \
		--json databaseId,headSha,headBranch,status,conclusion,createdAt,displayTitle

release-watch:
	@set -euo pipefail; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(STABLE_BRANCH)" ]; then \
		echo "Release watch requires branch $(STABLE_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	revision="$$(git rev-parse HEAD)"; \
	release_tag="$$(git tag --points-at HEAD --list 'v*-$(FORK_RELEASE_ID).r*' --sort=-version:refname | head -1)"; \
	if [ -z "$$release_tag" ]; then \
		echo "No formal fork release tag points at current $(STABLE_BRANCH) revision $$revision."; \
		exit 1; \
	fi; \
	echo "=== FIND FORMAL RELEASE RUN ==="; \
	echo "Release=$$release_tag"; \
	echo "Revision=$$revision"; \
	run_id=""; \
	for i in $$(seq 1 "$(CI_RUN_RETRIES)"); do \
		run_id="$$(gh run list \
			--repo "$(REPO)" \
			--workflow "$(RELEASE_WORKFLOW)" \
			--limit 20 \
			--json databaseId,headSha,headBranch \
			--jq '.[] | select(.headSha == "'"$$revision"'" and .headBranch == "'"$$release_tag"'") | .databaseId' \
			| head -1)"; \
		if [ -n "$$run_id" ]; then \
			break; \
		fi; \
		echo "Matching release run not available yet; retrying ($$i/$(CI_RUN_RETRIES))..."; \
		sleep "$(CI_RUN_RETRY_DELAY)"; \
	done; \
	if [ -z "$$run_id" ]; then \
		echo "No release run found for $$release_tag at $$revision."; \
		exit 1; \
	fi; \
	echo "Run=$$run_id"; \
	echo; \
	echo "=== WATCH FORMAL RELEASE ==="; \
	gh run watch "$$run_id" \
		--repo "$(REPO)" \
		--exit-status; \
	echo; \
	echo "=== FORMAL RELEASE RESULT ==="; \
	gh run view "$$run_id" \
		--repo "$(REPO)" \
		--json status,conclusion,headSha,headBranch,url

ci-watch:
	@if [ -z "$(RUN)" ]; then \
		echo "RUN is required. Example: make ci-watch RUN=12345"; \
		exit 2; \
	fi
	@gh run watch "$(RUN)" \
		--repo "$(REPO)" \
		--exit-status

ci-view:
	@if [ -z "$(RUN)" ]; then \
		echo "RUN is required. Example: make ci-view RUN=12345"; \
		exit 2; \
	fi
	@gh run view "$(RUN)" \
		--repo "$(REPO)" \
		--json status,conclusion,headSha,url

verify-dev:
	@echo "=== REFRESH ORIGIN ==="
	@git fetch origin --prune
	@echo
	@echo "=== CURRENT BRANCH ==="
	@git branch --show-current
	@echo
	@echo "=== DEV ==="
	@git log -1 --oneline --decorate $(DEV_BRANCH)
	@echo
	@echo "=== STATUS ==="
	@git status --short
	@echo
	@echo "=== DEV VS ORIGIN ==="
	@git rev-list --left-right --count origin/$(DEV_BRANCH)...$(DEV_BRANCH)
	@echo
	@echo "=== DEV VS MAIN ==="
	@git rev-list --left-right --count $(STABLE_BRANCH)...$(DEV_BRANCH)

verify-main:
	@echo "=== REFRESH ORIGIN ==="
	@git fetch origin --prune
	@echo
	@echo "=== REFRESH UPSTREAM ==="
	@git fetch upstream --prune
	@echo
	@echo "=== CURRENT BRANCH ==="
	@git branch --show-current
	@echo
	@echo "=== MAIN ==="
	@git log -1 --oneline --decorate $(STABLE_BRANCH)
	@echo
	@echo "=== STATUS ==="
	@git status --short
	@echo
	@echo "=== MAIN VS ORIGIN ==="
	@git rev-list --left-right --count origin/$(STABLE_BRANCH)...$(STABLE_BRANCH)
	@echo
	@echo "=== DEV VS MAIN ==="
	@git rev-list --left-right --count $(STABLE_BRANCH)...$(DEV_BRANCH)
	@echo
	@echo "=== MAIN VS UPSTREAM ==="
	@git rev-list --left-right --count upstream/main...$(STABLE_BRANCH)

release-status:
	@set -euo pipefail; \
	echo "=== REFRESH RELEASE REFERENCES ==="; \
	git fetch origin --prune --tags; \
	git fetch upstream --prune --tags; \
	echo; \
	upstream_release="$$(git tag --merged upstream/main --list 'v*' --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1)"; \
	if [ -z "$$upstream_release" ]; then \
		echo "Unable to determine latest formal upstream release."; \
		exit 1; \
	fi; \
	upstream_baseline="$$(git rev-parse upstream/main)"; \
	fork_revision="$$(git rev-parse HEAD)"; \
	latest_fork="$$(git tag --list "$${upstream_release}-$(FORK_RELEASE_ID).r*" --sort=-version:refname | head -1)"; \
	if [ -n "$$latest_fork" ]; then \
		latest_number="$${latest_fork##*.r}"; \
		if ! [[ "$$latest_number" =~ ^[0-9]+$$ ]]; then \
			echo "Unable to parse fork revision from $$latest_fork."; \
			exit 1; \
		fi; \
		next_number="$$((10#$$latest_number + 1))"; \
	else \
		next_number=1; \
	fi; \
	printf -v padded "%0*d" "$(FORK_RELEASE_WIDTH)" "$$next_number"; \
	next_release="$${upstream_release}-$(FORK_RELEASE_ID).r$${padded}"; \
	echo "Upstream release:  $$upstream_release"; \
	echo "Upstream baseline: $$upstream_baseline"; \
	echo "Fork revision:     $$fork_revision"; \
	echo "Latest fork tag:   $${latest_fork:-none}"; \
	echo "Next fork release: $$next_release"

release-check:
	@set -euo pipefail; \
	echo "=== RELEASE VALIDATION ==="; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(STABLE_BRANCH)" ]; then \
		echo "Release requires branch $(STABLE_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Release requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "Refreshing release references..."; \
	git fetch origin --prune --tags; \
	git fetch upstream --prune --tags; \
	local_revision="$$(git rev-parse HEAD)"; \
	existing_release="$$(git tag --points-at HEAD --list 'v*-$(FORK_RELEASE_ID).r*' --sort=-version:refname | head -1)"; \
	if [ -n "$$existing_release" ]; then \
		echo "Current $(STABLE_BRANCH) revision $$local_revision is already formally released as $$existing_release."; \
		echo "Refusing to create another formal release tag for the same revision."; \
		exit 1; \
	fi; \
	origin_revision="$$(git rev-parse origin/$(STABLE_BRANCH))"; \
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Local $(STABLE_BRANCH) does not match origin/$(STABLE_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	if ! git merge-base --is-ancestor origin/$(DEV_BRANCH) $(STABLE_BRANCH); then \
		echo "$(STABLE_BRANCH) does not contain the current origin/$(DEV_BRANCH)."; \
		echo "Dev:  $$(git rev-parse origin/$(DEV_BRANCH))"; \
		echo "Main: $$local_revision"; \
		echo "Promote $(DEV_BRANCH) to $(STABLE_BRANCH) before releasing."; \
		exit 1; \
	fi; \
	if ! git merge-base --is-ancestor upstream/main $(STABLE_BRANCH); then \
		echo "Current upstream/main is not fully incorporated into $(STABLE_BRANCH)."; \
		echo "Upstream: $$(git rev-parse upstream/main)"; \
		echo "Main:     $$local_revision"; \
		exit 1; \
	fi; \
	upstream_release="$$(git tag --merged upstream/main --list 'v*' --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1)"; \
	if [ -z "$$upstream_release" ]; then \
		echo "Unable to determine latest formal upstream release."; \
		exit 1; \
	fi; \
	latest_fork="$$(git tag --list "$${upstream_release}-$(FORK_RELEASE_ID).r*" --sort=-version:refname | head -1)"; \
	if [ -n "$$latest_fork" ]; then \
		latest_number="$${latest_fork##*.r}"; \
		if ! [[ "$$latest_number" =~ ^[0-9]+$$ ]]; then \
			echo "Unable to parse fork revision from $$latest_fork."; \
			exit 1; \
		fi; \
		next_number="$$((10#$$latest_number + 1))"; \
	else \
		next_number=1; \
	fi; \
	printf -v padded "%0*d" "$(FORK_RELEASE_WIDTH)" "$$next_number"; \
	next_release="$${upstream_release}-$(FORK_RELEASE_ID).r$${padded}"; \
	if git show-ref --verify --quiet "refs/tags/$$next_release"; then \
		echo "Release tag already exists locally: $$next_release"; \
		exit 1; \
	fi; \
	if git ls-remote --exit-code --tags origin "refs/tags/$$next_release" >/dev/null 2>&1; then \
		echo "Release tag already exists on origin: $$next_release"; \
		exit 1; \
	fi; \
	echo "Upstream release:  $$upstream_release"; \
	echo "Upstream baseline: $$(git rev-parse upstream/main)"; \
	echo "Fork revision:     $$local_revision"; \
	echo "Next fork release: $$next_release"; \
	echo; \
	echo "Running standard validation..."; \
	$(MAKE) check BASE_REF=origin/$(STABLE_BRANCH); \
	echo; \
	echo "Validating GoReleaser configuration..."; \
	$(MAKE) goreleaser-check; \
	echo; \
	echo "Release validation passed."

release: release-check
	@set -euo pipefail; \
	upstream_release="$$(git tag --merged upstream/main --list 'v*' --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1)"; \
	latest_fork="$$(git tag --list "$${upstream_release}-$(FORK_RELEASE_ID).r*" --sort=-version:refname | head -1)"; \
	if [ -n "$$latest_fork" ]; then \
		latest_number="$${latest_fork##*.r}"; \
		next_number="$$((10#$$latest_number + 1))"; \
	else \
		next_number=1; \
	fi; \
	printf -v padded "%0*d" "$(FORK_RELEASE_WIDTH)" "$$next_number"; \
	release_tag="$${upstream_release}-$(FORK_RELEASE_ID).r$${padded}"; \
	upstream_baseline="$$(git rev-parse upstream/main)"; \
	fork_revision="$$(git rev-parse HEAD)"; \
	echo "=== CREATE RELEASE TAG ==="; \
	echo "Release:           $$release_tag"; \
	echo "Upstream release:  $$upstream_release"; \
	echo "Upstream baseline: $$upstream_baseline"; \
	echo "Fork revision:     $$fork_revision"; \
	git tag -a "$$release_tag" \
		-m "Glance fork release $$release_tag" \
		-m "Upstream release: $$upstream_release" \
		-m "Upstream baseline: $$upstream_baseline" \
		-m "Fork revision: $$fork_revision"; \
	echo; \
	echo "=== PUSH RELEASE TAG ==="; \
	if ! git push origin "$$release_tag"; then \
		echo "Release tag push failed."; \
		echo "Local tag $$release_tag has been retained."; \
		echo "Inspect local and remote tag state before retrying."; \
		exit 1; \
	fi; \
	echo; \
	echo "=== RELEASE STARTED ==="; \
	echo "Tag=$$release_tag"; \
	echo "The tag push will invoke the GitHub Actions GoReleaser workflow."


ship:
	@set -euo pipefail; \
	if [ -z "$(TITLE)" ]; then \
		echo "TITLE is required. Example: make ship TITLE='Add feature'"; \
		exit 2; \
	fi; \
	if [ -n "$(BODY_FILE)" ] && [ ! -f "$(BODY_FILE)" ]; then \
		echo "BODY_FILE does not exist: $(BODY_FILE)"; \
		exit 1; \
	fi; \
	feature="$$(git branch --show-current)"; \
	if [ -z "$$feature" ] || [ "$$feature" = "$(DEV_BRANCH)" ] || [ "$$feature" = "$(STABLE_BRANCH)" ]; then \
		echo "ship must start on a feature branch; current branch is $${feature:-unknown}."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "ship requires a clean working tree with the feature already committed."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "=== END-TO-END RELEASE PIPELINE ==="; \
	echo "Feature=$$feature"; \
	echo "Title=$(TITLE)"; \
	echo; \
	echo "=== PUSH FEATURE ==="; \
	$(MAKE) push; \
	generated_feature_body="$$(mktemp)"; \
	promotion_body="$$(mktemp)"; \
	trap 'rm -f "$$generated_feature_body" "$$promotion_body"' EXIT; \
	if [ -n "$(BODY_FILE)" ]; then \
		feature_body="$(BODY_FILE)"; \
		echo "Feature PR body: $(BODY_FILE)"; \
	else \
		feature_body="$$generated_feature_body"; \
		printf '%s\n\n%s\n' \
			'## Summary' \
			'$(TITLE)' > "$$feature_body"; \
		echo "Feature PR body: generated summary"; \
	fi; \
	echo; \
	echo "=== FEATURE -> $(DEV_BRANCH) ==="; \
	feature_pr="$$(gh pr list --repo "$(REPO)" --head "$$feature" --base "$(DEV_BRANCH)" --state open --json number --jq '.[0].number // empty')"; \
	if [ -z "$$feature_pr" ]; then \
		$(MAKE) pr-create TITLE="$(TITLE)" BODY_FILE="$$feature_body"; \
		feature_pr="$$(python3 scripts/resolve_pr.py --repo "$(REPO)" --head "$$feature" --base "$(DEV_BRANCH)")"; \
	else \
		echo "Reusing existing feature PR #$$feature_pr."; \
	fi; \
	$(MAKE) pr-finish PR="$$feature_pr"; \
	echo; \
	echo "=== $(DEV_BRANCH) -> $(STABLE_BRANCH) ==="; \
	printf '%s\n\n%s\n' \
		'## Summary' \
		'Promote validated development changes to the stable branch for formal release.' > "$$promotion_body"; \
	promotion_pr="$$(gh pr list --repo "$(REPO)" --head "$(DEV_BRANCH)" --base "$(STABLE_BRANCH)" --state open --json number --jq '.[0].number // empty')"; \
	if [ -z "$$promotion_pr" ]; then \
		$(MAKE) promote-create TITLE="Promote dev to main" BODY_FILE="$$promotion_body"; \
		promotion_pr="$$(python3 scripts/resolve_pr.py --repo "$(REPO)" --head "$(DEV_BRANCH)" --base "$(STABLE_BRANCH)")"; \
	else \
		echo "Reusing existing promotion PR #$$promotion_pr."; \
	fi; \
	$(MAKE) promote-finish PR="$$promotion_pr"; \
	echo; \
	echo "=== FORMAL RELEASE ==="; \
	$(MAKE) release-finish; \
	echo; \
	echo "=== SHIP COMPLETE ==="; \
	echo "Formal release completed and verified."; \
	echo "Production was NOT deployed."; \
	echo "Run make deploy-finish to cross the explicit production boundary."

release-finish:
	@set -euo pipefail; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(STABLE_BRANCH)" ]; then \
		echo "release-finish requires $(STABLE_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi
	@echo "=== FORMAL RELEASE PIPELINE ==="
	@$(MAKE) release
	@$(MAKE) release-watch
	@$(MAKE) release-status
	@$(MAKE) deploy-status
	@echo
	@echo "Release pipeline complete. Production was NOT deployed."

workflow-status:
	@echo "=== REPOSITORY ==="
	@$(MAKE) status
	@echo
	@echo "=== BRANCH RELATIONSHIPS ==="
	@$(MAKE) upstream-status
	@echo
	@echo "=== RELEASE ==="
	@$(MAKE) release-status
	@echo
	@echo "=== RECENT PR VALIDATION ==="
	@gh run list \
		--repo "$(REPO)" \
		--workflow "$(PR_WORKFLOW)" \
		--limit 3 \
		--json databaseId,headBranch,headSha,status,conclusion,displayTitle
	@echo
	@echo "=== RECENT DEV IMAGES ==="
	@$(MAKE) image-runs
	@echo
	@echo "=== RECENT RELEASES ==="
	@$(MAKE) release-runs
	@echo
	@echo "=== DEPLOYMENT ==="
	@$(MAKE) deploy-status

deploy-status:
	@set -euo pipefail; \
	echo "=== SOURCE CHECKOUT ==="; \
	branch="$$(git branch --show-current)"; \
	local_revision="$$(git rev-parse HEAD)"; \
	release_tag="$$(git tag --points-at HEAD --list 'v*-$(FORK_RELEASE_ID).r*' --sort=-version:refname | head -1)"; \
	echo "Branch=$$branch"; \
	echo "Revision=$$local_revision"; \
	echo "Release=$${release_tag:-none}"; \
	echo; \
	echo "=== COMPOSE CONFIGURATION ==="; \
	compose_images="$$(cd "$(DEPLOY_DIR)" && docker compose config --images "$(DEPLOY_SERVICE)" 2>/dev/null || true)"; \
	if [ -n "$$compose_images" ]; then \
		printf '%s\n' "$$compose_images"; \
		if printf '%s\n' "$$compose_images" | grep -Fxq "$(DEPLOY_IMAGE)"; then \
			echo "Formal image configured: yes"; \
		else \
			echo "Formal image configured: no"; \
		fi; \
	else \
		echo "Unable to read Compose image configuration."; \
	fi; \
	echo; \
	echo "=== FORMAL DEPLOY IMAGE ==="; \
	if docker image inspect "$(DEPLOY_IMAGE)" >/dev/null 2>&1; then \
		image_id="$$(docker image inspect "$(DEPLOY_IMAGE)" --format '{{.Id}}')"; \
		image_created="$$(docker image inspect "$(DEPLOY_IMAGE)" --format '{{.Created}}')"; \
		image_version="$$(docker run --rm --entrypoint /app/glance "$(DEPLOY_IMAGE)" --version 2>/dev/null || true)"; \
		echo "Version=$${image_version:-unknown}"; \
		echo "Image=$$image_id"; \
		echo "Created=$$image_created"; \
	else \
		echo "Image $(DEPLOY_IMAGE) is not available locally."; \
	fi; \
	echo; \
	echo "=== DEVELOPMENT DEPLOY IMAGE ==="; \
	if docker image inspect "$(DEPLOY_DEV_IMAGE)" >/dev/null 2>&1; then \
		dev_image_id="$$(docker image inspect "$(DEPLOY_DEV_IMAGE)" --format '{{.Id}}')"; \
		dev_image_created="$$(docker image inspect "$(DEPLOY_DEV_IMAGE)" --format '{{.Created}}')"; \
		dev_image_version="$$(docker run --rm --entrypoint /app/glance "$(DEPLOY_DEV_IMAGE)" --version 2>/dev/null || true)"; \
		echo "Version=$${dev_image_version:-unknown}"; \
		echo "Image=$$dev_image_id"; \
		echo "Created=$$dev_image_created"; \
	else \
		echo "Image $(DEPLOY_DEV_IMAGE) is not available locally."; \
	fi; \
	echo; \
	echo "=== PRODUCTION ==="; \
	if docker inspect "$(DEPLOY_CONTAINER)" >/dev/null 2>&1; then \
		container_image="$$(docker inspect "$(DEPLOY_CONTAINER)" --format '{{.Image}}')"; \
		container_started="$$(docker inspect "$(DEPLOY_CONTAINER)" --format '{{.State.StartedAt}}')"; \
		container_version="$$(docker exec "$(DEPLOY_CONTAINER)" /app/glance --version 2>/dev/null || true)"; \
		echo "Version=$${container_version:-unknown}"; \
		echo "Image=$$container_image"; \
		echo "Started=$$container_started"; \
	else \
		echo "Container $(DEPLOY_CONTAINER) does not exist."; \
	fi

deploy-dev:
	@set -euo pipefail; \
	echo "=== PRE-DEPLOY DEV VALIDATION ==="; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(DEV_BRANCH)" ]; then \
		echo "Development deployment requires branch $(DEV_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Development deployment requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "Refreshing origin..."; \
	git fetch origin --prune; \
	local_revision="$$(git rev-parse HEAD)"; \
	origin_revision="$$(git rev-parse origin/$(DEV_BRANCH))"; \
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Local $(DEV_BRANCH) does not match origin/$(DEV_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	echo "Verifying successful development image build..."; \
	run_id="$$(gh run list \
		--repo "$(REPO)" \
		--workflow "$(IMAGE_WORKFLOW)" \
		--branch "$(DEV_BRANCH)" \
		--limit 20 \
		--json databaseId,headSha,status,conclusion \
		--jq '.[] | select(.headSha == "'"$$local_revision"'" and .status == "completed" and .conclusion == "success") | .databaseId' \
		| head -1)"; \
	if [ -z "$$run_id" ]; then \
		echo "Refusing development deployment: no successful dev image build found for $$local_revision."; \
		echo "Run make image-watch after the dev image workflow starts."; \
		exit 1; \
	fi; \
	echo "Dev revision: $$local_revision"; \
	echo "Image run:    $$run_id"; \
	echo; \
	echo "=== VERIFY BASE COMPOSE CONFIGURATION ==="; \
	compose_images="$$(cd "$(DEPLOY_DIR)" && docker compose config --images "$(DEPLOY_SERVICE)")"; \
	if ! printf '%s\n' "$$compose_images" | grep -Fxq "$(DEPLOY_IMAGE)"; then \
		echo "Refusing development deployment: base Compose configuration does not reference $(DEPLOY_IMAGE)."; \
		echo "Keep the permanent Compose configuration on the formal latest image."; \
		printf '%s\n' "$$compose_images"; \
		exit 1; \
	fi; \
	echo "Base Compose remains configured for $(DEPLOY_IMAGE)."; \
	echo; \
	echo "=== PULL DEVELOPMENT IMAGE ==="; \
	docker pull "$(DEPLOY_DEV_IMAGE)"; \
	image_id="$$(docker image inspect "$(DEPLOY_DEV_IMAGE)" --format '{{.Id}}')"; \
	image_version="$$(docker run --rm --entrypoint /app/glance "$(DEPLOY_DEV_IMAGE)" --version)"; \
	echo "Image version: $$image_version"; \
	echo "Image ID:      $$image_id"; \
	if [ "$$image_version" != "dev" ]; then \
		echo "Refusing development deployment: $(DEPLOY_DEV_IMAGE) does not identify as dev."; \
		exit 1; \
	fi; \
	echo; \
	echo "=== DEPLOY DEVELOPMENT IMAGE ==="; \
	override_file="$$(mktemp)"; \
	trap 'rm -f "$$override_file"' EXIT; \
	printf 'services:\n  %s:\n    image: %s\n' "$(DEPLOY_SERVICE)" "$(DEPLOY_DEV_IMAGE)" > "$$override_file"; \
	cd "$(DEPLOY_DIR)"; \
	docker compose -f "$(DEPLOY_COMPOSE_FILE)" -f "$$override_file" up -d --force-recreate --no-deps "$(DEPLOY_SERVICE)"; \
	echo; \
	echo "=== VERIFY DEVELOPMENT CONTAINER ==="; \
	container_version=""; \
	for i in $$(seq 1 "$(DEPLOY_RETRIES)"); do \
		container_version="$$(docker exec "$(DEPLOY_CONTAINER)" /app/glance --version 2>/dev/null || true)"; \
		if [ -n "$$container_version" ]; then \
			break; \
		fi; \
		echo "Glance container not exec-ready; retrying ($$i/$(DEPLOY_RETRIES))..."; \
		sleep "$(DEPLOY_RETRY_DELAY)"; \
	done; \
	if [ -z "$$container_version" ]; then \
		echo "Development deployment verification failed: container did not become exec-ready."; \
		docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | tail -50 || true; \
		exit 1; \
	fi; \
	container_image="$$(docker inspect "$(DEPLOY_CONTAINER)" --format '{{.Image}}')"; \
	echo "Container version: $$container_version"; \
	echo "Container image:   $$container_image"; \
	if [ "$$container_version" != "dev" ]; then \
		echo "Development deployment verification failed: container does not identify as dev."; \
		exit 1; \
	fi; \
	if [ "$$container_image" != "$$image_id" ]; then \
		echo "Development deployment verification failed: container image does not match pulled dev image."; \
		echo "Pulled:    $$image_id"; \
		echo "Container: $$container_image"; \
		exit 1; \
	fi; \
	echo; \
	echo "=== HTTP CHECK ==="; \
	http_ok=0; \
	for i in $$(seq 1 "$(DEPLOY_RETRIES)"); do \
		code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(DEPLOY_URL)" 2>/dev/null || true)"; \
		if [ "$$code" = "200" ] || [ "$$code" = "302" ]; then \
			echo "Glance HTTP: $$code"; \
			http_ok=1; \
			break; \
		fi; \
		echo "Glance HTTP: $${code:-not ready}; retrying..."; \
		sleep "$(DEPLOY_RETRY_DELAY)"; \
	done; \
	if [ "$$http_ok" -ne 1 ]; then \
		echo "Development deployment verification failed: Glance did not become HTTP-ready."; \
		docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | tail -50 || true; \
		exit 1; \
	fi; \
	short_revision="$${local_revision:0:7}"; \
	if ! docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | grep -Fq "revision=$$short_revision"; then \
		echo "Development deployment verification failed: startup log does not report revision $$short_revision."; \
		docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | tail -50 || true; \
		exit 1; \
	fi; \
	echo; \
	echo "=== RECENT LOGS ==="; \
	docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | tail -50 || true; \
	echo; \
	echo "=== DEVELOPMENT DEPLOYMENT COMPLETE ==="; \
	echo "Revision=$$local_revision"; \
	echo "Image=$$container_image"; \
	echo "BaseComposeImage=$(DEPLOY_IMAGE)"

deploy:
	@set -euo pipefail; \
	echo "=== PRE-DEPLOY VALIDATION ==="; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(STABLE_BRANCH)" ]; then \
		echo "Deployment requires branch $(STABLE_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Deployment requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	echo "Refreshing origin..."; \
	git fetch origin --prune --tags; \
	local_revision="$$(git rev-parse HEAD)"; \
	origin_revision="$$(git rev-parse origin/$(STABLE_BRANCH))"; \
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Local $(STABLE_BRANCH) does not match origin/$(STABLE_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	release_tag="$$(git tag --points-at HEAD --list 'v*-$(FORK_RELEASE_ID).r*' --sort=-version:refname | head -1)"; \
	if [ -z "$$release_tag" ]; then \
		echo "Refusing deployment: current $(STABLE_BRANCH) revision has no formal fork release tag."; \
		echo "Revision: $$local_revision"; \
		exit 1; \
	fi; \
	release_revision="$$(git rev-list -n 1 "$$release_tag")"; \
	if [ "$$release_revision" != "$$local_revision" ]; then \
		echo "Refusing deployment: release tag does not resolve to local $(STABLE_BRANCH)."; \
		echo "Release: $$release_tag"; \
		echo "Tag:     $$release_revision"; \
		echo "Source:  $$local_revision"; \
		exit 1; \
	fi; \
	echo "Source revision: $$local_revision"; \
	echo "Release:         $$release_tag"; \
	echo; \
	echo "=== VERIFY BASE COMPOSE CONFIGURATION ==="; \
	compose_images="$$(cd "$(DEPLOY_DIR)" && docker compose config --images "$(DEPLOY_SERVICE)")"; \
	if ! printf '%s\n' "$$compose_images" | grep -Fxq "$(DEPLOY_IMAGE)"; then \
		echo "Refusing deployment: base Compose configuration does not reference $(DEPLOY_IMAGE)."; \
		echo "Formal production deployment requires the permanent Compose configuration to use latest."; \
		printf '%s\n' "$$compose_images"; \
		exit 1; \
	fi; \
	echo "Base Compose is configured for $(DEPLOY_IMAGE)."; \
	echo; \
	echo "=== PULL IMAGE ==="; \
	docker pull "$(DEPLOY_IMAGE)"; \
	image_id="$$(docker image inspect "$(DEPLOY_IMAGE)" --format '{{.Id}}')"; \
	image_version="$$(docker run --rm --entrypoint /app/glance "$(DEPLOY_IMAGE)" --version)"; \
	echo "Image version: $$image_version"; \
	echo "Image ID:      $$image_id"; \
	if [ "$$image_version" != "$$release_tag" ]; then \
		echo "Refusing deployment: $(DEPLOY_IMAGE) does not match the formal release at local $(STABLE_BRANCH)."; \
		echo "Release: $$release_tag"; \
		echo "Image:   $$image_version"; \
		exit 1; \
	fi; \
	echo; \
	echo "=== DEPLOY ==="; \
	cd "$(DEPLOY_DIR)"; \
	docker compose up -d --force-recreate --no-deps "$(DEPLOY_SERVICE)"; \
	echo; \
	echo "=== VERIFY CONTAINER ==="; \
	container_version=""; \
	for i in $$(seq 1 "$(DEPLOY_RETRIES)"); do \
		container_version="$$(docker exec "$(DEPLOY_CONTAINER)" /app/glance --version 2>/dev/null || true)"; \
		if [ -n "$$container_version" ]; then \
			break; \
		fi; \
		echo "Glance container not exec-ready; retrying ($$i/$(DEPLOY_RETRIES))..."; \
		sleep "$(DEPLOY_RETRY_DELAY)"; \
	done; \
	if [ -z "$$container_version" ]; then \
		echo "Deployment verification failed: container did not become exec-ready."; \
		docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | tail -50 || true; \
		exit 1; \
	fi; \
	container_image="$$(docker inspect "$(DEPLOY_CONTAINER)" --format '{{.Image}}')"; \
	echo "Container version: $$container_version"; \
	echo "Container image:   $$container_image"; \
	if [ "$$container_version" != "$$release_tag" ]; then \
		echo "Deployment verification failed: container version does not match formal release."; \
		echo "Release:   $$release_tag"; \
		echo "Container: $$container_version"; \
		exit 1; \
	fi; \
	if [ "$$container_image" != "$$image_id" ]; then \
		echo "Deployment verification failed: container image does not match pulled image."; \
		echo "Pulled:    $$image_id"; \
		echo "Container: $$container_image"; \
		exit 1; \
	fi; \
	echo; \
	echo "=== HTTP CHECK ==="; \
	http_ok=0; \
	for i in $$(seq 1 "$(DEPLOY_RETRIES)"); do \
		code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(DEPLOY_URL)" 2>/dev/null || true)"; \
		if [ "$$code" = "200" ] || [ "$$code" = "302" ]; then \
			echo "Glance HTTP: $$code"; \
			http_ok=1; \
			break; \
		fi; \
		echo "Glance HTTP: $${code:-not ready}; retrying..."; \
		sleep "$(DEPLOY_RETRY_DELAY)"; \
	done; \
	if [ "$$http_ok" -ne 1 ]; then \
		echo "Deployment verification failed: Glance did not become HTTP-ready."; \
		docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | tail -50 || true; \
		exit 1; \
	fi; \
	echo; \
	echo "=== RECENT LOGS ==="; \
	docker logs "$(DEPLOY_CONTAINER)" --since 2m 2>&1 | tail -50 || true; \
	echo; \
	echo "=== DEPLOYMENT COMPLETE ==="; \
	echo "Release=$$release_tag"; \
	echo "Revision=$$local_revision"; \
	echo "Image=$$container_image"

deploy-finish:
	@set -euo pipefail; \
	branch="$$(git branch --show-current)"; \
	if [ "$$branch" != "$(STABLE_BRANCH)" ]; then \
		echo "deploy-finish requires $(STABLE_BRANCH); current branch is $$branch."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "deploy-finish requires a clean working tree."; \
		git status --short; \
		exit 1; \
	fi; \
	release_tag="$$(git tag --points-at HEAD --list 'v*-$(FORK_RELEASE_ID).r*' --sort=-version:refname | head -1)"; \
	if [ -z "$$release_tag" ]; then \
		echo "deploy-finish requires the current $(STABLE_BRANCH) revision to have a formal release tag."; \
		exit 1; \
	fi; \
	echo "=== PRODUCTION + SYNCHRONIZATION PIPELINE ==="; \
	echo "Release=$$release_tag"; \
	echo; \
	echo "=== EXPLICIT PRODUCTION DEPLOYMENT ==="; \
	$(MAKE) deploy; \
	echo; \
	echo "=== PREPARE $(DEV_BRANCH) SYNCHRONIZATION ==="; \
	git fetch origin --prune; \
	git switch "$(DEV_BRANCH)"; \
	git pull --ff-only origin "$(DEV_BRANCH)"; \
	sync_body="$$(mktemp)"; \
	trap 'rm -f "$$sync_body"' EXIT; \
	printf '%s\n\n%s\n' \
		'## Summary' \
		'Synchronize the formally released stable branch back into development.' > "$$sync_body"; \
	echo; \
	echo "=== $(STABLE_BRANCH) -> $(DEV_BRANCH) ==="; \
	sync_pr="$$(gh pr list --repo "$(REPO)" --head "$(STABLE_BRANCH)" --base "$(DEV_BRANCH)" --state open --json number --jq '.[0].number // empty')"; \
	if [ -z "$$sync_pr" ]; then \
		$(MAKE) sync-dev-create TITLE="Sync main back to dev after $$release_tag" BODY_FILE="$$sync_body"; \
		sync_pr="$$(python3 scripts/resolve_pr.py --repo "$(REPO)" --head "$(STABLE_BRANCH)" --base "$(DEV_BRANCH)")"; \
	else \
		echo "Reusing existing synchronization PR #$$sync_pr."; \
	fi; \
	$(MAKE) sync-finish PR="$$sync_pr"; \
	echo; \
	echo "=== FINAL WORKFLOW VERIFICATION ==="; \
	git fetch origin --prune; \
	current="$$(git branch --show-current)"; \
	if [ "$$current" != "$(DEV_BRANCH)" ]; then \
		echo "Final verification failed: expected current branch $(DEV_BRANCH), found $$current."; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Final verification failed: working tree is not clean."; \
		git status --short; \
		exit 1; \
	fi; \
	if [ "$$(git rev-parse $(DEV_BRANCH))" != "$$(git rev-parse origin/$(DEV_BRANCH))" ]; then \
		echo "Final verification failed: local $(DEV_BRANCH) does not match origin/$(DEV_BRANCH)."; \
		exit 1; \
	fi; \
	if [ "$$(git rev-parse $(STABLE_BRANCH))" != "$$(git rev-parse origin/$(STABLE_BRANCH))" ]; then \
		echo "Final verification failed: local $(STABLE_BRANCH) does not match origin/$(STABLE_BRANCH)."; \
		exit 1; \
	fi; \
	if ! git merge-base --is-ancestor "$(STABLE_BRANCH)" "$(DEV_BRANCH)"; then \
		echo "Final verification failed: $(STABLE_BRANCH) is not contained in $(DEV_BRANCH)."; \
		exit 1; \
	fi; \
	container_version="$$(docker exec "$(DEPLOY_CONTAINER)" /app/glance --version 2>/dev/null || true)"; \
	if [ "$$container_version" != "$$release_tag" ]; then \
		echo "Final verification failed: production is not running $$release_tag."; \
		echo "Production: $${container_version:-unknown}"; \
		exit 1; \
	fi; \
	echo "Worktree:              clean"; \
	echo "Current branch:        $(DEV_BRANCH)"; \
	echo "Dev matches origin:    yes"; \
	echo "Main matches origin:   yes"; \
	echo "Main contained in dev: yes"; \
	echo "Production release:    $$container_version"; \
	echo "Production verified:   yes"; \
	echo; \
	$(MAKE) workflow-status; \
	echo; \
	echo "=== WORKFLOW COMPLETE ==="

TEST_FIXTURE_SCRIPT ?= testdata/visual/fixture-server.js
TEST_FIXTURE_URL ?= http://127.0.0.1:18089/visual-test-extension
TEST_FIXTURE_PID_FILE ?= /tmp/glance-test-fixture.pid
TEST_FIXTURE_LOG ?= /tmp/glance-test-fixture.log

.PHONY: test-instance-fixture-start test-instance-fixture-stop

test-instance-fixture-start:
	@set -euo pipefail; \
	if [ -f "$(TEST_FIXTURE_PID_FILE)" ]; then \
		pid="$$(cat "$(TEST_FIXTURE_PID_FILE)")"; \
		if kill -0 "$$pid" 2>/dev/null; then \
			code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_FIXTURE_URL)" 2>/dev/null || true)"; \
			if [ "$$code" = "200" ]; then \
				echo "Test fixture server is already running with PID $$pid."; \
				echo "Fixture URL=$(TEST_FIXTURE_URL)"; \
				exit 0; \
			fi; \
			echo "Stopping unhealthy test fixture server PID $$pid..."; \
			kill "$$pid" 2>/dev/null || true; \
		fi; \
		rm -f "$(TEST_FIXTURE_PID_FILE)"; \
	fi; \
	echo "=== START TEST FIXTURE SERVER ==="; \
	rm -f "$(TEST_FIXTURE_LOG)"; \
	node "$(TEST_FIXTURE_SCRIPT)" > "$(TEST_FIXTURE_LOG)" 2>&1 & \
	pid="$$!"; \
	echo "$$pid" > "$(TEST_FIXTURE_PID_FILE)"; \
	ready=0; \
	for i in $$(seq 1 50); do \
		if ! kill -0 "$$pid" 2>/dev/null; then \
			break; \
		fi; \
		code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_FIXTURE_URL)" 2>/dev/null || true)"; \
		if [ "$$code" = "200" ]; then \
			ready=1; \
			break; \
		fi; \
		sleep 0.1; \
	done; \
	if [ "$$ready" -ne 1 ]; then \
		echo "Test fixture server failed to become ready."; \
		cat "$(TEST_FIXTURE_LOG)" || true; \
		kill "$$pid" 2>/dev/null || true; \
		rm -f "$(TEST_FIXTURE_PID_FILE)"; \
		exit 1; \
	fi; \
	echo "Test fixture server started."; \
	echo "PID=$$pid"; \
	echo "Fixture URL=$(TEST_FIXTURE_URL)"

test-instance-fixture-stop:
	@set -euo pipefail; \
	if [ -f "$(TEST_FIXTURE_PID_FILE)" ]; then \
		pid="$$(cat "$(TEST_FIXTURE_PID_FILE)")"; \
		if kill -0 "$$pid" 2>/dev/null; then \
			echo "Stopping test fixture server PID $$pid..."; \
			kill "$$pid" 2>/dev/null || true; \
			for i in $$(seq 1 20); do \
				if ! kill -0 "$$pid" 2>/dev/null; then \
					break; \
				fi; \
				sleep 0.1; \
			done; \
			if kill -0 "$$pid" 2>/dev/null; then \
				kill -9 "$$pid" 2>/dev/null || true; \
			fi; \
		fi; \
	fi; \
	rm -f "$(TEST_FIXTURE_PID_FILE)" "$(TEST_FIXTURE_LOG)"

test-instance-start: test-instance-fixture-start
	@set -euo pipefail; \
	if ss -ltn "sport = :$(TEST_PORT)" 2>/dev/null | tail -n +2 | grep -q .; then \
		echo "Test port $(TEST_PORT) is already in use."; \
		echo "Refusing to start a canonical test instance against an occupied endpoint."; \
		$(MAKE) --no-print-directory test-instance-fixture-stop; \
		exit 1; \
	fi; \
	if [ ! -f "$(TEST_CONFIG)" ]; then \
		echo "Canonical test configuration does not exist: $(TEST_CONFIG)"; \
		$(MAKE) --no-print-directory test-instance-fixture-stop; \
		exit 1; \
	fi; \
	if [ -f "$(TEST_PID_FILE)" ]; then \
		pid="$$(cat "$(TEST_PID_FILE)")"; \
		if kill -0 "$$pid" 2>/dev/null; then \
			echo "Test instance is already running with PID $$pid."; \
			echo "URL=$(TEST_URL)"; \
			exit 0; \
		fi; \
		rm -f "$(TEST_PID_FILE)"; \
	fi; \
	echo "=== BUILD TEST BINARY ==="; \
	go build -o "$(TEST_BINARY)" . || { \
		$(MAKE) --no-print-directory test-instance-fixture-stop; \
		exit 1; \
	}; \
	echo; \
	echo "=== TEST CONFIG ==="; \
	echo "$(TEST_CONFIG)"; \
	echo; \
	echo "=== VALIDATE TEST CONFIG ==="; \
	"./$(TEST_BINARY)" --config "$(TEST_CONFIG)" config:validate || { \
		rm -f "$(TEST_BINARY)"; \
		$(MAKE) --no-print-directory test-instance-fixture-stop; \
		exit 1; \
	}; \
	echo; \
	echo "=== START TEST INSTANCE ==="; \
	"./$(TEST_BINARY)" --config "$(TEST_CONFIG)" > "$(TEST_LOG)" 2>&1 & \
	pid="$$!"; \
	echo "$$pid" > "$(TEST_PID_FILE)"; \
	ready=0; \
	for i in $$(seq 1 10); do \
		code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_URL)/api/healthz" 2>/dev/null || true)"; \
		if [ "$$code" = "200" ]; then \
			ready=1; \
			break; \
		fi; \
		if ! kill -0 "$$pid" 2>/dev/null; then \
			break; \
		fi; \
		sleep 1; \
	done; \
	if [ "$$ready" -ne 1 ]; then \
		echo "Test instance failed to become ready."; \
		cat "$(TEST_LOG)" || true; \
		kill "$$pid" 2>/dev/null || true; \
		rm -f "$(TEST_PID_FILE)"; \
		$(MAKE) --no-print-directory test-instance-fixture-stop; \
		exit 1; \
	fi; \
	echo "Test instance started."; \
	echo "Config=$(TEST_CONFIG)"; \
	echo "PID=$$pid"; \
	echo "URL=$(TEST_URL)"; \
	echo "Fixture URL=$(TEST_FIXTURE_URL)"

test-instance-status:
	@set -euo pipefail; \
	if [ ! -f "$(TEST_PID_FILE)" ]; then \
		echo "Test instance is not running."; \
		exit 1; \
	fi; \
	pid="$$(cat "$(TEST_PID_FILE)")"; \
	if ! kill -0 "$$pid" 2>/dev/null; then \
		echo "Test instance PID $$pid is not running."; \
		exit 1; \
	fi; \
	code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_URL)/" 2>/dev/null || true)"; \
	echo "Config=$(TEST_CONFIG)"; \
	echo "PID=$$pid"; \
	echo "URL=$(TEST_URL)"; \
	echo "HTTP=$${code:-unavailable}"; \
	fixture_code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_FIXTURE_URL)" 2>/dev/null || true)"; \
	if [ -f "$(TEST_FIXTURE_PID_FILE)" ]; then \
		fixture_pid="$$(cat "$(TEST_FIXTURE_PID_FILE)")"; \
	else \
		fixture_pid="unavailable"; \
	fi; \
	echo "Fixture PID=$$fixture_pid"; \
	echo "Fixture URL=$(TEST_FIXTURE_URL)"; \
	echo "Fixture HTTP=$${fixture_code:-unavailable}"

test-instance-stop:
	@set -euo pipefail; \
	if [ -f "$(TEST_PID_FILE)" ]; then \
		pid="$$(cat "$(TEST_PID_FILE)")"; \
		if kill -0 "$$pid" 2>/dev/null; then \
			echo "Stopping test instance PID $$pid..."; \
			kill "$$pid"; \
			for i in $$(seq 1 10); do \
				if ! kill -0 "$$pid" 2>/dev/null; then \
					break; \
				fi; \
				sleep 1; \
			done; \
		fi; \
	fi; \
	rm -f "$(TEST_PID_FILE)" "$(TEST_BINARY)" "$(TEST_LOG)"; \
	$(MAKE) --no-print-directory test-instance-fixture-stop; \
	echo "Test instance stopped and runtime artifacts removed."; \
	echo "Preserved $(TEST_CONFIG)."

.PHONY: test-prod-start test-prod-status test-prod-stop

test-prod-start:
	@set -euo pipefail; \
	if [ -z "$(TEST_RUNTIME_CONTAINER)" ]; then \
		echo "TEST_RUNTIME_CONTAINER is required."; \
		echo "Example: make test-prod-start TEST_RUNTIME_CONTAINER=glance"; \
		exit 1; \
	fi; \
	if ! docker inspect "$(TEST_RUNTIME_CONTAINER)" >/dev/null 2>&1; then \
		echo "Runtime reference container does not exist: $(TEST_RUNTIME_CONTAINER)"; \
		exit 1; \
	fi; \
	if docker inspect "$(TEST_PROD_CONTAINER)" >/dev/null 2>&1; then \
		echo "Production-runtime test container already exists: $(TEST_PROD_CONTAINER)"; \
		echo "Run make test-prod-stop first."; \
		exit 1; \
	fi; \
	if docker inspect "$(TEST_CONTAINER)" >/dev/null 2>&1; then \
		echo "Published-image test container already exists: $(TEST_CONTAINER)"; \
		echo "Run make test-container-stop first."; \
		exit 1; \
	fi; \
	if [ -f "$(TEST_PID_FILE)" ]; then \
		pid="$$(cat "$(TEST_PID_FILE)" 2>/dev/null || true)"; \
		if [ -n "$$pid" ] && kill -0 "$$pid" 2>/dev/null; then \
			echo "Canonical test instance is already running."; \
			echo "Run make test-instance-stop first."; \
			exit 1; \
		fi; \
	fi; \
	echo "=== BUILD CURRENT SOURCE TEST IMAGE ==="; \
	docker build -t "$(TEST_PROD_IMAGE)" .; \
	image_id="$$(docker image inspect "$(TEST_PROD_IMAGE)" --format "{{.Id}}")"; \
	echo "Image ID: $$image_id"; \
	echo; \
	echo "=== START WITH PRODUCTION RUNTIME ==="; \
	declare -a env_args mount_args network_args sysctl_args; \
	while IFS= read -r entry; do \
		[ -n "$$entry" ] || continue; \
		key="$${entry%%=*}"; \
		value="$${entry#*=}"; \
		printf -v "$$key" "%s" "$$value"; \
		export "$$key"; \
		env_args+=(-e "$$key"); \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format "{{range .Config.Env}}{{println .}}{{end}}"); \
	while IFS=$$'\t' read -r type source destination rw; do \
		[ -n "$$destination" ] || continue; \
		case "$$type" in \
			bind) \
				if [ "$$rw" = "false" ]; then \
					mount_args+=(-v "$${source}:$${destination}:ro"); \
				else \
					mount_args+=(-v "$${source}:$${destination}"); \
				fi \
				;; \
		esac; \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format "{{range .Mounts}}{{printf \"%s\\t%s\\t%s\\t%t\\n\" .Type .Source .Destination .RW}}{{end}}"); \
	while IFS= read -r network; do \
		[ -n "$$network" ] || continue; \
		network_args+=(--network "$$network"); \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format '{{range $$name, $$network := .NetworkSettings.Networks}}{{println $$name}}{{end}}'); \
	while IFS= read -r sysctl; do \
		[ -n "$$sysctl" ] || continue; \
		sysctl_args+=(--sysctl "$$sysctl"); \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format '{{range $$key, $$value := .HostConfig.Sysctls}}{{printf "%s=%s\n" $$key $$value}}{{end}}'); \
	docker run -d \
		--name "$(TEST_PROD_CONTAINER)" \
		--hostname "$(TEST_PROD_CONTAINER)" \
		--restart=no \
		-p "$(TEST_CONTAINER_PORT):8080" \
		"$${env_args[@]}" \
		"$${mount_args[@]}" \
		"$${network_args[@]}" \
		"$${sysctl_args[@]}" \
		"$(TEST_PROD_IMAGE)" >/dev/null; \
	ready=0; \
	for attempt in $$(seq 1 20); do \
		code="$$(curl -sS -o /dev/null -w "%{http_code}" "$(TEST_CONTAINER_URL)/" 2>/dev/null || true)"; \
		if [ "$$code" = "200" ] || [ "$$code" = "302" ]; then \
			ready=1; \
			break; \
		fi; \
		if [ "$$(docker inspect "$(TEST_PROD_CONTAINER)" --format "{{.State.Running}}" 2>/dev/null || true)" != "true" ]; then \
			break; \
		fi; \
		sleep 1; \
	done; \
	if [ "$$ready" -ne 1 ]; then \
		echo "Production-runtime test failed to become ready."; \
		docker logs --tail 80 "$(TEST_PROD_CONTAINER)" 2>&1 || true; \
		exit 1; \
	fi; \
	container_image="$$(docker inspect "$(TEST_PROD_CONTAINER)" --format "{{.Image}}")"; \
	if [ "$$container_image" != "$$image_id" ]; then \
		echo "Production-runtime container image mismatch."; \
		echo "Built:     $$image_id"; \
		echo "Container: $$container_image"; \
		exit 1; \
	fi; \
	echo "Production-runtime test started."; \
	echo "Mode=current source + production runtime"; \
	echo "Runtime reference=$(TEST_RUNTIME_CONTAINER)"; \
	echo "Container=$(TEST_PROD_CONTAINER)"; \
	echo "Image=$(TEST_PROD_IMAGE)"; \
	echo "URL=$(TEST_CONTAINER_URL)"

test-prod-status:
	@set -euo pipefail; \
	if ! docker inspect "$(TEST_PROD_CONTAINER)" >/dev/null 2>&1; then \
		echo "Production-runtime test container does not exist."; \
		exit 1; \
	fi; \
	state="$$(docker inspect "$(TEST_PROD_CONTAINER)" --format "{{.State.Status}}")"; \
	image="$$(docker inspect "$(TEST_PROD_CONTAINER)" --format "{{.Config.Image}}")"; \
	code="$$(curl -sS -o /dev/null -w "%{http_code}" "$(TEST_CONTAINER_URL)/" 2>/dev/null || true)"; \
	echo "Mode=current source + production runtime"; \
	echo "Container=$(TEST_PROD_CONTAINER)"; \
	echo "Image=$$image"; \
	echo "State=$$state"; \
	echo "URL=$(TEST_CONTAINER_URL)"; \
	echo "HTTP=$${code:-unavailable}"

test-prod-stop:
	@set -euo pipefail; \
	if docker inspect "$(TEST_PROD_CONTAINER)" >/dev/null 2>&1; then \
		docker rm -f "$(TEST_PROD_CONTAINER)" >/dev/null; \
		echo "Removed container $(TEST_PROD_CONTAINER)."; \
	else \
		echo "Production-runtime test container is not present."; \
	fi; \
	if docker image inspect "$(TEST_PROD_IMAGE)" >/dev/null 2>&1; then \
		docker image rm "$(TEST_PROD_IMAGE)" >/dev/null; \
		echo "Removed image $(TEST_PROD_IMAGE)."; \
	fi

.PHONY: test-container-start test-container-status test-container-stop

test-container-start:
	@set -euo pipefail; \
	if [ -z "$(TEST_RUNTIME_CONTAINER)" ]; then \
		echo "TEST_RUNTIME_CONTAINER is required."; \
		echo "Example: make test-container-start TEST_RUNTIME_CONTAINER=<container>"; \
		exit 1; \
	fi; \
	if ! docker inspect "$(TEST_RUNTIME_CONTAINER)" >/dev/null 2>&1; then \
		echo "Runtime reference container does not exist: $(TEST_RUNTIME_CONTAINER)"; \
		exit 1; \
	fi; \
	if docker inspect "$(TEST_CONTAINER)" >/dev/null 2>&1; then \
		echo "Test container already exists: $(TEST_CONTAINER)"; \
		echo "Run make test-container-stop first."; \
		exit 1; \
	fi; \
	echo "=== VERIFY DEVELOPMENT IMAGE ==="; \
	git fetch origin --prune; \
	dev_revision="$$(git rev-parse origin/$(DEV_BRANCH))"; \
	run_id="$$(gh run list \
		--repo "$(REPO)" \
		--workflow "$(IMAGE_WORKFLOW)" \
		--branch "$(DEV_BRANCH)" \
		--limit 20 \
		--json databaseId,headSha,status,conclusion \
		--jq '.[] | select(.headSha == "'"$$dev_revision"'" and .status == "completed" and .conclusion == "success") | .databaseId' \
		| head -1)"; \
	if [ -z "$$run_id" ]; then \
		echo "Refusing container test: no successful dev image build found for $$dev_revision."; \
		echo "Run make image-watch after the dev image workflow starts."; \
		exit 1; \
	fi; \
	echo "Dev revision: $$dev_revision"; \
	echo "Image run:    $$run_id"; \
	echo; \
	echo "=== PULL DEVELOPMENT IMAGE ==="; \
	docker pull "$(TEST_CONTAINER_IMAGE)"; \
	image_id="$$(docker image inspect "$(TEST_CONTAINER_IMAGE)" --format '{{.Id}}')"; \
	image_version="$$(docker run --rm --entrypoint /app/glance "$(TEST_CONTAINER_IMAGE)" --version)"; \
	echo "Image version: $$image_version"; \
	echo "Image ID:      $$image_id"; \
	if [ "$$image_version" != "dev" ]; then \
		echo "Refusing container test: $(TEST_CONTAINER_IMAGE) does not identify as dev."; \
		exit 1; \
	fi; \
	echo; \
	echo "=== START TEST CONTAINER ==="; \
	declare -a env_args mount_args network_args sysctl_args; \
	while IFS= read -r entry; do \
		[ -n "$$entry" ] || continue; \
		key="$${entry%%=*}"; \
		value="$${entry#*=}"; \
		printf -v "$$key" '%s' "$$value"; \
		export "$$key"; \
		env_args+=(-e "$$key"); \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format '{{range .Config.Env}}{{println .}}{{end}}'); \
	while IFS=$$'\t' read -r type source destination rw; do \
		[ -n "$$destination" ] || continue; \
		case "$$type" in \
			bind) \
				if [ "$$rw" = "false" ]; then \
					mount_args+=(-v "$${source}:$${destination}:ro"); \
				else \
					mount_args+=(-v "$${source}:$${destination}"); \
				fi \
				;; \
		esac; \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format '{{range .Mounts}}{{printf "%s\t%s\t%s\t%t\n" .Type .Source .Destination .RW}}{{end}}'); \
	while IFS= read -r network; do \
		[ -n "$$network" ] || continue; \
		network_args+=(--network "$$network"); \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format '{{range $$name, $$network := .NetworkSettings.Networks}}{{println $$name}}{{end}}'); \
	while IFS= read -r sysctl; do \
		[ -n "$$sysctl" ] || continue; \
		sysctl_args+=(--sysctl "$$sysctl"); \
	done < <(docker inspect "$(TEST_RUNTIME_CONTAINER)" --format '{{range $$key, $$value := .HostConfig.Sysctls}}{{printf "%s=%s\n" $$key $$value}}{{end}}'); \
	docker run -d \
		--name "$(TEST_CONTAINER)" \
		--hostname "$(TEST_CONTAINER)" \
		--restart=no \
		-p "$(TEST_CONTAINER_PORT):8080" \
		"$${env_args[@]}" \
		"$${mount_args[@]}" \
		"$${network_args[@]}" \
		"$${sysctl_args[@]}" \
		"$(TEST_CONTAINER_IMAGE)" >/dev/null; \
	ready=0; \
	for attempt in $$(seq 1 20); do \
		code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_CONTAINER_URL)/" 2>/dev/null || true)"; \
		if [ "$$code" = "200" ] || [ "$$code" = "302" ]; then \
			ready=1; \
			break; \
		fi; \
		if [ "$$(docker inspect "$(TEST_CONTAINER)" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]; then \
			break; \
		fi; \
		sleep 1; \
	done; \
	if [ "$$ready" -ne 1 ]; then \
		echo "Test container failed to become ready."; \
		docker logs --tail 80 "$(TEST_CONTAINER)" 2>&1 || true; \
		exit 1; \
	fi; \
	short_revision="$${dev_revision:0:7}"; \
	if ! docker logs "$(TEST_CONTAINER)" 2>&1 | grep -Fq "revision=$$short_revision"; then \
		echo "Test container revision does not match origin/$(DEV_BRANCH): $$short_revision"; \
		docker logs --tail 80 "$(TEST_CONTAINER)" 2>&1 || true; \
		exit 1; \
	fi; \
	container_image="$$(docker inspect "$(TEST_CONTAINER)" --format '{{.Image}}')"; \
	if [ "$$container_image" != "$$image_id" ]; then \
		echo "Test container image does not match pulled dev image."; \
		echo "Pulled:    $$image_id"; \
		echo "Container: $$container_image"; \
		exit 1; \
	fi; \
	echo "Test container started."; \
	echo "Runtime reference=$(TEST_RUNTIME_CONTAINER)"; \
	echo "Revision=$$dev_revision"; \
	echo "Container=$(TEST_CONTAINER)"; \
	echo "Image=$(TEST_CONTAINER_IMAGE)"; \
	echo "URL=$(TEST_CONTAINER_URL)"

test-container-status:
	@set -euo pipefail; \
	if ! docker inspect "$(TEST_CONTAINER)" >/dev/null 2>&1; then \
		echo "Test container does not exist."; \
		exit 1; \
	fi; \
	state="$$(docker inspect "$(TEST_CONTAINER)" --format '{{.State.Status}}')"; \
	code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_CONTAINER_URL)/" 2>/dev/null || true)"; \
	echo "Container=$(TEST_CONTAINER)"; \
	echo "State=$$state"; \
	echo "URL=$(TEST_CONTAINER_URL)"; \
	echo "HTTP=$${code:-unavailable}"

test-container-stop:
	@set -euo pipefail; \
	if docker inspect "$(TEST_CONTAINER)" >/dev/null 2>&1; then \
		docker rm -f "$(TEST_CONTAINER)" >/dev/null; \
		echo "Removed container $(TEST_CONTAINER)."; \
	else \
		echo "Test container is not present."; \
	fi; \
	echo "Preserved image $(TEST_CONTAINER_IMAGE).";

# -----------------------------------------------------------------------------
# Frontend validation and visual QA
# -----------------------------------------------------------------------------

.PHONY: frontend-audit frontend-check frontend-coverage visual-check visual-screenshots visual-docs visual-docs-promote visual-all

frontend-audit:
	@echo "=== FRONTEND ARCHITECTURE AUDIT ==="
	@python3 scripts/audit_frontend.py

frontend-check:
	@echo "=== FRONTEND REGRESSION CHECK ==="
	@bash testdata/visual/run.sh frontend

frontend-coverage:
	@echo "=== FRONTEND JS EXECUTION COVERAGE ==="
	@bash testdata/visual/run.sh frontend-coverage

visual-check:
	@echo "=== VISUAL QA CONTRACT ==="
	@python3 testdata/visual/check-gallery.py

visual-screenshots: visual-check
	@echo "=== VISUAL QA SCREENSHOTS ==="
	@bash testdata/visual/run.sh qa $(if $(DASHBOARD),--dashboard=$(DASHBOARD)) $(if $(PAGE),--page=$(PAGE)) $(if $(VIEWPORT),--viewport=$(VIEWPORT))

visual-docs:
	@echo "=== VISUAL DOCUMENTATION STAGING CONTRACT ==="
	@python3 testdata/visual/check-gallery.py --allow-missing-browser-images
	@echo "=== VISUAL DOCUMENTATION SCREENSHOTS - STAGING ONLY ==="
	@bash testdata/visual/run.sh docs $(if $(DASHBOARD),--dashboard=$(DASHBOARD)) $(if $(PAGE),--page=$(PAGE)) $(if $(IMAGE),--image=$(IMAGE))
	@echo
	@echo "Documentation captures are staged only."
	@echo "Review testdata/visual/docs-staging before promotion."

visual-docs-promote:
	@echo "=== VISUAL DOCUMENTATION PROMOTION CONTRACT ==="
	@python3 testdata/visual/check-gallery.py --allow-missing-browser-images
	@echo "=== PROMOTE APPROVED DOCUMENTATION SCREENSHOTS ==="
	@python3 testdata/visual/promote-docs.py $(if $(DASHBOARD),--dashboard=$(DASHBOARD)) $(if $(PAGE),--page=$(PAGE)) $(if $(IMAGE),--image=$(IMAGE))

visual-all: visual-check
	@echo "=== VISUAL QA + STAGED DOCUMENTATION SCREENSHOTS ==="
	@bash testdata/visual/run.sh all

visual-final: visual-all
	@echo
	@echo "=== PROMOTE FINAL DOCUMENTATION SCREENSHOTS ==="
	@python3 testdata/visual/promote-docs.py
	@echo
	@echo "=== VERIFY FINAL VISUAL CONTRACT ==="
	@$(MAKE) --no-print-directory visual-check
