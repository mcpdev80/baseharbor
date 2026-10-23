#!/usr/bin/env bash
# handoff-to-codex.sh - durable worker-completion -> Codex-6.1 delegation helper
# (Runbook per AGENTS.md Ablauf step 7; Codeman has NO automatic handoff.
#  Source: o6-codeman-delegation-investigation.md, 2026-10-19.)
# Usage: handoff-to-codex.sh <worker-session-id> <codex-session-id> <evidence-file> <handoff-prompt-file>
set -euo pipefail
worker="${1:?}"; codex="${2:?}"; evidence="${3:?}"; promptfile="${4:?}"

# 1) wait for worker completion (60s poll budget per AGENTS.md; exits 0 on idle/exit)
until codeman agent wait "$worker" --until idle,exit --timeout 60000 >/dev/null 2>&1; do
  st=$(codeman agent list --json 2>/dev/null | python3 -c "
import json,sys
print(next((s['status'] for s in json.load(sys.stdin) if s['id'].startswith('${worker:0:8}')),'gone'))")
  [ "$st" = "idle" ] && break
  [ "$st" = "gone" ] && { echo "worker session gone"; exit 3; }
done

# 2) verify evidence independently (idle alone proves nothing)
[ -s "$evidence" ] || { echo "evidence missing/empty: $evidence"; exit 4; }

# 3) deliver the acceptance prompt to the SAME reviewer session
codeman agent send "$codex" "$(cat "$promptfile")"
echo "handoff delivered to $codex at $(date -u +%FT%TZ)"
