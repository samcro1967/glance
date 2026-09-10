#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

MODE="${1:-qa}"
shift || true

case "$MODE" in
    frontend)
        RUNNER="testdata/visual/frontend-check.js"
        RUNNER_ARGS=()
        ACTIVITY="Frontend regression check"
        ;;
    frontend-coverage)
        RUNNER="testdata/visual/frontend-check.js"
        RUNNER_ARGS=(--coverage=/tmp/glance-frontend-coverage-main.json)
        ACTIVITY="Frontend coverage check"
        ;;
    qa)
        RUNNER="testdata/visual/screenshots.js"
        RUNNER_ARGS=()
        ACTIVITY="Visual QA capture"
        ;;
    docs)
        RUNNER="testdata/visual/screenshots.js"
        RUNNER_ARGS=(--docs)
        ACTIVITY="Documentation screenshot capture"
        ;;
    all)
        RUNNER="testdata/visual/screenshots.js"
        RUNNER_ARGS=(--all)
        ACTIVITY="Visual QA and documentation capture"
        ;;
    *)
        echo "ERROR: unknown visual runner mode: $MODE" >&2
        echo "Expected: frontend, frontend-coverage, qa, docs, or all" >&2
        exit 2
        ;;
esac

cleanup() {
    rc=$?
    trap - EXIT INT TERM

    rm -f /tmp/glance-frontend-coverage-main.json /tmp/glance-frontend-coverage-auth.json

    echo
    echo "=== RUNNER CLEANUP ==="

    if [ "$rc" -ne 0 ]; then
        echo "$ACTIVITY failed with exit code $rc."
    else
        echo "$ACTIVITY complete."
    fi

    # The Makefile owns both Glance and the deterministic fixture API. Preserve
    # the existing shared-runner contract: failed runs stop the test instance,
    # while successful runs leave it available for interactive review.
    if [ "$rc" -ne 0 ]; then
        make test-instance-stop >/dev/null 2>&1 || true
    fi

    exit "$rc"
}

trap cleanup EXIT INT TERM

cd "$ROOT"

echo "=== PREPARE TEST INSTANCE ==="
make test-instance-stop >/dev/null 2>&1 || true

echo
echo "=== START TEST INSTANCE + FIXTURE API ==="
make test-instance-start

echo
if [ "$MODE" = "frontend" ] || [ "$MODE" = "frontend-coverage" ]; then
    echo "=== RUN FRONTEND CHECKS ==="
else
    echo "=== CAPTURE VISUALS ==="
fi
node "$RUNNER" "${RUNNER_ARGS[@]}" "$@"

if [ "$MODE" = "frontend" ] || [ "$MODE" = "frontend-coverage" ]; then
    echo
    echo "=== PREPARE AUTHENTICATION TEST INSTANCE ==="
    make test-instance-stop
    make TEST_CONFIG=glance-test-auth.yml test-instance-start

    echo
    echo "=== RUN AUTHENTICATION FRONTEND CHECKS ==="
    if [ "$MODE" = "frontend-coverage" ]; then
        node "$RUNNER" --auth --coverage=/tmp/glance-frontend-coverage-auth.json
    else
        node "$RUNNER" --auth
    fi

    if [ "$MODE" = "frontend-coverage" ]; then
        echo
        node testdata/visual/frontend-coverage.js \
            /tmp/glance-frontend-coverage-main.json \
            /tmp/glance-frontend-coverage-auth.json
    fi

    echo
    echo "=== RESTORE CANONICAL TEST INSTANCE ==="
    make TEST_CONFIG=glance-test-auth.yml test-instance-stop
    make test-instance-start
fi
