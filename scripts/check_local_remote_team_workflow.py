#!/usr/bin/env python3
"""Keep the local, remote and team guide aligned with shipped CLI commands."""
from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]
COMMAND_INDEX = ROOT / "docs/cli/command-index.md"
GUIDES = {
    "English": ROOT / "docs/how-to/local-remote-team-workflow.md",
    "German": ROOT / "docs/de/how-to/local-remote-team-workflow.md",
}
SUPPORTED = (
    "baha init", "baha plan", "baha up", "baha status", "baha onboard",
    "baha use", "baha connections", "baha target list", "baha target show",
    "baha workstation new", "baha workstation revoke", "baha access assign",
    "baha whoami", "baha user offboard", "baha app secure-profile export",
    "baha app backup", "baha app restore", "baha app version", "baha app versions",
    "baha app workspace resolve", "baha app update",
)
UNSUPPORTED = ("baha app secure-profile import", "baha promote")
CLAIMS = {
    "English": (
        "management connection", "native image store", "OCI-compatible registry",
        "A missing Git revision stays unknown", "intended/applied/observed values",
        "migration tool", "Backup does not prove", "There is no `baha app secure-profile import` command.",
        "deferred to v0.4.28", "baha app versions --all-targets",
        "baha app workspace resolve -o json", "baha access assign PERSON",
    ),
    "German": (
        "Management-Verbindung", "nativen Image-Store", "OCI-kompatible Registry",
        "Eine fehlende Git-Revision bleibt unbekannt", "gewünschte, angewendete und beobachtete Werte",
        "Migrationstool", "Ein Backup beweist nicht", "Es gibt keinen Befehl `baha app secure-profile import`.",
        "auf v0.4.28 verschoben", "baha app versions --all-targets",
        "baha app workspace resolve -o json", "baha access assign PERSON",
    ),
}


def fail(message: str) -> None:
    print("local/remote/team workflow audit: " + message, file=sys.stderr)
    raise SystemExit(1)


index = COMMAND_INDEX.read_text()
for command in SUPPORTED:
    if f"| `{command}` |" not in index:
        fail(f"supported command is absent from the generated CLI index: {command}")
for command in UNSUPPORTED:
    if f"| `{command}` |" in index:
        fail(f"unsupported v0.4.27 workflow became a shipped command: {command}")

for language, path in GUIDES.items():
    text = path.read_text()
    for command in SUPPORTED:
        if command not in text:
            fail(f"{language} guide omits supported command {command}")
    for command in UNSUPPORTED:
        if command not in text:
            fail(f"{language} guide omits unsupported-command boundary {command}")
    for claim in CLAIMS[language]:
        if claim not in text:
            fail(f"{language} guide omits required boundary or entry point {claim!r}")

print(f"Local/remote/team workflow PASS ({len(SUPPORTED)} shipped commands; EN/DE positive and negative boundaries)")
