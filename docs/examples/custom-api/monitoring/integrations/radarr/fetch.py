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

BASE_URL = os.getenv("RADARR_URL", "http://localhost:7878").rstrip("/")
API_KEY = os.environ["RADARR_API_KEY"]
HEADERS = {"X-Api-Key": API_KEY}
def api(path, params=None):
    r = requests.get(f"{BASE_URL}/api/v3/{path}", headers=HEADERS, params=params, timeout=TIMEOUT)
    r.raise_for_status()
    return r.json()

movies = api("movie")
queue = api("queue", {"page": 1, "pageSize": 1000})
status = api("system/status")
queue_records = queue.get("records") or []
queue_size_mb = sum(float(x.get("sizeleft") or x.get("size") or 0) for x in queue_records) / (1024 * 1024)
monitored = [m for m in movies if m.get("monitored")]
wanted = [m for m in monitored if not m.get("hasFile")]
downloaded = sum(bool(m.get("hasFile")) for m in movies)
write({"icon": "🎬", "updated": now(), "state": "ok",
       "message": "Radarr operational",
       "metrics": [{"label": "Movies", "value": str(len(movies)), "state": "neutral"},
                   {"label": "Monitored", "value": str(len(monitored)), "state": "ok"},
                   {"label": "Downloaded", "value": str(downloaded), "state": "neutral"},
                   {"label": "Wanted", "value": str(len(wanted)), "state": "warning" if wanted else "neutral"},
                   {"label": "Queue", "value": str(queue.get("totalRecords", 0)), "state": "neutral"},
                   {"label": "Queue Size", "value": f"{queue_size_mb:.1f} MB", "state": "neutral"},
                   {"label": "Version", "value": str(status.get("version") or "Unknown"), "state": "neutral"}],
       "details": [], "expandable_label": "Wanted Movies",
       "expandable_items": [{"label": str(m.get("title") or "Unknown"), "value": str(m.get("year") or "Wanted"), "state": "warning"} for m in wanted[:20]]})
