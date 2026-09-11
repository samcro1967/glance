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

BASE_URL = os.getenv("PORTAINER_URL", "http://localhost:9000").rstrip("/")

auth = requests.post(
    f"{BASE_URL}/api/auth",
    json={
        "Username": os.environ["PORTAINER_USERNAME"],
        "Password": os.environ["PORTAINER_PASSWORD"],
    },
    timeout=TIMEOUT,
)
auth.raise_for_status()
headers = {"Authorization": f"Bearer {auth.json()['jwt']}"}

def get(path):
    response = requests.get(
        f"{BASE_URL}{path}",
        headers=headers,
        timeout=TIMEOUT,
    )
    response.raise_for_status()
    return response.json()

endpoints = get("/api/endpoints")
if not endpoints:
    raise RuntimeError("Portainer returned no endpoints")

endpoint_id = os.getenv("PORTAINER_ENDPOINT_ID") or str(endpoints[0]["Id"])

containers = get(
    f"/api/endpoints/{endpoint_id}/docker/containers/json?all=1"
)
images = get(
    f"/api/endpoints/{endpoint_id}/docker/images/json"
)
volumes = get(
    f"/api/endpoints/{endpoint_id}/docker/volumes"
)
networks = get(
    f"/api/endpoints/{endpoint_id}/docker/networks"
)
stacks = get("/api/stacks")
disk = get(
    f"/api/endpoints/{endpoint_id}/docker/system/df"
)
endpoint = get(
    f"/api/endpoints/{endpoint_id}"
)

running = sum(
    container.get("State") == "running"
    for container in containers
)
stopped = len(containers) - running
unhealthy_count = sum(
    "(unhealthy)" in str(container.get("Status", "")).lower()
    for container in containers
)

items = []

for container in containers:
    state = str(container.get("State") or "").lower()
    unhealthy = "(unhealthy)" in str(
        container.get("Status", "")
    ).lower()

    if state == "running" and not unhealthy:
        continue

    names = container.get("Names") or []
    name = (
        names[0].lstrip("/")
        if names
        else str(container.get("Id") or "")[:12]
    )

    items.append({
        "label": name,
        "value": str(
            container.get("Status")
            or container.get("State")
            or "Unknown"
        ),
        "state": "error" if unhealthy else "warning",
    })

volume_items = volumes.get("Volumes") or []

layers_size = disk.get("LayersSize") or 0
if isinstance(layers_size, list):
    disk_bytes = sum(
        float(layer.get("Size") or 0)
        for layer in layers_size
        if isinstance(layer, dict)
    )
elif isinstance(layers_size, (int, float)):
    disk_bytes = float(layers_size)
else:
    disk_bytes = 0

disk_gb = disk_bytes / (1024 ** 3)

snapshots = endpoint.get("Snapshots") or []
docker_version = "Unknown"
if snapshots and isinstance(snapshots[0], dict):
    docker_version = str(
        snapshots[0].get("DockerVersion") or "Unknown"
    )

if unhealthy_count:
    state = "error"
    message = (
        f"{unhealthy_count} container"
        f"{'s' if unhealthy_count != 1 else ''} unhealthy"
    )
    if stopped:
        message += f" · {stopped} not running"
elif stopped:
    state = "warning"
    message = (
        f"{stopped} container"
        f"{'s' if stopped != 1 else ''} not running"
    )
elif containers:
    state = "ok"
    message = "All containers running"
else:
    state = "warning"
    message = "No containers found"

write({
    "icon": "🐳",
    "updated": now(),
    "state": state,
    "message": message,
    "metrics": [
        {
            "label": "Containers",
            "value": str(len(containers)),
            "state": "neutral",
        },
        {
            "label": "Running",
            "value": str(running),
            "state": "ok",
        },
        {
            "label": "Stopped",
            "value": str(stopped),
            "state": "warning" if stopped else "neutral",
        },
        {
            "label": "Unhealthy",
            "value": str(unhealthy_count),
            "state": "error" if unhealthy_count else "neutral",
        },
        {
            "label": "Images",
            "value": str(len(images)),
            "state": "neutral",
        },
        {
            "label": "Volumes",
            "value": str(len(volume_items)),
            "state": "neutral",
        },
        {
            "label": "Networks",
            "value": str(len(networks)),
            "state": "neutral",
        },
        {
            "label": "Stacks",
            "value": str(len(stacks)),
            "state": "neutral",
        },
        {
            "label": "Disk",
            "value": f"{disk_gb:.1f} GB",
            "state": "neutral",
        },
        {
            "label": "Docker",
            "value": docker_version,
            "state": "neutral",
        },
    ],
    "details": [],
    "expandable_label": "Container Details",
    "expandable_items": items,
})
