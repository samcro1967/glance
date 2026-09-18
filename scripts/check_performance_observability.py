#!/usr/bin/env python3
"""Validate Glance performance-observability architecture contracts.

This is intentionally a structural guard, not a performance budget. Runtime
latencies vary with providers, networks, browsers, and host load; those values
remain engineering evidence rather than deterministic release thresholds.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parent.parent


@dataclass(frozen=True)
class Contract:
    category: str
    path: str
    needles: tuple[str, ...]


CONTRACTS = (
    Contract(
        "outbound-http",
        "internal/glance/outbound-http-diagnostics.go",
        (
            "type outboundHTTPRuntimeDiagnostics",
            "func observeHTTPTransport(",
            "func outboundHTTPDestination(",
            "outboundHTTPDiagnosticsDestinationLimit",
        ),
    ),
    Contract(
        "outbound-http",
        "internal/glance/widget-utils.go",
        ("observeHTTPTransport(defaultHTTPTransport)",),
    ),
    Contract(
        "outbound-http",
        "internal/glance/runtime-diagnostics.go",
        (
            "OutboundHTTP",
            "outboundHTTPRuntimeDiagnosticsResponseFromSnapshot(",
        ),
    ),
    Contract(
        "outbound-http",
        "internal/glance/runtime-diagnostics-report.go",
        ("OUTBOUND HTTP", "response body/decode excluded"),
    ),
    Contract(
        "rendering",
        "internal/glance/render-diagnostics.go",
        ("type renderRuntimeDiagnostics", "func (d *renderRuntimeDiagnostics) snapshot("),
    ),
    Contract(
        "rendering",
        "internal/glance/widget.go",
        (
            "renderDiagnostics.recordWidgetSnapshotHit()",
            "renderDiagnostics.recordWidgetRefreshLockWait(",
            "renderDiagnostics.recordWidgetRender(",
        ),
    ),
    Contract(
        "rendering",
        "internal/glance/glance.go",
        (
            "renderDiagnostics.recordPageLockWait(",
            "renderDiagnostics.recordPageTemplateExecution(",
        ),
    ),
    Contract(
        "rendering",
        "internal/glance/runtime-diagnostics-report.go",
        ("RENDERING", "widget Render() excludes refresh-lock wait"),
    ),
    Contract(
        "profiling",
        "internal/glance/server-runtime.go",
        (
            "runtime.SetMutexProfileFraction(",
            "runtime.SetBlockProfileRate(",
            "enableContentionProfiling()",
            "disableContentionProfiling()",
        ),
    ),
    Contract(
        "frontend",
        "internal/glance/static/js/diagnostics.js",
        (
            '"performance_snapshot"',
            '"navigation_snapshot"',
            '"resource_snapshot"',
            '"memory_snapshot"',
            '"paint_snapshot"',
            '"web_vitals_snapshot"',
            '"largest-contentful-paint"',
            "finalizeFrontendDiagnosticLCP",
            "lcp_finalized",
            "frontendDiagnosticResourceIsPersistent",
            "persistent_count",
            '"layout-shift"',
            '"event"',
        ),
    ),
    Contract(
        "frontend",
        "internal/glance/frontend-diagnostics.go",
        ('"paint_snapshot"', '"web_vitals_snapshot"'),
    ),
    Contract(
        "integration",
        "internal/glance/glance.go",
        (
            '"GET /api/diagnostics"',
            '"GET /api/diagnostics/report"',
            '"POST /api/frontend-diagnostics/performance-snapshot"',
        ),
    ),
)


def main() -> int:
    failures: list[str] = []
    categories: dict[str, int] = {}

    print("=== PERFORMANCE OBSERVABILITY CHECK ===")

    for contract in CONTRACTS:
        categories[contract.category] = categories.get(contract.category, 0) + 1
        path = ROOT / contract.path
        if not path.is_file():
            failures.append(f"{contract.category}: missing file {contract.path}")
            continue

        content = path.read_text(encoding="utf-8")
        for needle in contract.needles:
            if needle not in content:
                failures.append(
                    f"{contract.category}: {contract.path} missing required contract: {needle}"
                )

    for category in categories:
        category_failures = [f for f in failures if f.startswith(f"{category}:")]
        if category_failures:
            print(f"FAIL  {category}")
        else:
            print(f"PASS  {category}")

    if failures:
        print("\nOBSERVABILITY CONTRACT FAILURES")
        print("-------------------------------")
        for failure in failures:
            print(f"- {failure}")
        return 1

    print("\nAll required performance observability surfaces are present.")
    print("Runtime performance values remain informational; no latency thresholds are enforced.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
