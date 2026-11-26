#!/usr/bin/env python3
"""0428-321 contract: the documented CLI exit-code set matches behavior.

The stable documented set is exactly {0, 1, 2, 130, 143} plus the child-status
passthrough range 1-255 (what internal/cli.WithExitCode can carry and what
ExitCode returns). Positive assertions require the documented signal codes
(130 SIGINT, 143 SIGTERM) in cli.md. A negative assertion fails if a page names
a *specific* undocumented code (3-129, 131-142, 144-255) as a CLI exit code,
while a source-line anchor such as main.go:61-63 is never mistaken for one.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# Standalone integer inside a code span, excluding source-line anchors
# (path:N-M) that merely point at the implementation.
CODE_SPAN = re.compile(r"(?<![\w:-])`(\d{1,3})`(?![\w:-])")
UNDOCUMENTED = {c for c in range(3, 130)} | {c for c in range(131, 143)} | {c for c in range(144, 256)}


def pages():
    files = [ROOT / "docs/reference/cli.md", ROOT / "docs/reference/errors.md"]
    present = [f for f in files if f.is_file()]
    missing = [f for f in files if not f.is_file()]
    if missing:
        print(f"exit-code taxonomy audit: missing required page: {missing[0].relative_to(ROOT)}")
        sys.exit(1)
    return present


def audit():
    cli, errors = pages()
    cli_text = cli.read_text(encoding="utf-8")
    text = cli_text + errors.read_text(encoding="utf-8")

    # Signal codes must be documented where the process-exit section lives.
    for code in ("130", "143"):
        if code not in cli_text:
            print(f"exit-code taxonomy audit: cli.md must document signal exit code {code}")
            sys.exit(1)

    documented = {int(m.group(1)) for m in CODE_SPAN.finditer(text)}
    undocumented = sorted(undocumented for undocumented in sorted(documented) if undocumented in UNDOCUMENTED)
    if undocumented:
        print(f"exit-code taxonomy audit: undocumented exit code(s) documented as CLI behavior: {undocumented} (0428-321)")
        sys.exit(1)

    print("exit-code taxonomy audit: documented set matches the stable contract (0428-321)")


if __name__ == "__main__":
    audit()
