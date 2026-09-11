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

BASE_URL = os.getenv("SONARR_URL", "http://localhost:8989").rstrip("/")
API_KEY = os.environ["SONARR_API_KEY"]
HEADERS = {"X-Api-Key": API_KEY}
def api(path, params=None):
    r = requests.get(f"{BASE_URL}/api/v3/{path}", headers=HEADERS, params=params, timeout=TIMEOUT)
    r.raise_for_status()
    return r.json()

series = api("series")
queue = api("queue", {"page": 1, "pageSize": 1000})
status = api("system/status")
queue_records = queue.get("records") or []
queue_size_mb = sum(float(x.get("sizeleft") or x.get("size") or 0) for x in queue_records) / (1024 * 1024)
monitored = [s for s in series if s.get("monitored")]
missing = []
downloaded = 0
for item in series:
    stats = item.get("statistics") or {}
    downloaded += int(stats.get("episodeFileCount") or 0)
    if item.get("monitored"):
        count = max(0, int(stats.get("episodeCount") or 0) - int(stats.get("episodeFileCount") or 0))
        if count:
            missing.append((str(item.get("title") or "Unknown"), count))
missing_episodes = sum(count for _, count in missing)
write({"icon": "📺", "updated": now(), "state": "ok",
       "message": "Sonarr operational",
       "metrics": [{"label": "Series", "value": str(len(series)), "state": "neutral"},
                   {"label": "Monitored", "value": str(len(monitored)), "state": "ok"},
                   {"label": "Missing Series", "value": str(len(missing)), "state": "warning" if missing else "neutral"},
                   {"label": "Missing Episodes", "value": str(missing_episodes), "state": "warning" if missing else "neutral"},
                   {"label": "Downloaded", "value": str(downloaded), "state": "neutral"},
                   {"label": "Queue", "value": str(queue.get("totalRecords", 0)), "state": "neutral"},
                   {"label": "Queue Size", "value": f"{queue_size_mb:.1f} MB", "state": "neutral"},
                   {"label": "Version", "value": str(status.get("version") or "Unknown"), "state": "neutral"}],
       "details": [], "expandable_label": "Missing Series",
       "expandable_items": [{"label": n, "value": f"{c} episode{'s' if c != 1 else ''}", "state": "warning"} for n, c in sorted(missing, key=lambda x: x[1], reverse=True)[:20]]})
