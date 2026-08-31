SHELL := /bin/bash

export GIT_PAGER := cat
export PAGER := cat
export GH_PAGER := cat
export GIT_EDITOR := true
export GIT_MERGE_AUTOEDIT := no

.PHONY: help deps build test-instance-start test-instance-status test-instance-stop test test-race test-count test-race-count fmt-check diff-check staged-check check coverage vuln status staged-diff upstream-status pr-view pr-runs pr-merge post-merge image-runs ci-watch ci-view verify-main deploy-status deploy

COUNT ?= 10
COVERAGE_FILE ?= coverage.out
BASE_REF ?= origin/main

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
	@echo "  make upstream-status         Refresh and compare main with remotes"
	@echo "  make verify-main             Verify post-merge main repository state"
	@echo
	@echo "GitHub pull requests:"
	@echo "  make pr-view PR=55           Show a pull request"
	@echo "  make pr-runs                 Show recent validation runs for current branch"
	@echo "  make pr-runs BRANCH=main     Show recent validation runs for a branch"
	@echo "  make pr-merge PR=55          Merge a pull request and delete its remote branch"
	@echo "  make post-merge PR=55        Update main and remove the merged local branch"
	@echo
	@echo "GitHub Actions:"
	@echo "  make image-runs              Show recent main container image builds"
	@echo "  make ci-watch RUN=12345      Watch a GitHub Actions run"
	@echo "  make ci-view RUN=12345       Show a GitHub Actions run result"
	@echo
	@echo "Production:"
	@echo "  make deploy-status           Compare source, GHCR, and production revisions"
	@echo "  make deploy                  Pull, verify, and deploy current main to production"

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
	@echo "=== MAIN VS ORIGIN ==="
	@git rev-list --left-right --count origin/main...main
	@echo
	@echo "=== MAIN VS UPSTREAM ==="
	@git rev-list --left-right --count upstream/main...main

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
		echo "BRANCH could not be determined. Example: make pr-runs BRANCH=main"; \
		exit 2; \
	fi
	@gh run list \
		--repo "$(REPO)" \
		--workflow "$(PR_WORKFLOW)" \
		--branch "$(BRANCH)" \
		--limit 5 \
		--json databaseId,headSha,status,conclusion,createdAt,displayTitle

pr-merge:
	@if [ -z "$(PR)" ]; then \
		echo "PR is required. Example: make pr-merge PR=55"; \
		exit 2; \
	fi
	@gh pr merge "$(PR)" \
		--repo "$(REPO)" \
		--merge \
		--delete-branch

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
	if [ "$$pr_state" != "MERGED" ]; then \
		echo "PR #$(PR) is not merged; current state is $$pr_state."; \
		exit 1; \
	fi; \
	echo "State=$$pr_state"; \
	echo "Head=$$head_branch"; \
	echo; \
	echo "=== UPDATE LOCAL MAIN ==="; \
	git switch main; \
	git pull --ff-only origin main; \
	echo; \
	echo "=== LOCAL BRANCH CLEANUP ==="; \
	if [ "$$head_branch" = "main" ]; then \
		echo "PR head branch is main; refusing to delete it."; \
	elif git show-ref --verify --quiet "refs/heads/$$head_branch"; then \
		git branch -d "$$head_branch"; \
	else \
		echo "Local branch $$head_branch does not exist; nothing to delete."; \
	fi; \
	echo; \
	$(MAKE) verify-main

image-runs:
	@gh run list \
		--repo "$(REPO)" \
		--workflow "$(IMAGE_WORKFLOW)" \
		--branch main \
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
	@git rev-list --left-right --count origin/main...main
	@echo
	@echo "=== MAIN VS UPSTREAM ==="
	@git rev-list --left-right --count upstream/main...main

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
	if [ "$$branch" != "main" ]; then \
		echo "Deployment requires branch main; current branch is $$branch."; \
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
	origin_revision="$$(git rev-parse origin/main)"; \
	if [ "$$local_revision" != "$$origin_revision" ]; then \
		echo "Local main does not match origin/main."; \
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
		echo "Refusing deployment: GHCR latest does not match local main."; \
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
