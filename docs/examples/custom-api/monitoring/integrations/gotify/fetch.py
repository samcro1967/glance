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
    temporary.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    temporary.replace(OUTPUT_FILE)
    print(f"Wrote {OUTPUT_FILE}")

BASE_URL = os.getenv("GOTIFY_URL", "http://localhost:80").rstrip("/")
TOKEN = os.environ["GOTIFY_TOKEN"]
params = {"token": TOKEN}
health = requests.get(f"{BASE_URL}/health", params=params, timeout=TIMEOUT)
health.raise_for_status()
apps_r = requests.get(f"{BASE_URL}/application", params=params, timeout=TIMEOUT)
apps_r.raise_for_status()
apps = apps_r.json()
messages_r = requests.get(f"{BASE_URL}/message", params={**params, "limit": 100}, timeout=TIMEOUT)
messages_r.raise_for_status()
data = messages_r.json()
messages = data.get("messages", []) if isinstance(data, dict) else data
counts = {}
for item in messages:
    app_id = item.get("appid")
    counts[app_id] = counts.get(app_id, 0) + 1
write({"icon": "🔔", "updated": now(), "state": "ok", "message": "Gotify operational",
       "metrics": [{"label": "Applications", "value": str(len(apps)), "state": "neutral"},
                   {"label": "Messages", "value": str(len(messages)), "state": "neutral"},
                   {"label": "Health", "value": "Healthy", "state": "ok"},
                   {"label": "Version", "value": str(health.json().get("version") or "Unknown"), "state": "neutral"}],
       "details": [], "expandable_label": "Application Details",
       "expandable_items": [{"label": str(app.get("name") or f"Application {app.get('id')}"),
                             "value": f"{counts.get(app.get('id'), 0)} messages", "state": "neutral"}
                            for app in apps if counts.get(app.get("id"), 0)]})
