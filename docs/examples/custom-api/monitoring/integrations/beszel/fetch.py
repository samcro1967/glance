#!/usr/bin/env python3

import json
import os
import sys
from datetime import datetime
from pathlib import Path

import requests


BESZEL_URL = os.environ.get("BESZEL_URL", "http://localhost:8090").rstrip("/")
BESZEL_TOKEN = os.environ.get("BESZEL_TOKEN")
OUTPUT_FILE = Path(os.environ.get("OUTPUT_FILE", "beszel_metrics.json"))

WARNING_THRESHOLD = 80.0
ERROR_THRESHOLD = 90.0
REQUEST_TIMEOUT = 10


def fetch_systems():
    if not BESZEL_TOKEN:
        raise RuntimeError("BESZEL_TOKEN is required")

    response = requests.get(
        f"{BESZEL_URL}/api/collections/systems/records",
        headers={
            "Authorization": f"Bearer {BESZEL_TOKEN}",
            "Accept": "application/json",
        },
        timeout=REQUEST_TIMEOUT,
    )
    response.raise_for_status()

    payload = response.json()
    return payload.get("items", [])


def system_state(system):
    if system.get("status") != "up":
        return "error", "Down"

    info = system.get("info") or {}
    resources = {
        "CPU": float(info.get("cpu") or 0),
        "Memory": float(info.get("mp") or 0),
        "Disk": float(info.get("dp") or 0),
    }

    errors = [
        f"{name} {value:.1f}%"
        for name, value in resources.items()
        if value >= ERROR_THRESHOLD
    ]
    if errors:
        return "error", " · ".join(errors)

    warnings = [
        f"{name} {value:.1f}%"
        for name, value in resources.items()
        if value >= WARNING_THRESHOLD
    ]
    if warnings:
        return "warning", " · ".join(warnings)

    return "ok", "Healthy"


def system_detail(system):
    name = system.get("name") or "Unknown system"
    host = system.get("host") or ""
    status = system.get("status") or "unknown"
    info = system.get("info") or {}
    version = info.get("v") or ""

    state, _ = system_state(system)

    if status != "up":
        parts = [status.capitalize()]
    else:
        parts = [
            f"CPU {float(info.get('cpu') or 0):.1f}%",
            f"Memory {float(info.get('mp') or 0):.1f}%",
            f"Disk {float(info.get('dp') or 0):.1f}%",
        ]

        uptime = info.get("u")
        if uptime:
            parts.append(f"Uptime {uptime}")

    if host:
        parts.append(str(host))
    if version:
        parts.append(f"v{version}")

    return {
        "label": name,
        "value": " · ".join(parts),
        "state": state,
    }


def build_payload(systems):
    results = [
        (system, *system_state(system))
        for system in systems
    ]

    up_count = sum(1 for system, _, _ in results if system.get("status") == "up")
    down_count = len(systems) - up_count
    error_count = sum(1 for _, state, _ in results if state == "error")
    warning_count = sum(1 for _, state, _ in results if state == "warning")

    if not systems:
        state = "warning"
        message = "No Beszel systems found"
    elif error_count:
        state = "error"
        message = "1 system needs attention" if error_count == 1 else f"{error_count} systems need attention"
    elif warning_count:
        state = "warning"
        message = "1 system under load" if warning_count == 1 else f"{warning_count} systems under load"
    else:
        state = "ok"
        message = "All systems operational"

    metrics = [
        {"value": str(len(systems)), "label": "Systems"},
        {"value": str(up_count), "label": "Up"},
        {"value": str(down_count), "label": "Down"},
    ]
    if warning_count:
        metrics.append({"value": str(warning_count), "label": "Warnings"})

    details = [
        {
            "label": system.get("name") or "Unknown system",
            "value": reason,
            "state": item_state,
        }
        for system, item_state, reason in results
        if item_state != "ok"
    ]

    expandable_items = sorted(
        (system_detail(system) for system in systems),
        key=lambda item: item["label"].lower(),
    )

    return {
        "icon": "🖥️",
        "updated": datetime.now().strftime("%Y-%m-%d %H:%M"),
        "state": state,
        "message": message,
        "metrics": metrics,
        "details": details,
        "expandable_label": "System Details",
        "expandable_items": expandable_items,
    }


def main():
    systems = fetch_systems()
    payload = build_payload(systems)

    OUTPUT_FILE.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT_FILE.write_text(
        json.dumps(payload, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )


if __name__ == "__main__":
    try:
        main()
    except (requests.RequestException, ValueError, RuntimeError) as exc:
        print(f"Beszel fetch failed: {exc}", file=sys.stderr)
        sys.exit(1)
