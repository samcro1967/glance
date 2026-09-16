#!/usr/bin/env python3
"""Validate that changed repository paths cannot affect the Glance runtime artifact.

Paths are read from standard input, one per line. The validator is deliberately
deny-by-default: every changed path must match an explicitly approved
non-runtime category.

This script classifies paths only. Git revision selection and diff generation
remain the responsibility of the caller.
"""

from __future__ import annotations

import sys
from pathlib import PurePosixPath


EXACT_NONRUNTIME_PATHS = frozenset(
    {
        ".gitignore",
        ".golangci.yml",
        "CODE_OF_CONDUCT.md",
        "CONTRIBUTING.md",
        "LICENSE",
        "Makefile",
        "README.md",
        "test-instance.yml",
    }
)

NONRUNTIME_PREFIXES = (
    ".github/",
    "docs/",
    "scripts/",
    "testdata/",
)


def is_nonruntime_path(path: str) -> bool:
    """Return whether a repository path is explicitly non-runtime."""
    path = path.strip()

    if not path:
        return False

    if path in EXACT_NONRUNTIME_PATHS:
        return True

    if path.startswith(NONRUNTIME_PREFIXES):
        return True

    return PurePosixPath(path).name.endswith("_test.go")


def main() -> int:
    paths = [line.strip() for line in sys.stdin if line.strip()]

    if not paths:
        print("No changed paths supplied.", file=sys.stderr)
        return 2

    rejected = [path for path in paths if not is_nonruntime_path(path)]

    if rejected:
        print("Runtime-impacting or unapproved paths changed:", file=sys.stderr)
        for path in rejected:
            print(path, file=sys.stderr)
        return 1

    print(f"Non-runtime scope validated: {len(paths)} changed path(s).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
