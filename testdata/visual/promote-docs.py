#!/usr/bin/env python3
"""Promote an exact, reviewed documentation screenshot staging set."""

import argparse
import json
import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / "testdata/visual/docs-images.json"
STAGING = ROOT / "testdata/visual/docs-staging"
DESTINATION = ROOT / "docs/images"

parser = argparse.ArgumentParser(
    description="Promote reviewed Glance documentation screenshots."
)
group = parser.add_mutually_exclusive_group()
group.add_argument("--dashboard")
group.add_argument("--page")
group.add_argument("--image")
args = parser.parse_args()

visual_pages = json.loads(
    (ROOT / "testdata/visual/visual-pages.json").read_text()
)

qa_page_routes = {
    "layout-composition": "/layout-composition",
    "feeds-content": "/feeds-content",
    "search-custom-content": "/search-custom-content",
    "date-time-weather": "/date-time-weather",
    "homelab-monitoring": "/homelab-monitoring",
    "development-releases": "/development-releases",
    "markets-streaming": "/markets-streaming",
    "utilities": "/utilities",
    "theme-global": "/themes/",
    "theme-dark-page": "/themes/theme-dark-page",
    "theme-light-page": "/themes/theme-light-page",
    "theme-partial": "/themes/theme-partial",
    "theme-components": "/themes/theme-components",
    "theme-composition": "/themes/theme-composition",
}

def page_route(page):
    return qa_page_routes.get(page, f"/screenshots/{page}")

allowed_routes = None

if args.dashboard:
    if args.dashboard not in visual_pages:
        raise SystemExit(
            f"ERROR: unknown visual dashboard: {args.dashboard}"
        )
    allowed_routes = {
        page_route(page)
        for page in visual_pages[args.dashboard]
    }

if args.page:
    known_pages = {
        page
        for pages in visual_pages.values()
        for page in pages
    }
    if args.page not in known_pages:
        raise SystemExit(
            f"ERROR: unknown visual page: {args.page}"
        )
    allowed_routes = {page_route(args.page)}

recipes = json.loads(MANIFEST.read_text())

if args.image:
    recipe = recipes.get(args.image)
    if recipe is None:
        raise SystemExit(f"ERROR: unknown documentation image: {args.image}")
    if recipe.get("kind") != "browser":
        raise SystemExit(f"ERROR: documentation image is not browser-managed: {args.image}")

browser_images = {
    filename
    for filename, recipe in recipes.items()
    if recipe.get("kind") == "browser"
}

if args.image:
    selected_browser_images = {args.image}
else:
    selected_browser_images = {
        filename
        for filename in browser_images
        if (
            allowed_routes is None
            or recipes[filename].get("route") in allowed_routes
        )
    }

if allowed_routes is not None and not selected_browser_images:
    raise SystemExit(
        "ERROR: no documentation images match the selected visual scope"
    )

static_images = {
    filename
    for filename, recipe in recipes.items()
    if recipe.get("kind") == "static"
}

unknown = {
    filename: recipe.get("kind")
    for filename, recipe in recipes.items()
    if recipe.get("kind") not in {"browser", "static"}
}

if unknown:
    details = ", ".join(
        f"{filename}={kind!r}"
        for filename, kind in sorted(unknown.items())
    )
    raise SystemExit(
        "ERROR: unsupported documentation image kinds: " + details
    )

if not STAGING.is_dir():
    raise SystemExit(
        f"ERROR: documentation staging directory does not exist: {STAGING}"
    )

staged_images = {
    str(path.relative_to(STAGING))
    for path in STAGING.rglob("*")
    if path.is_file()
}

if args.image:
    missing = sorted(selected_browser_images - staged_images)
    extra = []
elif allowed_routes is None:
    missing = sorted(browser_images - staged_images)
    extra = sorted(staged_images - browser_images)
else:
    missing = sorted(selected_browser_images - staged_images)
    extra = []

if missing or extra:
    messages = [
        "ERROR: staged documentation screenshots do not exactly match "
        "the browser-managed manifest."
    ]

    if missing:
        messages.append("Missing staged images:")
        messages.extend(f"  {name}" for name in missing)

    if extra:
        messages.append("Unexpected staged images:")
        messages.extend(f"  {name}" for name in extra)

    raise SystemExit("\n".join(messages))

for filename in sorted(selected_browser_images):
    source = STAGING / filename
    destination = DESTINATION / filename

    destination.parent.mkdir(
        parents=True,
        exist_ok=True,
    )

    shutil.copy2(source, destination)
    print(f"DOCS promote {filename}")

print()
print(f"Browser images promoted: {len(selected_browser_images)}")
print(f"Static images preserved: {len(static_images)}")
print(f"Managed documentation images: {len(recipes)}")
