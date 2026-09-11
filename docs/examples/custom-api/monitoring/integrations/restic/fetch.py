#!/usr/bin/env python3
"""Public reference producer for Glance Custom API Monitoring."""
import json
import os
from datetime import datetime
from pathlib import Path


OUTPUT_FILE = Path(os.getenv("OUTPUT_FILE", "monitoring.json"))

def now():
    return datetime.now().astimezone().strftime("%Y-%m-%d %H:%M")

def write(payload):
    temporary = OUTPUT_FILE.with_suffix(OUTPUT_FILE.suffix + ".tmp")
    temporary.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    temporary.replace(OUTPUT_FILE)
    print(f"Wrote {OUTPUT_FILE}")

import subprocess

MESSAGE_OK = "Backups current"
MESSAGE_OLD = "Latest backup is"
REPOSITORY = os.environ["RESTIC_REPOSITORY"]
env = os.environ.copy()
password_file = os.getenv("RESTIC_PASSWORD_FILE")
if password_file:
    env["RESTIC_PASSWORD_FILE"] = password_file
r = subprocess.run(["restic", "-r", REPOSITORY, "snapshots", "--json"], env=env, capture_output=True, text=True, check=True)
snapshots = json.loads(r.stdout)
if not snapshots:
    raise RuntimeError("Restic returned no snapshots")
latest = max(snapshots, key=lambda s: s["time"])
timestamp = datetime.fromisoformat(latest["time"].replace("Z", "+00:00")).astimezone()
age_hours = max(0, int((datetime.now().astimezone() - timestamp).total_seconds() // 3600))
state = "error" if age_hours >= 48 else "warning" if age_hours >= 24 else "ok"
message = MESSAGE_OK if state == "ok" else f"{MESSAGE_OLD} {age_hours} hours old"
snapshot_id = str(latest.get("short_id") or latest.get("id") or "Unknown")[:8]
stats = latest.get("stats") or {}
size_bytes = stats.get("total_size") or stats.get("size")
latest_size = f"{float(size_bytes) / (1024 ** 3):.2f} GB" if size_bytes is not None else "Unknown"
write({"icon": "💾", "updated": now(), "state": state, "message": message,
       "metrics": [{"label": "Last Backup", "value": f"{age_hours} hours", "state": state},
                   {"label": "Snapshots", "value": str(len(snapshots)), "state": "neutral"},
                   {"label": "Latest Size", "value": latest_size, "state": "neutral"}],
       "details": [], "expandable_label": "Snapshot Details",
       "expandable_items": [{"label": "Snapshot Time", "value": timestamp.strftime("%b %d %I:%M %p %Z"), "state": "neutral"},
                            {"label": "Snapshot ID", "value": snapshot_id, "state": "neutral"},
                            {"label": "Repository", "value": REPOSITORY, "state": "neutral"}]})
