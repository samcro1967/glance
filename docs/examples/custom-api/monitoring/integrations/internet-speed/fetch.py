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

BASE_URL = os.getenv("SPEEDTEST_TRACKER_URL", "http://localhost:8765").rstrip("/")
TOKEN = os.environ["SPEEDTEST_TRACKER_TOKEN"]
r = requests.get(f"{BASE_URL}/api/v1/results/latest", headers={"Authorization": f"Bearer {TOKEN}", "Accept": "application/json"}, timeout=TIMEOUT)
r.raise_for_status()
result = r.json()["data"]
status = str(result.get("status") or "unknown").lower()
if status in {"waiting", "pending", "running", "started", "processing"}:
    print("Latest test is still in progress; preserving previous output")
    raise SystemExit(0)
benchmarks = result.get("benchmarks") or {}
download_b = benchmarks.get("download") or {}
upload_b = benchmarks.get("upload") or {}

def bstate(item):
    return "ok" if item.get("passed") is True else "error" if item.get("passed") is False else "neutral"

def bvalue(item):
    unit = str(item.get("unit") or "").upper()
    unit = "Mbps" if unit == "MBPS" else unit
    result_text = "Passed" if item.get("passed") is True else "Failed" if item.get("passed") is False else "Unknown"
    return f"{result_text} · {item.get('test_value', 'Unknown')} {unit} · Minimum {item.get('benchmark_value', 'Unknown')} {unit}"

failed = download_b.get("passed") is False or upload_b.get("passed") is False
healthy = result.get("healthy")
state = "warning" if healthy is False or failed else "ok" if healthy is True else "neutral"
details = []
for label, item in (("Download Benchmark", download_b), ("Upload Benchmark", upload_b)):
    if item.get("passed") is False:
        details.append({"label": label, "value": bvalue(item), "state": "error"})
raw = result.get("data") or {}
server = raw.get("server") or {}
server_value = " · ".join(str(server.get(k)) for k in ("name", "location", "country") if server.get(k)) or "Unknown"
items = [
    {"label": "Download Benchmark", "value": bvalue(download_b), "state": bstate(download_b)},
    {"label": "Upload Benchmark", "value": bvalue(upload_b), "state": bstate(upload_b)},
    {"label": "ISP", "value": str(raw.get("isp") or "Unknown"), "state": "neutral"},
    {"label": "Server", "value": server_value, "state": "neutral"},
    {"label": "Jitter", "value": f"{float(raw['ping']['jitter']):.1f} ms" if isinstance(raw.get("ping"), dict) and raw["ping"].get("jitter") is not None else "Unknown", "state": "neutral"},
    {"label": "Packet Loss", "value": f"{float(raw['packetLoss']):.1f}%" if raw.get("packetLoss") is not None else "Unknown", "state": "neutral"},
    {"label": "Test Time", "value": str(result.get("created_at") or result.get("updated_at") or raw.get("timestamp") or "Unknown"), "state": "neutral"},
]
write({
    "icon": "🌐", "updated": now(), "state": state,
    "message": "Internet speed below benchmark" if state == "warning" else "Internet speed healthy" if state == "ok" else "Internet speed result available",
    "metrics": [
        {"label": "Download", "value": str(result.get("download_bits_human") or "Unknown"), "state": bstate(download_b)},
        {"label": "Upload", "value": str(result.get("upload_bits_human") or "Unknown"), "state": bstate(upload_b)},
        {"label": "Ping", "value": f"{float(result['ping']):.1f} ms" if result.get("ping") is not None else "Unknown", "state": "neutral"},
    ],
    "details": details, "expandable_label": "Test Details", "expandable_items": items,
})
