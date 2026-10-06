#!/usr/bin/env python3
"""Bound and redact the original failure before outer runtime cleanup."""
import argparse
import hashlib
import json
import pathlib
import re


def summarize(log):
    findings = []
    for line in log.splitlines():
        if "[runtime-reset]" in line or "[acceptance] cleanup:" in line:
            continue
        if not re.search(r"--- FAIL:|\bFAILED\b|\bFAIL\s|Error:|AssertionError|preflight failed|timed out|panic:|\b[\w./-]+_test\.go:\d+:", line):
            continue
        line = re.sub(r"[a-z][a-z0-9+.-]*://[^\s/@]+@", "<scheme>://<redacted>@", line, flags=re.I)
        line = re.sub(r'''(?i)((?:password|secret|token|authorization|access[_-]?key|private[_-]?key)[\w-]*["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|\S+)''', r"\1<redacted>", line)
        line = re.sub(r"(?i)\b(?:bearer|basic)\s+\S+", "<redacted authorization>", line)
        line = re.sub(r"[A-Za-z0-9_+/=-]{48,}", "<redacted long value>", line)
        findings.append(line.replace("```", "'''")[:300])
        if len(findings) == 8:
            break
    text = "\n".join(findings)
    classification = "unclassified"
    for name, pattern in [("tls", r"tls|x509|certificate"), ("ownership", r"ownership|owned|foreign"), ("preflight", r"preflight"), ("timeout", r"timeout|timed out"), ("assertion", r"FAIL|AssertionError")]:
        if re.search(pattern, text, re.I):
            classification = name
            break
    tests = re.findall(r"--- FAIL:\s+(\w+)", text)
    key = hashlib.sha256((classification + "/" + "/".join(tests)).encode()).hexdigest()[:16]
    return {"classification": classification, "failure_key": key, "assertions": findings}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("log", type=pathlib.Path)
    parser.add_argument("output", type=pathlib.Path)
    args = parser.parse_args()
    log = args.log.read_text(errors="replace") if args.log.is_file() else ""
    result = summarize(log)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print("\n### Original failure (bounded, redacted)\n\n```text")
    print("\n".join(result["assertions"]) or "No assertion captured; see the failed setup step and attached diagnostics.")
    print("```\n")


if __name__ == "__main__":
    main()
