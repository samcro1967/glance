SHELL := /bin/bash

export GIT_PAGER := cat
export PAGER := cat
export GH_PAGER := cat
export GIT_EDITOR := true
export GIT_MERGE_AUTOEDIT := no

.PHONY: help deps build test-instance-start test-instance-status test-instance-stop test test-race test-count test-race-count fmt-check diff-check staged-check check coverage vuln status staged-diff upstream-status branch push pr-create promote-create pr-view pr-runs pr-merge post-merge image-runs ci-watch ci-view verify-dev verify-main release-status release-check release deploy-status deploy

COUNT ?= 10
COVERAGE_FILE ?= coverage.out
BASE_REF ?= origin/dev

TEST_PORT ?= 18080
TEST_BINARY ?= .glance-test
TEST_CONFIG ?= glance-test.yml
TEST_PID_FILE ?= .glance-test.pid
TEST_LOG ?= .glance-test.log
TEST_URL ?= http://127.0.0.1:$(TEST_PORT)

REPO ?= samcro1967/glance
PR_WORKFLOW ?= 345456314
IMAGE_WORKFLOW ?= 344869583
BRANCH ?= $(shell git branch --show-current 2>/dev/null)
NEW_BRANCH ?=

DEV_BRANCH ?= dev
STABLE_BRANCH ?= main
PR_BASE ?= $(DEV_BRANCH)

FORK_RELEASE_ID ?= samcro1967
FORK_RELEASE_WIDTH ?= 3

DEPLOY_IMAGE ?= ghcr.io/samcro1967/glance:latest
DEPLOY_CONTAINER ?= glance
DEPLOY_SERVICE ?= glance
DEPLOY_DIR ?= /home/osuhickeys/Documents/Docker
DEPLOY_URL ?= http://127.0.0.1:8092/
DEPLOY_RETRIES ?= 6
DEPLOY_RETRY_DELAY ?= 2

help:
	@echo "Available targets:"
	@echo
	@echo "Development:"
	@echo "  make deps                    Download Go module dependencies"
	@echo "  make build                   Build all Go packages"
	@echo "  make test-instance-start     Start isolated Glance on port $(TEST_PORT)"
	@echo "  make test-instance-status    Show isolated Glance status"
	@echo "  make test-instance-stop      Stop isolated Glance and clean artifacts"
	@echo
	@echo "Testing:"
	@echo "  make test                    Run the Go test suite"
	@echo "  make test-race               Run the Go test suite with the race detector"
	@echo "  make test-count COUNT=10     Run the Go test suite repeatedly"
	@echo "  make test-race-count COUNT=10"
	@echo "                               Run the race test suite repeatedly"
	@echo "  make coverage                Generate test coverage"
	@echo "  make vuln                    Run Go vulnerability analysis"
	@echo
	@echo "Validation:"
	@echo "  make fmt-check               Verify formatting of changed Go files"
	@echo "  make diff-check              Check working-tree whitespace errors"
	@echo "  make staged-check            Check staged whitespace errors"
	@echo "  make check                   Run standard pre-PR validation"
	@echo
	@echo "Repository inspection:"
	@echo "  make status                  Show branch, latest commit, and status"
	@echo "  make staged-diff             Show staged diff summary and contents"
	@echo "  make upstream-status         Refresh and compare dev/main with remotes"
	@echo "  make verify-dev              Verify dev repository state"
	@echo "  make verify-main             Verify main repository state"
	@echo "  make branch NEW_BRANCH=name  Create a feature branch from clean, current dev"
	@echo "  make push                    Push current feature branch to origin"
	@echo
	@echo "GitHub pull requests:"
	@echo "  make pr-create TITLE='...' BODY_FILE=file"
	@echo "                               Create a PR from feature branch to dev"
	@echo "  make promote-create TITLE='...' BODY_FILE=file"
	@echo "                               Create a promotion PR from dev to main"
	@echo "  make pr-view PR=55           Show a pull request"
	@echo "  make pr-runs                 Show recent validation runs for current branch"
	@echo "  make pr-runs BRANCH=dev      Show recent validation runs for a branch"
	@echo "  make pr-merge PR=55          Merge a pull request and delete feature branch"
	@echo "  make post-merge PR=55        Update PR base and clean merged feature branch"
	@echo
	@echo "GitHub Actions:"
	@echo "  make image-runs              Show recent dev container image builds"
	@echo "  make ci-watch RUN=12345      Watch a GitHub Actions run"
	@echo "  make ci-view RUN=12345       Show a GitHub Actions run result"
	@echo
	@echo "Releases:"
	@echo "  make release-status          Show upstream baseline and next fork release"
	@echo "  make release-check           Validate main for a formal fork release"
	@echo "  make release                 Validate, tag, and push the next fork release"
	@echo
	@echo "Production:"
	@echo "  make deploy-status           Compare source, GHCR, and production revisions"
	@echo "  make deploy                  Deploy the current formal GHCR release"

deps:
	go mod download

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

check: test test-race build fmt-check diff-check staged-check

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
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Local $(DEV_BRANCH) does not match origin/$(DEV_BRANCH)."; \
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

pr-merge:
	@set -euo pipefail; \
	if [ -z "$(PR)" ]; then \
		echo "PR is required. Example: make pr-merge PR=55"; \
		exit 2; \
	fi; \
	head_branch="$$(gh pr view "$(PR)" --repo "$(REPO)" --json headRefName --jq '.headRefName')"; \
	if [ "$$head_branch" = "$(DEV_BRANCH)" ]; then \
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
	if git show-ref --verify --quiet "refs/heads/$$base_branch"; then \
		git switch "$$base_branch"; \
	else \
		git switch -c "$$base_branch" --track "origin/$$base_branch"; \
	fi; \
	git pull --ff-only origin "$$base_branch"; \
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

image-runs:
	@gh run list \
		--repo "$(REPO)" \
		--workflow "$(IMAGE_WORKFLOW)" \
		--branch "$(DEV_BRANCH)" \
		--limit 5 \
		--json databaseId,headSha,status,conclusion,createdAt,displayTitle

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
	@echo "=== BRANCH ==="
	@git branch --show-current
	@echo
	@echo "=== DEV ==="
	@git log -1 --oneline --decorate
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
	@echo "=== BRANCH ==="
	@git branch --show-current
	@echo
	@echo "=== MAIN ==="
	@git log -1 --oneline --decorate
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

deploy-status:
	@echo "=== SOURCE ==="
	@echo "Branch=$$(git branch --show-current)"
	@echo "Revision=$$(git rev-parse HEAD)"
	@echo
	@echo "=== LOCAL GHCR IMAGE ==="
	@if docker image inspect "$(DEPLOY_IMAGE)" >/dev/null 2>&1; then \
		docker image inspect "$(DEPLOY_IMAGE)" \
			--format 'Revision={{index .Config.Labels "org.opencontainers.image.revision"}} Image={{.Id}} Created={{.Created}}'; \
	else \
		echo "Image $(DEPLOY_IMAGE) is not available locally."; \
	fi
	@echo
	@echo "=== PRODUCTION ==="
	@if docker inspect "$(DEPLOY_CONTAINER)" >/dev/null 2>&1; then \
		docker inspect "$(DEPLOY_CONTAINER)" \
			--format 'Revision={{index .Config.Labels "org.opencontainers.image.revision"}} Image={{.Image}} Started={{.State.StartedAt}}'; \
	else \
		echo "Container $(DEPLOY_CONTAINER) does not exist."; \
	fi

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
	git fetch origin --prune; \
	local_revision="$$(git rev-parse HEAD)"; \
	origin_revision="$$(git rev-parse origin/$(STABLE_BRANCH))"; \
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Local $(STABLE_BRANCH) does not match origin/$(STABLE_BRANCH)."; \
		echo "Local:  $$local_revision"; \
		echo "Origin: $$origin_revision"; \
		exit 1; \
	fi; \
	echo "Source revision: $$local_revision"; \
	echo; \
	echo "=== PULL IMAGE ==="; \
	docker pull "$(DEPLOY_IMAGE)"; \
	image_revision="$$(docker image inspect "$(DEPLOY_IMAGE)" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')"; \
	image_id="$$(docker image inspect "$(DEPLOY_IMAGE)" --format '{{.Id}}')"; \
	echo "Image revision: $$image_revision"; \
	echo "Image ID:       $$image_id"; \
	if [ "$$image_revision" != "$$local_revision" ]; then \
		echo "Refusing deployment: GHCR latest does not match local $(STABLE_BRANCH)."; \
		echo "Source: $$local_revision"; \
		echo "Image:  $$image_revision"; \
		exit 1; \
	fi; \
	echo; \
	echo "=== DEPLOY ==="; \
	cd "$(DEPLOY_DIR)"; \
	docker compose up -d --no-deps "$(DEPLOY_SERVICE)"; \
	echo; \
	echo "=== VERIFY CONTAINER ==="; \
	container_revision="$$(docker inspect "$(DEPLOY_CONTAINER)" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')"; \
	container_image="$$(docker inspect "$(DEPLOY_CONTAINER)" --format '{{.Image}}')"; \
	echo "Container revision: $$container_revision"; \
	echo "Container image:    $$container_image"; \
	if [ "$$container_revision" != "$$local_revision" ]; then \
		echo "Deployment verification failed: container revision does not match source."; \
		exit 1; \
	fi; \
	if [ "$$container_image" != "$$image_id" ]; then \
		echo "Deployment verification failed: container image does not match pulled image."; \
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
	echo "Revision=$$container_revision"; \
	echo "Image=$$container_image"

test-instance-start:
	@set -euo pipefail; \
	if [ -f "$(TEST_PID_FILE)" ]; then \
		pid="$$(cat "$(TEST_PID_FILE)")"; \
		if kill -0 "$$pid" 2>/dev/null; then \
			echo "Test instance is already running with PID $$pid."; \
			echo "URL=$(TEST_URL)"; \
			exit 1; \
		fi; \
		rm -f "$(TEST_PID_FILE)"; \
	fi; \
	echo "=== BUILD TEST BINARY ==="; \
	go build -o "$(TEST_BINARY)" .; \
	echo; \
	echo "=== CREATE TEST CONFIG ==="; \
	printf '%s\n' \
		'server:' \
		'  host: 0.0.0.0' \
		'  port: $(TEST_PORT)' \
		'' \
		'branding:' \
		'  app-name: "Glance Test"' \
		'' \
		'theme:' \
		'  light: false' \
		'' \
		'pages:' \
		'  - name: Home' \
		'    slug: home' \
		'    columns:' \
		'      - size: full' \
		'        widgets:' \
		'          - type: html' \
		'            source: "<p>Home test page</p>"' \
		'' \
		'  - name: News' \
		'    slug: news' \
		'    columns:' \
		'      - size: full' \
		'        widgets:' \
		'          - type: html' \
		'            source: "<p>News test page</p>"' \
		> "$(TEST_CONFIG)"; \
	echo; \
	echo "=== VALIDATE TEST CONFIG ==="; \
	"./$(TEST_BINARY)" --config "$(TEST_CONFIG)" config:validate; \
	echo; \
	echo "=== START TEST INSTANCE ==="; \
	"./$(TEST_BINARY)" --config "$(TEST_CONFIG)" > "$(TEST_LOG)" 2>&1 & \
	pid="$$!"; \
	echo "$$pid" > "$(TEST_PID_FILE)"; \
	ready=0; \
	for i in $$(seq 1 10); do \
		code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(TEST_URL)/" 2>/dev/null || true)"; \
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
		exit 1; \
	fi; \
	echo "Test instance started."; \
	echo "PID=$$pid"; \
	echo "URL=$(TEST_URL)"

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
	echo "PID=$$pid"; \
	echo "URL=$(TEST_URL)"; \
	echo "HTTP=$${code:-unavailable}"

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
	rm -f "$(TEST_PID_FILE)" "$(TEST_BINARY)" "$(TEST_CONFIG)" "$(TEST_LOG)"; \
	echo "Test instance stopped and artifacts removed."
