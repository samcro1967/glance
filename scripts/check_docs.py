#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"
WIDGETS = DOCS / "widgets"
EXAMPLES = DOCS / "examples"
EXAMPLE_INDEX = DOCS / "examples.md"
CONFIGURATION = DOCS / "configuration.md"
WIDGET_INDEX = DOCS / "widgets.md"

EXPECTED_WIDGET_DOCS = {
    "analog-clock.md",
    "bookmarks.md",
    "calculator.md",
    "calendar-legacy.md",
    "calendar.md",
    "change-detection.md",
    "clock.md",
    "custom-api.md",
    "dns-stats.md",
    "docker-containers.md",
    "extension.md",
    "group.md",
    "hacker-news.md",
    "html.md",
    "ics-events.md",
    "iframe.md",
    "lobsters.md",
    "markdown.md",
    "markets.md",
    "monitor.md",
    "reddit.md",
    "releases.md",
    "repository.md",
    "rss.md",
    "search.md",
    "server-stats.md",
    "split-column.md",
    "stack.md",
    "status-bar.md",
    "timer.md",
    "todo.md",
    "twitch-channels.md",
    "twitch-top-games.md",
    "unit-converter.md",
    "videos.md",
    "weather.md",
}

LINK_RE = re.compile(r"!?(?:\[[^]]*\])\(([^)]+)\)")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*$")
TOP_NAV = "[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)"
SHARED_PROPERTIES = "[shared widget properties](../widgets.md#shared-properties)"
BACK_TO_TOP = "[Back to top]"


def markdown_files() -> list[Path]:
    return [
        CONFIGURATION,
        WIDGET_INDEX,
        EXAMPLE_INDEX,
        *sorted(WIDGETS.glob("*.md")),
        *sorted(EXAMPLES.rglob("*.md")),
    ]


def strip_fenced_code(text: str) -> str:
    lines: list[str] = []
    fenced = False

    for line in text.splitlines():
        if line.lstrip().startswith("```"):
            fenced = not fenced
            continue

        if not fenced:
            lines.append(line)

    return "\n".join(lines)


def anchor_for_heading(heading: str) -> str:
    value = heading.strip().lower()
    value = re.sub(r"<[^>]+>", "", value)
    value = re.sub(r"[*_`~]", "", value)
    value = re.sub(r"[^a-z0-9 _-]", "", value)
    value = value.replace(" ", "-")
    return value.strip("-")


def anchors(path: Path) -> set[str]:
    clean = strip_fenced_code(path.read_text(encoding="utf-8"))
    result: set[str] = set()

    for line in clean.splitlines():
        match = HEADING_RE.match(line)
        if match:
            result.add(anchor_for_heading(match.group(2)))

    return result


def fail(errors: list[str], message: str) -> None:
    errors.append(message)


def validate_widget_catalog(errors: list[str]) -> None:
    actual = {path.name for path in WIDGETS.glob("*.md")}

    for name in sorted(EXPECTED_WIDGET_DOCS - actual):
        fail(errors, f"missing widget document: docs/widgets/{name}")

    for name in sorted(actual - EXPECTED_WIDGET_DOCS):
        fail(errors, f"unexpected widget document: docs/widgets/{name}")

    index_text = WIDGET_INDEX.read_text(encoding="utf-8")

    for name in sorted(EXPECTED_WIDGET_DOCS):
        if f"(widgets/{name})" not in index_text:
            fail(errors, f"widgets.md does not link to widgets/{name}")


def validate_widget_pages(errors: list[str]) -> None:
    for path in sorted(WIDGETS.glob("*.md")):
        text = path.read_text(encoding="utf-8")
        clean = strip_fenced_code(text)

        h1 = [
            line
            for line in clean.splitlines()
            if line.startswith("# ") and not line.startswith("## ")
        ]

        if len(h1) != 1:
            fail(errors, f"{path.relative_to(ROOT)} has {len(h1)} top-level headings")

        if TOP_NAV not in text:
            fail(errors, f"{path.relative_to(ROOT)} is missing top navigation")

        if BACK_TO_TOP not in text:
            fail(errors, f"{path.relative_to(ROOT)} is missing bottom navigation")

        if SHARED_PROPERTIES not in text:
            fail(errors, f"{path.relative_to(ROOT)} is missing shared-properties link")
        if "## Quick start" not in clean:
            fail(errors, f"{path.relative_to(ROOT)} is missing Quick start section")

        if not re.search(r"^```ya?ml[ 	]*$", text, re.MULTILINE):
            fail(errors, f"{path.relative_to(ROOT)} is missing YAML example")

        if path.name not in {"calculator.md", "unit-converter.md"}:
            if "## Configuration" not in clean:
                fail(errors, f"{path.relative_to(ROOT)} is missing Configuration section")


def validate_local_links(errors: list[str]) -> None:
    anchor_cache: dict[Path, set[str]] = {}

    for path in markdown_files():
        text = path.read_text(encoding="utf-8")

        for raw_target in LINK_RE.findall(text):
            target = raw_target.strip()

            if not target:
                continue

            if target.startswith(("http://", "https://", "mailto:")):
                continue

            if target.startswith("#"):
                destination = path
                fragment = target[1:]
            else:
                location, separator, fragment = target.partition("#")
                destination = (path.parent / location).resolve()
                fragment = fragment if separator else ""

                if not destination.exists():
                    fail(
                        errors,
                        f"{path.relative_to(ROOT)} has missing local target: {target}",
                    )
                    continue

            if fragment and destination.suffix.lower() == ".md":
                if destination not in anchor_cache:
                    anchor_cache[destination] = anchors(destination)

                if fragment not in anchor_cache[destination]:
                    fail(
                        errors,
                        f"{path.relative_to(ROOT)} links to missing anchor "
                        f"#{fragment} in {destination.relative_to(ROOT)}",
                    )


def validate_images(errors: list[str]) -> None:
    for path in markdown_files():
        if "![](" in path.read_text(encoding="utf-8"):
            fail(errors, f"{path.relative_to(ROOT)} contains image with empty alt text")


def validate_configuration_split(errors: list[str]) -> None:
    text = CONFIGURATION.read_text(encoding="utf-8")

    if "(widgets.md)" not in text:
        fail(errors, "configuration.md does not link to widgets.md")


def main() -> int:
    errors: list[str] = []

    for path in [CONFIGURATION, WIDGET_INDEX, WIDGETS]:
        if not path.exists():
            fail(errors, f"required documentation path is missing: {path.relative_to(ROOT)}")

    if not errors:
        validate_widget_catalog(errors)
        validate_widget_pages(errors)
        validate_local_links(errors)
        validate_images(errors)
        validate_configuration_split(errors)

    if errors:
        print("Documentation validation failed:")
        for error in errors:
            print(f"  - {error}")
        return 1

    print("Documentation validation passed.")
    print(f"Widget documents: {len(EXPECTED_WIDGET_DOCS)}")
    print("Widget navigation: complete")
    print("Shared-property references: complete")
    print("Local Markdown links and anchors: valid")
    print("Image alt text: valid")
    print("Configuration split: valid")
    return 0


if __name__ == "__main__":
    sys.exit(main())
