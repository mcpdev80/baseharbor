#!/usr/bin/env python3
"""Fail closed when public English pages lack corresponding German pages."""
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1]
docs = root / "docs"
excluded = ("brand/", "stylesheets/", "internal/", "de/")
english = {
    p.relative_to(docs).as_posix()
    for p in docs.rglob("*.md")
    if not p.relative_to(docs).as_posix().startswith(excluded)
}
german = {
    p.relative_to(docs / "de").as_posix()
    for p in (docs / "de").rglob("*.md")
}
missing_de = sorted(english - german)
missing_en = sorted(german - english)
errors = []
if missing_de:
    errors.append("EN pages missing DE counterparts:\n" + "\n".join(missing_de))
if missing_en:
    errors.append("DE pages missing EN counterparts:\n" + "\n".join(missing_en))
config = (root / "mkdocs.de.yml").read_text(encoding="utf-8")
for line in config.splitlines():
    if "https://mcpdev80.github.io/baseharbor/" in line and "(EN)" in line:
        errors.append("German nav points to English-only documentation: " + line.strip())
if errors:
    print("\n\n".join(errors), file=sys.stderr)
    sys.exit(1)
print(f"EN/DE page coverage OK: {len(english)} matched Markdown pages")
