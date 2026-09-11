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

URL = os.getenv("UPTIME_KUMA_METRICS_URL", "http://localhost:3001/metrics")
username = os.getenv("UPTIME_KUMA_USERNAME")
password = os.getenv("UPTIME_KUMA_PASSWORD")
r = requests.get(URL, auth=(username, password) if username and password else None, timeout=TIMEOUT)
r.raise_for_status()
monitors = []
for line in r.text.splitlines():
    if not line.startswith("monitor_status{"):
        continue
    sample, raw_value = line.rsplit(" ", 1)
    labels = {}
    for part in sample[sample.find("{") + 1:sample.rfind("}")].split(","):
        if "=" in part:
            key, value = part.split("=", 1)
            labels[key.strip()] = value.strip().strip('"')
    status = {"0": "Down", "1": "Up", "2": "Pending"}.get(raw_value.strip(), "Unknown")
    monitors.append((labels.get("monitor_name", "Unknown"), status))
total = len(monitors)
up = sum(s == "Up" for _, s in monitors)
down = sum(s == "Down" for _, s in monitors)
pending = sum(s == "Pending" for _, s in monitors)
unknown = total - up - down - pending
if down:
    state, message = "error", f"{down} monitor{' is' if down == 1 else 's are'} down"
elif pending:
    state, message = "warning", f"{pending} monitor{' is' if pending == 1 else 's are'} pending"
elif unknown:
    state, message = "warning", f"{unknown} monitor{' has' if unknown == 1 else 's have'} unknown status"
elif total:
    state, message = "ok", "All systems operational"
else:
    state, message = "warning", "No monitor data available"
state_map = {"Down": "error", "Up": "ok", "Pending": "warning", "Unknown": "neutral"}
metrics = [
    {"label": "Total", "value": str(total), "state": "neutral"},
    {"label": "Up", "value": str(up), "state": "ok"},
    {"label": "Down", "value": str(down), "state": "error" if down else "neutral"},
]
if pending:
    metrics.append({"label": "Pending", "value": str(pending), "state": "warning"})
if unknown:
    metrics.append({"label": "Unknown", "value": str(unknown), "state": "warning"})
write({"icon": "📡", "updated": now(), "state": state, "message": message, "metrics": metrics,
       "details": [{"label": n, "value": s, "state": state_map[s]} for n, s in monitors if s != "Up"]})
