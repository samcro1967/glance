.PHONY: help deps test test-race test-count test-race-count build fmt-check diff-check check coverage vuln

COUNT ?= 10
COVERAGE_FILE ?= coverage.out
BASE_REF ?= origin/main

help:
	@echo "Available targets:"
	@echo "  make deps                    Download Go module dependencies"
	@echo "  make test                    Run the Go test suite"
	@echo "  make test-race               Run the Go test suite with the race detector"
	@echo "  make test-count COUNT=10     Run the Go test suite repeatedly"
	@echo "  make test-race-count COUNT=10"
	@echo "                               Run the race test suite repeatedly"
	@echo "  make build                   Build all Go packages"
	@echo "  make fmt-check               Verify formatting of changed Go files"
	@echo "  make diff-check              Check for whitespace errors"
	@echo "  make check                   Run standard pre-PR validation"
	@echo "  make coverage                Generate test coverage"
	@echo "  make vuln                    Run Go vulnerability analysis"

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

check: test test-race build fmt-check diff-check

coverage:
	go test ./... -coverprofile=$(COVERAGE_FILE)
	go tool cover -func=$(COVERAGE_FILE)

vuln:
	govulncheck ./...
