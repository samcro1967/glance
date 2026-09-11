#!/usr/bin/env python3
"""Public reference producer for Glance Custom API Monitoring."""
import json
import os
from datetime import datetime
from pathlib import Path

import requests

TIMEOUT = float(os.getenv("REQUEST_TIMEOUT", "15"))
OUTPUT_FILE = Path(os.getenv("OUTPUT_FILE", "monitoring.json"))

def now():
    return datetime.now().astimezone().strftime("%Y-%m-%d %H:%M")

def write(payload):
    temporary = OUTPUT_FILE.with_suffix(OUTPUT_FILE.suffix + ".tmp")
    temporary.write_text(
        json.dumps(payload, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    temporary.replace(OUTPUT_FILE)
    print(f"Wrote {OUTPUT_FILE}")

BASE_URL = os.getenv(
    "SABNZBD_URL",
    "http://localhost:8080",
).rstrip("/")
API_KEY = os.environ["SABNZBD_API_KEY"]

def api(mode, **extra):
    response = requests.get(
        f"{BASE_URL}/api",
        params={
            "apikey": API_KEY,
            "output": "json",
            "mode": mode,
            **extra,
        },
        timeout=TIMEOUT,
    )
    response.raise_for_status()
    return response.json()

queue = api("queue").get("queue", {})
history = api("history", limit=10).get("history", {})

queue_slots = queue.get("slots") or []
history_slots = history.get("slots") or []

status = str(queue.get("status") or "Unknown")
status_lower = status.lower()

warnings = queue.get("warnings") or []
warning_count = len(warnings)

if warning_count:
    state = "warning"
    message = (
        f"{warning_count} SABnzbd "
        f"{'warning' if warning_count == 1 else 'warnings'}"
    )
elif status_lower in {"downloading", "fetching"}:
    state = "ok"
    message = "SABnzbd downloading"
elif status_lower == "idle":
    state = "ok"
    message = "SABnzbd idle"
elif status_lower == "paused" or queue.get("paused") is True:
    state = "warning"
    message = "SABnzbd paused"
elif status_lower in {
    "checking",
    "repairing",
    "extracting",
    "moving",
}:
    state = "ok"
    message = f"SABnzbd {status_lower}"
elif status_lower in {"unknown", ""}:
    state = "warning"
    message = "SABnzbd status unknown"
else:
    state = "ok"
    message = f"SABnzbd {status_lower}"

details = [
    {
        "label": "Warning",
        "value": str(warning),
        "state": "warning",
    }
    for warning in warnings
]

items = []

if queue_slots:
    expandable_label = "Queue"

    for slot in queue_slots[:10]:
        name = str(
            slot.get("filename")
            or slot.get("name")
            or "Download"
        )
        slot_status = str(
            slot.get("status") or "Unknown"
        )

        parts = [slot_status]

        remaining = slot.get("mbleft")
        if remaining is not None:
            parts.append(f"{remaining} MB left")

        category = slot.get("cat")
        if category:
            parts.append(str(category))

        items.append({
            "label": name,
            "value": " · ".join(parts),
            "state": "neutral",
        })
else:
    expandable_label = "Recent Downloads"

    for slot in history_slots[:10]:
        name = str(
            slot.get("name")
            or slot.get("nzb_name")
            or "Download"
        )
        slot_status = str(
            slot.get("status") or "Unknown"
        )

        items.append({
            "label": name,
            "value": slot_status,
            "state": (
                "ok"
                if slot_status.lower() == "completed"
                else "neutral"
            ),
        })

today = (
    history.get("day_size")
    or queue.get("day_size")
    or "Unknown"
)
week = (
    history.get("week_size")
    or queue.get("week_size")
    or "Unknown"
)

write({
    "icon": "⚡",
    "updated": now(),
    "state": state,
    "message": message,
    "metrics": [
        {
            "label": "Status",
            "value": status,
            "state": (
                "warning"
                if state == "warning"
                else "ok"
            ),
        },
        {
            "label": "Queue",
            "value": str(len(queue_slots)),
            "state": (
                "warning"
                if queue_slots
                else "neutral"
            ),
        },
        {
            "label": "Speed",
            "value": str(queue.get("speed") or "0 B/s"),
            "state": "neutral",
        },
        {
            "label": "Today",
            "value": str(today),
            "state": "neutral",
        },
        {
            "label": "This Week",
            "value": str(week),
            "state": "neutral",
        },
        {
            "label": "Free Space",
            "value": str(
                queue.get("diskspace1")
                or "Unknown"
            ),
            "state": "neutral",
        },
        {
            "label": "Version",
            "value": str(
                queue.get("version")
                or "Unknown"
            ),
            "state": "neutral",
        },
    ],
    "details": details,
    "expandable_label": expandable_label,
    "expandable_items": items,
})
