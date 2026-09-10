#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
CSS_ROOT = ROOT / "internal/glance/static/css"
JS_ROOT = ROOT / "internal/glance/static/js"

# CSS properties for which the global theme exposes an applicable semantic
# capability. A match is a review candidate, not necessarily a violation:
# specialized visualizations and component geometry can legitimately own
# concrete values.
THEME_CAPABLE_PROPERTIES = {
    "background",
    "background-color",
    "background-image",
    "backdrop-filter",
    "-webkit-backdrop-filter",
    "border",
    "border-color",
    "border-top",
    "border-right",
    "border-bottom",
    "border-left",
    "border-top-color",
    "border-right-color",
    "border-bottom-color",
    "border-left-color",
    "box-shadow",
    "color",
    "fill",
    "outline",
    "outline-color",
    "stroke",
}

DERIVED_KEYWORDS = {
    "currentcolor",
    "inherit",
    "initial",
    "unset",
    "revert",
    "revert-layer",
    "transparent",
}

COMMENT_RE = re.compile(r"/\*.*?\*/", re.DOTALL)
RULE_RE = re.compile(r"([^{}]+)\{([^{}]*)\}", re.DOTALL)
DECLARATION_RE = re.compile(r"(?m)^[ \t]*([A-Za-z-]+)[ \t]*:[ \t]*([^;]+);?")


@dataclass(frozen=True)
class Finding:
    path: Path
    line: int
    selector: str
    property: str
    value: str


def css_files() -> list[Path]:
    return [
        path
        for path in sorted(CSS_ROOT.glob("*.css"))
        if "vendor" not in path.parts
    ]


def classify(property_name: str, value: str) -> str:
    lowered = value.lower().strip()
    compact = "".join(lowered.split())

    if "var(--theme-" in compact:
        return "THEMED"

    if "var(--" in compact:
        return "DERIVED"

    if lowered in DERIVED_KEYWORDS:
        return "DERIVED"

    reset_properties = {
        "background",
        "background-color",
        "background-image",
        "box-shadow",
        "backdrop-filter",
        "-webkit-backdrop-filter",
        "outline",
        "outline-color",
    }

    if lowered in {"0", "none"}:
        if property_name.startswith("border") or property_name in reset_properties:
            return "STRUCTURAL"

    if property_name.startswith("border") and lowered.endswith(" transparent"):
        return "STRUCTURAL"

    return "REVIEW"


def audit_theme() -> int:
    paths = css_files()
    counts = {"THEMED": 0, "DERIVED": 0, "STRUCTURAL": 0, "REVIEW": 0}
    findings: list[Finding] = []
    declaration_count = 0

    for path in paths:
        try:
            original = path.read_text(encoding="utf-8")
        except OSError as error:
            print(f"ERROR: unable to read {path.relative_to(ROOT)}: {error}", file=sys.stderr)
            return 1

        # Preserve character positions so line numbers remain tied to source.
        text = COMMENT_RE.sub(lambda match: " " * len(match.group(0)), original)

        for rule in RULE_RE.finditer(text):
            selector = " ".join(rule.group(1).split())
            body = rule.group(2)
            body_offset = rule.start(2)

            for declaration in DECLARATION_RE.finditer(body):
                property_name = declaration.group(1).lower()
                if property_name not in THEME_CAPABLE_PROPERTIES:
                    continue

                declaration_count += 1
                value = " ".join(declaration.group(2).strip().split())
                classification = classify(property_name, value)
                counts[classification] += 1

                if classification == "REVIEW":
                    offset = body_offset + declaration.start(1)
                    line = original.count("\n", 0, offset) + 1
                    findings.append(
                        Finding(
                            path=path.relative_to(ROOT),
                            line=line,
                            selector=selector,
                            property=property_name,
                            value=value,
                        )
                    )

    print("=== FRONTEND THEME AUDIT ===")
    print(f"CSS files:                   {len(paths)}")
    print(f"Theme-capable declarations: {declaration_count}")
    print(f"Theme-connected:             {counts['THEMED']}")
    print(f"Derived/inherited:           {counts['DERIVED']}")
    print(f"Structural/resets:           {counts['STRUCTURAL']}")
    print(f"Review candidates:           {counts['REVIEW']}")

    if findings:
        print()
        print("REVIEW CANDIDATES")
        print("-----------------")
        for finding in sorted(findings, key=lambda item: (str(item.path), item.line, item.property)):
            print(f"{finding.path}:{finding.line}")
            print(f"  selector: {finding.selector}")
            print(f"  property: {finding.property}")
            print(f"  value:    {finding.value}")

    print()
    print("Theme audit completed. REVIEW findings are informational.")
    return 0


def javascript_files() -> list[Path]:
    return [
        path
        for path in sorted(JS_ROOT.glob("*.js"))
        if "vendor" not in path.parts
    ]


def diagnostic_block(lines: list[str], index: int) -> str:
    start = index
    depth = 0
    found_block = False

    while start >= 0:
        line = lines[start]
        depth += line.count("}") - line.count("{")
        if "catch" in line or depth < 0:
            found_block = True
            break
        start -= 1

    if not found_block:
        start = max(0, index - 8)

    end = index
    depth = 0
    while end < len(lines):
        line = lines[end]
        depth += line.count("{") - line.count("}")
        if end > index and depth < 0:
            break
        end += 1

    end = min(len(lines), max(end + 1, index + 9))
    return "\n".join(lines[start:end])


def audit_diagnostics() -> int:
    paths = javascript_files()
    counts = {"COMPLIANT": 0, "STRUCTURAL": 0, "REVIEW": 0}
    findings: list[tuple[Path, int, str, str]] = []

    for path in paths:
        try:
            text = path.read_text(encoding="utf-8")
        except OSError as error:
            print(f"ERROR: unable to read {path.relative_to(ROOT)}: {error}", file=sys.stderr)
            return 1

        lines = text.splitlines()

        for index, line in enumerate(lines):
            if "console.error" not in line and "console.warn" not in line:
                continue

            block = diagnostic_block(lines, index)
            if "frontendDiagnostic(" in block or "frontendDiagnosticError(" in block:
                classification = "COMPLIANT"
            else:
                classification = "REVIEW"

            counts[classification] += 1
            if classification == "REVIEW":
                findings.append(
                    (
                        path.relative_to(ROOT),
                        index + 1,
                        line.strip(),
                        "console diagnostic has no proven shared diagnostic in its local failure block",
                    )
                )

        if path.name == "diagnostics.js":
            for index, line in enumerate(lines):
                if ".catch(() => {" not in line:
                    continue

                block = "\n".join(lines[index:min(len(lines), index + 5)])
                if "Diagnostics must never interfere with normal page behavior." in block:
                    counts["STRUCTURAL"] += 1
                else:
                    counts["REVIEW"] += 1
                    findings.append(
                        (
                            path.relative_to(ROOT),
                            index + 1,
                            line.strip(),
                            "promise catch is not documented as diagnostics transport isolation",
                        )
                    )

    print("=== FRONTEND DIAGNOSTICS AUDIT ===")
    print(f"JavaScript files:             {len(paths)}")
    print("Diagnostic-compliant:         {}".format(counts["COMPLIANT"]))
    print("Structural/isolation:         {}".format(counts["STRUCTURAL"]))
    print("Review candidates:            {}".format(counts["REVIEW"]))

    if findings:
        print()
        print("REVIEW CANDIDATES")
        print("-----------------")
        for path, line, source, reason in sorted(findings):
            print(f"{path}:{line}")
            print(f"  source: {source}")
            print(f"  reason: {reason}")

    print()
    print("Diagnostics audit completed. REVIEW findings are informational.")
    return 0


def main() -> int:
    if not CSS_ROOT.is_dir():
        print(f"ERROR: CSS source directory is missing: {CSS_ROOT.relative_to(ROOT)}", file=sys.stderr)
        return 1

    if not JS_ROOT.is_dir():
        print(f"ERROR: JavaScript source directory is missing: {JS_ROOT.relative_to(ROOT)}", file=sys.stderr)
        return 1

    theme_status = audit_theme()
    if theme_status != 0:
        return theme_status

    print()
    return audit_diagnostics()



if __name__ == "__main__":
    sys.exit(main())
