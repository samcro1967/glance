#!/usr/bin/env python3
"""Prepare an isolated production-derived configuration for test-prod."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


def parse_bool(value: str) -> bool:
    if value == "true":
        return True
    if value == "false":
        return False
    raise ValueError(f"expected true or false, got {value!r}")


def server_bounds(lines: list[str]) -> tuple[int, int]:
    indexes = [
        index
        for index, line in enumerate(lines)
        if line.rstrip() == "server:" and line == line.lstrip()
    ]
    if len(indexes) != 1:
        raise ValueError(
            f"expected exactly one top-level server mapping; found {len(indexes)}"
        )

    start = indexes[0]
    end = len(lines)
    for index in range(start + 1, len(lines)):
        line = lines[index]
        if line.strip() and line == line.lstrip():
            end = index
            break

    return start, end


def set_server_bool(lines: list[str], key: str, enabled: bool) -> None:
    server_index, server_end = server_bounds(lines)
    prefix = f"  {key}:"
    matches = [
        index
        for index in range(server_index + 1, server_end)
        if lines[index].startswith(prefix)
    ]
    if len(matches) > 1:
        raise ValueError(f"expected at most one server {key} setting")
    value = "true" if enabled else "false"
    if matches:
        lines[matches[0]] = f"  {key}: {value}"
    elif enabled:
        lines.insert(server_index + 1, f"  {key}: true")


def add_resource_proxy(lines: list[str], origins: list[str]) -> None:
    if not origins:
        return
    server_index, server_end = server_bounds(lines)
    if any(
        lines[index].startswith("  resource-proxy:")
        for index in range(server_index + 1, server_end)
    ):
        raise ValueError("production config already contains server resource-proxy")
    block = ["  resource-proxy:", "    allowed-origins:"]
    block.extend(f"      - {origin}" for origin in origins)
    lines[server_index + 1:server_index + 1] = block


def prepare(
    source: Path,
    destination: Path,
    overlay: Path | None,
    frontend_diagnostics: bool,
    https: bool,
    resource_proxy_origins: list[str],
) -> None:
    lines = source.read_text(encoding="utf-8").splitlines()
    server_bounds(lines)
    set_server_bool(lines, "https", https)
    set_server_bool(lines, "frontend-diagnostics", frontend_diagnostics)
    add_resource_proxy(lines, resource_proxy_origins)

    contents = "\n".join(lines) + "\n"
    if overlay is not None:
        overlay_contents = overlay.read_text(encoding="utf-8").strip()
        if overlay_contents:
            contents += "\n" + overlay_contents + "\n"

    temporary = destination.with_name(destination.name + ".tmp")
    temporary.write_text(contents, encoding="utf-8")
    temporary.replace(destination)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Prepare the isolated production-derived test-prod configuration."
    )
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--destination", type=Path, required=True)
    parser.add_argument("--overlay", type=Path)
    parser.add_argument("--frontend-diagnostics", default="true")
    parser.add_argument("--https", default="false")
    parser.add_argument(
        "--resource-proxy-origins",
        default="",
        help="Whitespace-separated resource-proxy origins.",
    )
    args = parser.parse_args()

    try:
        prepare(
            source=args.source,
            destination=args.destination,
            overlay=args.overlay,
            frontend_diagnostics=parse_bool(args.frontend_diagnostics),
            https=parse_bool(args.https),
            resource_proxy_origins=args.resource_proxy_origins.split(),
        )
    except (OSError, ValueError) as error:
        print(f"test-prod config preparation failed: {error}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
