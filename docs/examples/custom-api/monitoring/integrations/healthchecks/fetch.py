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

BASE_URL = os.getenv("HEALTHCHECKS_URL", "http://localhost:8000").rstrip("/")
API_KEY = os.environ["HEALTHCHECKS_API_KEY"]
r = requests.get(f"{BASE_URL}/api/v3/checks/", headers={"X-Api-Key": API_KEY, "Accept": "application/json"}, timeout=TIMEOUT)
r.raise_for_status()
data = r.json()
checks = data.get("checks", []) if isinstance(data, dict) else data
up = sum(str(c.get("status")).lower() == "up" for c in checks)
due = sum(str(c.get("status")).lower() == "grace" for c in checks)
down = sum(str(c.get("status")).lower() == "down" for c in checks)
state = "error" if down else "warning" if due else "ok"
message = f"{down} check{' is' if down == 1 else 's are'} down" if down else f"{due} check{' is' if due == 1 else 's are'} due" if due else "All checks are up"
items = []
for check in checks:
    status = str(check.get("status") or "unknown").lower()
    if status not in {"grace", "down"}:
        continue
    value = "Due" if status == "grace" else "Down"
    if check.get("last_ping"):
        value += f" · Last {check['last_ping']}"
    items.append({"label": str(check.get("name") or "Unnamed check"), "value": value, "state": "warning" if status == "grace" else "error"})
write({"icon": "❤️", "updated": now(), "state": state, "message": message,
       "metrics": [{"label": "Checks", "value": str(len(checks)), "state": "neutral"},
                   {"label": "Up", "value": str(up), "state": "ok"},
                   {"label": "Due", "value": str(due), "state": "warning" if due else "neutral"},
                   {"label": "Down", "value": str(down), "state": "error" if down else "neutral"}],
       "details": [], "expandable_label": "Check Details", "expandable_items": items})
