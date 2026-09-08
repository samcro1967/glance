#!/usr/bin/env python3
# Path: testdata/visual/check-gallery.py
# File: check-gallery.py
"""Validate Glance visual-QA and documentation-image contracts.

Ownership: development-only visual QA infrastructure.

Responsibilities:
- prove every registered widget is classified exactly once;
- prove every registered widget has one canonical QA screenshot mapping;
- prove every configured visual page exists in glance-test.yml;
- validate browser and static documentation-image ownership;
- prove every local Markdown image is managed exactly once;
- report obsolete docs/images files during the screenshot migration.

Non-goals:
- browser capture;
- image comparison;
- runtime widget validation;
- mutation of repository state.
"""

# Standard library
import json
import re
from pathlib import Path


# -----------------------------------------------------------------------------
# Constants / configuration
# -----------------------------------------------------------------------------

ROOT = Path(__file__).resolve().parents[2]

REGISTRY = ROOT / "internal/glance/widget-capability-map.go"
GALLERY_MANIFEST = ROOT / "testdata/visual/widget-gallery.json"
PAGES_MANIFEST = ROOT / "testdata/visual/visual-pages.json"
DOCS_MANIFEST = ROOT / "testdata/visual/docs-images.json"
WIDGET_SCREENSHOTS = ROOT / "testdata/visual/widget-screenshots.json"

TEST_CONFIG = ROOT / "glance-test.yml"
README = ROOT / "README.md"
DOCS_ROOT = ROOT / "docs"
DOCS_IMAGE_DIR = DOCS_ROOT / "images"


def fail(message):
    raise SystemExit("ERROR: " + message)


# -----------------------------------------------------------------------------
# Widget registry / gallery contract
# -----------------------------------------------------------------------------

registry_text = REGISTRY.read_text()

match = re.search(
    r"var widgetRegistry = map\[string\]widgetDescriptor\{(?P<body>.*?)\n\}",
    registry_text,
    re.S,
)

if not match:
    fail("could not locate widgetRegistry")

registered = set(
    re.findall(
        r'^\s*"([a-z0-9-]+)":\s*\{',
        match.group("body"),
        re.M,
    )
)

gallery = json.loads(GALLERY_MANIFEST.read_text())

if not isinstance(gallery, dict):
    fail("widget-gallery.json must contain an object")

classified = [
    widget
    for widgets in gallery.values()
    for widget in widgets
]

duplicates = sorted(
    widget
    for widget in set(classified)
    if classified.count(widget) > 1
)

missing = sorted(registered - set(classified))
unknown = sorted(set(classified) - registered)

print("=== GALLERY REGISTRY CONTRACT ===")
print(f"Registered widgets: {len(registered)}")
print(f"Classified widgets: {len(set(classified))}")
print(f"Gallery categories: {len(gallery)}")

if duplicates or missing or unknown:
    if duplicates:
        print("Duplicate:", ", ".join(duplicates))
    if missing:
        print("Missing:", ", ".join(missing))
    if unknown:
        print("Unknown:", ", ".join(unknown))
    raise SystemExit(1)

print("Gallery registry coverage: COMPLETE")

for page, widgets in gallery.items():
    print(
        f"{page:24} {len(widgets):2}  "
        + ", ".join(widgets)
    )


# -----------------------------------------------------------------------------
# Visual page contract
# -----------------------------------------------------------------------------

config_text = TEST_CONFIG.read_text()

configured_slugs = set(
    re.findall(
        r"^\s*slug:\s*['\"]?([a-z0-9-]+)['\"]?\s*$",
        config_text,
        re.M,
    )
)

visual_pages = json.loads(PAGES_MANIFEST.read_text())

if not isinstance(visual_pages, dict):
    fail("visual-pages.json must contain an object")

expected_slugs = {
    slug
    for slugs in visual_pages.values()
    for slug in slugs
}

missing_pages = sorted(expected_slugs - configured_slugs)

if missing_pages:
    fail(
        "visual pages missing from glance-test.yml: "
        + ", ".join(missing_pages)
    )


# -----------------------------------------------------------------------------
# Canonical widget screenshot contract
# -----------------------------------------------------------------------------

widget_recipes = json.loads(WIDGET_SCREENSHOTS.read_text())

if not isinstance(widget_recipes, dict):
    fail("widget-screenshots.json must contain an object")

mapped_widgets = set(widget_recipes)

missing_widget_screenshots = sorted(
    registered - mapped_widgets
)

unknown_widget_screenshots = sorted(
    mapped_widgets - registered
)

invalid_widget_recipes = []

for widget, recipe in widget_recipes.items():
    if not isinstance(recipe, dict):
        invalid_widget_recipes.append(
            f"{widget}: recipe must be an object"
        )
        continue

    route = recipe.get("route")
    selector = recipe.get("selector")

    if (
        not isinstance(route, str)
        or not route.startswith("/")
    ):
        invalid_widget_recipes.append(
            f"{widget}: invalid route"
        )

    if (
        not isinstance(selector, str)
        or not selector.strip()
    ):
        invalid_widget_recipes.append(
            f"{widget}: missing selector"
        )

    if "nth" in recipe:
        nth = recipe["nth"]
        if (
            not isinstance(nth, int)
            or isinstance(nth, bool)
            or nth < 0
        ):
            invalid_widget_recipes.append(
                f"{widget}: nth must be a non-negative integer"
            )

print()
print("=== WIDGET SCREENSHOT CONTRACT ===")
print(f"Registered widgets: {len(registered)}")
print(f"Widget screenshot mappings: {len(mapped_widgets)}")

if (
    missing_widget_screenshots
    or unknown_widget_screenshots
    or invalid_widget_recipes
):
    if missing_widget_screenshots:
        print(
            "Missing:",
            ", ".join(missing_widget_screenshots),
        )

    if unknown_widget_screenshots:
        print(
            "Unknown:",
            ", ".join(unknown_widget_screenshots),
        )

    if invalid_widget_recipes:
        print("Invalid recipes:")
        for error in invalid_widget_recipes:
            print(f"  {error}")

    raise SystemExit(1)

print("Widget screenshot coverage: COMPLETE")


# -----------------------------------------------------------------------------
# Documentation image manifest contract
# -----------------------------------------------------------------------------

docs_images = json.loads(DOCS_MANIFEST.read_text())

if not isinstance(docs_images, dict):
    fail("docs-images.json must contain an object")

browser_images = set()
static_images = set()
invalid_docs_recipes = []

supported_extensions = {
    ".png",
    ".gif",
    ".jpg",
    ".jpeg",
    ".webp",
}

for filename, recipe in docs_images.items():
    suffix = Path(filename).suffix.lower()

    if suffix not in supported_extensions:
        invalid_docs_recipes.append(
            f"{filename}: unsupported image extension"
        )

    if not isinstance(recipe, dict):
        invalid_docs_recipes.append(
            f"{filename}: recipe must be an object"
        )
        continue

    kind = recipe.get("kind")

    if kind == "static":
        unexpected = sorted(set(recipe) - {"kind"})

        if unexpected:
            invalid_docs_recipes.append(
                f"{filename}: static image has browser fields: "
                + ", ".join(unexpected)
            )

        if not (DOCS_IMAGE_DIR / filename).is_file():
            invalid_docs_recipes.append(
                f"{filename}: static image does not exist"
            )

        static_images.add(filename)
        continue

    if kind != "browser":
        invalid_docs_recipes.append(
            f"{filename}: kind must be 'browser' or 'static'"
        )
        continue

    if suffix != ".png":
        invalid_docs_recipes.append(
            f"{filename}: browser-managed image must be PNG"
        )

    capture = recipe.get("capture")

    if capture not in {"element", "page"}:
        invalid_docs_recipes.append(
            f"{filename}: capture must be 'element' or 'page'"
        )

    route = recipe.get("route")

    if (
        not isinstance(route, str)
        or not route.startswith("/")
    ):
        invalid_docs_recipes.append(
            f"{filename}: invalid route"
        )

    if capture == "element":
        selector = recipe.get("selector")

        if (
            not isinstance(selector, str)
            or not selector.strip()
        ):
            invalid_docs_recipes.append(
                f"{filename}: element capture requires selector"
            )

        if "fullPage" in recipe:
            invalid_docs_recipes.append(
                f"{filename}: fullPage is only valid for page capture"
            )

    elif "selector" in recipe:
        invalid_docs_recipes.append(
            f"{filename}: page capture must not define selector"
        )

    if "nth" in recipe:
        nth = recipe["nth"]

        if capture != "element":
            invalid_docs_recipes.append(
                f"{filename}: nth is only valid for element capture"
            )
        elif (
            not isinstance(nth, int)
            or isinstance(nth, bool)
            or nth < 0
        ):
            invalid_docs_recipes.append(
                f"{filename}: nth must be a non-negative integer"
            )

    viewport = recipe.get("viewport")

    if viewport is not None:
        if not isinstance(viewport, dict):
            invalid_docs_recipes.append(
                f"{filename}: viewport must be an object"
            )
        elif set(viewport) != {"width", "height"}:
            invalid_docs_recipes.append(
                f"{filename}: viewport requires exactly width and height"
            )
        else:
            for key in ("width", "height"):
                value = viewport[key]

                if (
                    not isinstance(value, int)
                    or isinstance(value, bool)
                    or value <= 0
                ):
                    invalid_docs_recipes.append(
                        f"{filename}: viewport.{key} "
                        "must be a positive integer"
                    )

    if (
        "fullPage" in recipe
        and not isinstance(recipe["fullPage"], bool)
    ):
        invalid_docs_recipes.append(
            f"{filename}: fullPage must be boolean"
        )

    browser_images.add(filename)

if invalid_docs_recipes:
    raise SystemExit(
        "ERROR: invalid docs image recipes:\n  "
        + "\n  ".join(invalid_docs_recipes)
    )


# -----------------------------------------------------------------------------
# Markdown -> managed image ownership contract
# -----------------------------------------------------------------------------

markdown_image_re = re.compile(
    r'!\[[^\]]*\]\(([^)]+)\)'
)

referenced_images = set()
reference_count = 0

markdown_files = [
    README,
    *sorted(DOCS_ROOT.rglob("*.md")),
]

for markdown in markdown_files:
    if not markdown.is_file():
        continue

    lines = markdown.read_text(
        errors="replace"
    ).splitlines()

    for line_number, line in enumerate(lines, 1):
        for match in markdown_image_re.finditer(line):
            raw = match.group(1).strip()
            target = raw.split()[0].strip("<>")

            if target.startswith(
                ("http://", "https://", "data:")
            ):
                continue

            target = (
                target
                .split("#", 1)[0]
                .split("?", 1)[0]
            )

            resolved = (
                markdown.parent / target
            ).resolve()

            try:
                image_name = str(
                    resolved.relative_to(
                        DOCS_IMAGE_DIR.resolve()
                    )
                )
            except ValueError:
                fail(
                    "local Markdown image outside docs/images: "
                    f"{markdown.relative_to(ROOT)}:"
                    f"{line_number}: {raw}"
                )

            # Browser-managed images may legitimately be absent from
            # docs/images until a newly introduced capture has been staged,
            # reviewed, and promoted. Static assets are validated separately
            # by the manifest contract and must always exist.
            referenced_images.add(image_name)
            reference_count += 1


managed_images = browser_images | static_images

unmanaged_references = sorted(
    referenced_images - managed_images
)

unreferenced_managed = sorted(
    managed_images - referenced_images
)

overlap = sorted(
    browser_images & static_images
)

if unmanaged_references:
    fail(
        "referenced documentation images are not managed: "
        + ", ".join(unmanaged_references)
    )

if unreferenced_managed:
    fail(
        "managed documentation images are not referenced: "
        + ", ".join(unreferenced_managed)
    )

if overlap:
    fail(
        "documentation images cannot be both browser and static: "
        + ", ".join(overlap)
    )


# -----------------------------------------------------------------------------
# docs/images filesystem ownership
#
# The manifest is the complete ownership contract for documentation images.
# Every managed asset must exist and no unmanaged files may be present.
# -----------------------------------------------------------------------------

existing_images = {
    str(path.relative_to(DOCS_IMAGE_DIR))
    for path in DOCS_IMAGE_DIR.rglob("*")
    if path.is_file()
}

missing_images = sorted(
    managed_images - existing_images
)

unmanaged_images = sorted(
    existing_images - managed_images
)

if missing_images:
    fail(
        "managed documentation images are missing from docs/images: "
        + ", ".join(missing_images)
    )

if unmanaged_images:
    fail(
        "unmanaged files exist in docs/images: "
        + ", ".join(unmanaged_images)
    )


# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------

print()
print("=== DOCUMENTATION IMAGE CONTRACT ===")
print(
    f"Referenced documentation images: "
    f"{len(referenced_images)}"
)
print(
    f"Markdown image references: "
    f"{reference_count}"
)
print(
    f"Browser-managed images: "
    f"{len(browser_images)}"
)
print(
    f"Static-managed images: "
    f"{len(static_images)}"
)
print(
    f"Managed documentation images: "
    f"{len(managed_images)}"
)
print(
    f"Existing docs/images files: "
    f"{len(existing_images)}"
)
print(
    f"Missing docs/images files: "
    f"{len(missing_images)}"
)
print(
    f"Unmanaged docs/images files: "
    f"{len(unmanaged_images)}"
)
print("Documentation filesystem ownership: EXACT")

print()
print("=== VISUAL PAGE CONTRACT ===")
print(f"Configured visual pages: {len(expected_slugs)}")
print(
    f"Canonical widget screenshots: "
    f"{len(mapped_widgets)}"
)
print(
    f"Managed docs images: "
    f"{len(docs_images)}"
)
print(
    "Visual page, widget screenshot, and "
    "documentation mapping coverage: COMPLETE"
)
