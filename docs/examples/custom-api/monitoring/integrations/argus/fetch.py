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

BASE_URL = os.getenv("ARGUS_URL", "http://localhost:8080").rstrip("/")
r = requests.get(f"{BASE_URL}/metrics", timeout=TIMEOUT)
r.raise_for_status()
services = {}
service_ids = {}
for line in r.text.splitlines():
    if "latest_version_is_deployed" not in line or line.startswith("#"):
        continue
    try:
        sample, value = line.rsplit(" ", 1)
        labels = {}
        for part in sample[sample.find("{") + 1:sample.rfind("}")].split(","):
            if "=" in part:
                key, raw = part.split("=", 1)
                labels[key.strip()] = raw.strip().strip('"')
        name = labels.get("service_name") or labels.get("name") or labels.get("service") or "Unknown"
        services[name] = float(value)
        if labels.get("service_id"):
            service_ids[name] = labels["service_id"]
    except ValueError:
        continue
updates = sorted(name for name, deployed in services.items() if deployed == 0)
update_items = []
for name in updates:
    value = "Update available"
    service_id = service_ids.get(name)
    if service_id:
        response = requests.get(f"{BASE_URL}/api/v1/service/summary", params={"service_id": service_id}, timeout=TIMEOUT)
        response.raise_for_status()
        summary = response.json()
        deployed_version = summary.get("deployed_version") or summary.get("current_version")
        latest_version = summary.get("latest_version") or summary.get("new_version")
        if deployed_version and latest_version:
            value = f"{deployed_version} → {latest_version}"
    update_items.append({"label": name, "value": value, "state": "warning"})
count = len(updates)
write({"icon": "⬆️", "updated": now(), "state": "warning" if count else "ok",
       "message": f"{count} update{'s' if count != 1 else ''} available" if count else "All services current",
       "metrics": [{"label": "Services", "value": str(len(services)), "state": "neutral"},
                   {"label": "Updates", "value": str(count), "state": "warning" if count else "neutral"},
                   {"label": "Skipped", "value": "0", "state": "neutral"}],
       "details": [], "expandable_label": "Update Details",
       "expandable_items": update_items})
