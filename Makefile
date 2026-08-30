.PHONY: help deps test test-race test-count test-race-count build fmt-check diff-check staged-check check coverage vuln status staged-diff upstream-status pr-view pr-runs ci-watch ci-view verify-main

COUNT ?= 10
COVERAGE_FILE ?= coverage.out
BASE_REF ?= origin/main
REPO ?= samcro1967/glance
PR_WORKFLOW ?= 345456314
BRANCH ?= $(shell git branch --show-current 2>/dev/null)

help:
	@echo "Available targets:"
	@echo
	@echo "Development:"
	@echo "  make deps                    Download Go module dependencies"
	@echo "  make build                   Build all Go packages"
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
	@echo "GitHub inspection:"
	@echo "  make pr-view PR=55           Show a pull request"
	@echo "  make pr-runs                 Show recent validation runs for current branch"
	@echo "  make pr-runs BRANCH=main     Show recent validation runs for a branch"
	@echo "  make ci-watch RUN=12345      Watch a GitHub Actions run"
	@echo "  make ci-view RUN=12345       Show a GitHub Actions run result"

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
