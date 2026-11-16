#!/usr/bin/env python3
"""Verify code fences and API/CLI literals survive EN->DE Markdown translation."""
from __future__ import annotations
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1] / "docs"
fenced = re.compile(r"(?ms)^([ 	]*)(`{3,}|~{3,})[^\n]*\n(.*?)^\1\2[ 	]*$", re.MULTILINE)
inline = re.compile(r"(?<!`)`([^`\n]+)`(?!`)")


def find_fences(value: str) -> list[str]:
    return [m.group(3) for m in fenced.finditer(value)]


def main() -> int:
    errors: list[str] = []
    checked = 0
    for english in root.rglob("*.md"):
        relative = english.relative_to(root)
        if relative.parts[0] in {"de", "internal"}:
            continue
        german = root / "de" / relative
        if not german.is_file():
            continue  # Existing page-coverage gate reports this separately.
        checked += 1
        en = english.read_text(encoding="utf-8")
        de = german.read_text(encoding="utf-8")
        a, b = find_fences(en), find_fences(de)
        if a != b:
            errors.append(f"{relative}: fenced code/example contents differ ({len(a)} EN vs {len(b)} DE)")
        if en.count("```") != de.count("```"):
            errors.append(f"{relative}: unbalanced/changed Markdown code fences")
        for token in sorted(set(inline.findall(en))):
            if token not in de and not token.startswith(("https://", "http://")):
                errors.append(f"{relative}: literal lost: {token[:100]}")
    if errors:
        for error in errors[:100]:
            print(error, file=sys.stderr)
        print(f"DE technical integrity: FAIL ({len(errors)} findings across {checked} paired pages)", file=sys.stderr)
        return 1
    print(f"DE technical integrity: PASS ({checked} paired pages)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
