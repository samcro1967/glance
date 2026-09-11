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

BASE_URL = os.getenv("DRONE_URL", "http://localhost:8080").rstrip("/")
TOKEN = os.environ["DRONE_TOKEN"]
headers = {"Authorization": f"Bearer {TOKEN}", "Accept": "application/json"}
r = requests.get(f"{BASE_URL}/api/user/repos", headers=headers, timeout=TIMEOUT)
r.raise_for_status()
repos = r.json()
active = []
for repo in repos:
    if not repo.get("active", True):
        continue
    namespace = repo.get("namespace") or repo.get("owner")
    name = repo.get("name")
    if not namespace or not name:
        continue
    build = requests.get(f"{BASE_URL}/api/repos/{namespace}/{name}/builds/latest", headers=headers, timeout=TIMEOUT)
    if build.status_code == 404:
        continue
    build.raise_for_status()
    active.append((f"{namespace}/{name}", str(build.json().get("status") or "unknown").lower()))
success = sum(s == "success" for _, s in active)
failed = sum(s in {"failure", "error", "killed"} for _, s in active)
pending = sum(s in {"pending", "running"} for _, s in active)
state = "error" if failed else "warning" if pending else "ok"
message = f"{failed} active build{'s' if failed != 1 else ''} failed" if failed else f"{pending} build{'s' if pending != 1 else ''} pending" if pending else "All active builds successful"
write({"icon": "🚁", "updated": now(), "state": state, "message": message,
       "metrics": [{"label": "Repositories", "value": str(len(repos)), "state": "neutral"},
                   {"label": "Active", "value": str(len(active)), "state": "neutral"},
                   {"label": "Successful", "value": str(success), "state": "ok"},
                   {"label": "Failed", "value": str(failed), "state": "error" if failed else "neutral"},
                   {"label": "Pending", "value": str(pending), "state": "warning" if pending else "neutral"}],
       "details": [{"label": n, "value": s.capitalize(), "state": "error" if s in {"failure", "error", "killed"} else "warning"} for n, s in active if s != "success"]})
