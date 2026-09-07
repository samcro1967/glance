#!/usr/bin/env python3

import argparse
import json
import subprocess
import sys


def run(*args: str) -> str:
    result = subprocess.run(
        args,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    return result.stdout.strip()


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Resolve exactly one open pull request for a head/base pair."
    )
    parser.add_argument("--repo", required=True)
    parser.add_argument("--head", required=True)
    parser.add_argument("--base", required=True)
    args = parser.parse_args()

    output = run(
        "gh",
        "pr",
        "list",
        "--repo",
        args.repo,
        "--state",
        "open",
        "--head",
        args.head,
        "--base",
        args.base,
        "--json",
        "number,headRefName,baseRefName",
    )

    prs = json.loads(output)

    if len(prs) == 0:
        print(
            f"No open pull request found for {args.head} -> {args.base}.",
            file=sys.stderr,
        )
        return 1

    if len(prs) != 1:
        numbers = ", ".join(f"#{pr['number']}" for pr in prs)
        print(
            f"Expected exactly one open pull request for "
            f"{args.head} -> {args.base}; found {len(prs)}: {numbers}.",
            file=sys.stderr,
        )
        return 1

    print(prs[0]["number"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
