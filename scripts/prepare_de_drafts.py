#!/usr/bin/env python3
"""Generate reviewable EN->DE Markdown translation drafts, preserving code and links.

Optional authoring helper, never used as a release gate. Requires transformers<5,
torch, sentencepiece and an externally downloaded translation model.
"""
from __future__ import annotations
import argparse
import json
from pathlib import Path
import re
import sys

PROTECTED = re.compile(r"(`[^`\n]*`|\[[^\]]+\]\([^\n)]+\)|https?://[^\s)]+|<[^>\n]+>)")
PREFIX = re.compile(r"^(\s*(?:#{1,6}\s+|[-*+]\s+|\d+\.\s+|>\s*)?)(.*)$")
BULLET = re.compile(r"^[\s|:\-]+$")
ENGLISH = re.compile(r"[A-Za-z]{3,}")


def missing_pages(root: Path) -> list[Path]:
    docs = root / "docs"
    result = []
    for p in docs.rglob("*.md"):
        rel = p.relative_to(docs)
        if rel.parts[0] in ("de", "internal", "brand", "stylesheets"):
            continue
        if not (docs / "de" / rel).is_file():
            result.append(p)
    return sorted(result, key=lambda file: (file.stat().st_size, str(file)))


def page_segments(source: str) -> tuple[list[str], list[str]]:
    """Yield placeholders for natural-language pieces, never translate fenced code."""
    pieces: list[str] = []
    items: list[str] = []
    fence = False
    generated = False
    for raw in source.splitlines(keepends=True):
        stripped = raw.lstrip()
        if stripped.startswith("<!-- BEGIN GENERATED"):
            generated = True
        if stripped.startswith("```") or stripped.startswith("~~~"):
            fence = not fence
            pieces.append(raw)
            continue
        if fence or generated or stripped.startswith(("<!--", "<!---", "    ```")) or BULLET.fullmatch(stripped.rstrip("\n")):
            pieces.append(raw)
        else:
            match = PREFIX.match(raw.rstrip("\r\n"))
            assert match
            prefix, line = match.groups()
            if line.startswith(("|", "!", "{", "import ", "export ")) and not line.startswith("| "):
                pieces.append(raw)
                continue
            parts = PROTECTED.split(line)
            fragments = []
            for part in parts:
                if part and not PROTECTED.fullmatch(part) and ENGLISH.search(part):
                    if part.strip().startswith(("http", "www.", "git ", "go ", "npm ")):
                        fragments.append(part)
                    else:
                        key = "@@BHSEG" + str(len(items)) + "@@"
                        items.append(part)
                        fragments.append(key)
                else:
                    fragments.append(part)
            pieces.append(prefix + "".join(fragments) + raw[len(raw.rstrip("\r\n")):])
        if stripped.startswith("<!-- END GENERATED"):
            generated = False
    return pieces, items


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path("."))
    parser.add_argument("--offset", type=int, default=0)
    parser.add_argument("--limit", type=int, default=8)
    args = parser.parse_args()
    files = missing_pages(args.root)[args.offset:args.offset + args.limit]
    if not files:
        print("BH_TRANSLATION_EMPTY", flush=True)
        return
    from transformers import pipeline
    translator = pipeline("translation", model="Helsinki-NLP/opus-mt-en-de", device=-1)
    for file in files:
        pieces, items = page_segments(file.read_text(encoding="utf-8"))
        translations = []
        for chunk in range(0, len(items), 16):
            batch = items[chunk:chunk + 16]
            results = translator(batch, max_length=512, batch_size=8)
            translations.extend([r["translation_text"] for r in results])
        translated = "".join(pieces)
        for idx, value in enumerate(translations):
            translated = translated.replace("@@BHSEG" + str(idx) + "@@", value)
        rel = file.relative_to(args.root / "docs").as_posix()
        print("BH_DE_FILE\t" + json.dumps({"path": "docs/de/" + rel, "content": translated}, ensure_ascii=False), flush=True)
        print("BH_DE_PROGRESS\t" + rel + "\t" + str(len(items)), file=sys.stderr, flush=True)


if __name__ == "__main__":
    main()
