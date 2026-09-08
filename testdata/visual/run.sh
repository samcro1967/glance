#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

MODE="${1:-qa}"
shift || true

case "$MODE" in
    qa)
        SCREENSHOT_ARGS=()
        ;;
    docs)
        SCREENSHOT_ARGS=(--docs)
        ;;
    all)
        SCREENSHOT_ARGS=(--all)
        ;;
    *)
        echo "ERROR: unknown visual capture mode: $MODE" >&2
        echo "Expected: qa, docs, or all" >&2
        exit 2
        ;;
esac

cleanup() {
    rc=$?
    trap - EXIT INT TERM

    echo
    echo "=== VISUAL QA CLEANUP ==="

    if [ "$rc" -ne 0 ]; then
        echo "Visual QA failed with exit code $rc."
    else
        echo "Visual QA capture complete."
    fi

    # The Makefile owns both Glance and the deterministic fixture API. Keep
    # Glance running after a successful capture for interactive review, which
    # matches the existing visual QA workflow.
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
echo "=== CAPTURE VISUALS ==="
node testdata/visual/screenshots.js "${SCREENSHOT_ARGS[@]}" "$@"
