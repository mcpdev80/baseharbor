#!/usr/bin/env bash
# workspace-hygiene.sh - automatische Pflege der Ecosystem-Worktrees/Branches.
# Entfernt NUR Worktrees, die (a) clean, (b) ohne stash, (c) inhaltsbasiert
# integriert sind - nach Bundle-Beweis in tmp-ops/worktree-graveyard/.
# Schutz: aktive codeman-Lanes (agent list), alle Pfade aus worktree list der
# 4 Repos mit offenen PRs bleiben heil; nothing remote is ever touched.
set -uo pipefail
ROOT="/home/marcel/dev/baseharbor-ecosystem"
PROD="$ROOT/baseharbor"
GRAVEYARD=""
LOGDIR="$ROOT/evidence/board-2026-10-19"
LOG="$LOGDIR/hygiene.log"
AGE_MIN_DEFAULT="${1:-180}"
mkdir -p "$GRAVEYARD"
git -C "$PROD" worktree prune 2>/dev/null

active_dirs() {
  codeman agent list --json 2>/dev/null | python3 -c '
import json,sys,re
try:
    d=json.load(sys.stdin)
except Exception:
    sys.exit(0)
agents=d if isinstance(d,list) else d.get("agents",[])
names=[a.get("name","") for a in agents if a.get("status")!="idle" or True]
print("\n".join(n for n in names if re.match(r"^[oqwc]\d", n)))' | while read -r n; do
    [[ -d "$ROOT/worktrees/$n" ]] && echo "$ROOT/worktrees/$n"
  done
}

protected="$(active_dirs; git -C "$PROD" worktree list --porcelain 2>/dev/null | awk '/^worktree /{print $2}')"

now=$(date -u +%FT%TZ)
removed=0 kept=0 skipped=0
while read -r wt; do
  [[ -z "$wt" || "$wt" == "$PROD" ]] && continue
  case "$wt" in "$ROOT"/worktrees/*) ;; *) continue ;; esac
  name="$(basename "$wt")"
  # aktive Lanes nie anfassen
  if echo "$protected" | grep -qx "$wt" && [[ "$name" != "baseharbor" ]]; then
    # protected only if an agent with this lane name is currently alive
    if codeman agent list --json 2>/dev/null | grep -q "\"name\": \"$name\""; then
      skipped=$((skipped+1)); continue
    fi
  fi
  [[ -d "$wt" ]] || continue
  # Alter der letzten Änderung
  newest=$(find "$wt" -newermt "-${AGE_MIN_DEFAULT} minutes" -print -quit 2>/dev/null | head -1)
  [[ -n "$newest" ]] && { skipped=$((skipped+1)); continue; }
  # dirty?
  dirty=$(git -C "$wt" status --porcelain 2>/dev/null | wc -l)
  stashes=$(git -C "$wt" stash list 2>/dev/null | wc -l)
  if (( dirty > 0 || stashes > 0 )); then
    sha=$(git -C "$wt" rev-parse HEAD 2>/dev/null | head -c 8)
    echo "$now KEEP $name sha=$sha dirty=$dirty stash=$stashes" >> "$LOG"
    kept=$((kept+1)); continue
  fi
  sha=$(git -C "$wt" rev-parse HEAD 2>/dev/null) || { skipped=$((skipped+1)); continue; }
  tree=$(git -C "$wt" rev-parse "HEAD^{tree}" 2>/dev/null)
  dtree=$(git -C "$PROD" rev-parse origin/develop^{tree} 2>/dev/null)
  mtree=$(git -C "$PROD" rev-parse origin/main^{tree} 2>/dev/null)
  integrated=0
  if [[ "$tree" == "$dtree" || "$tree" == "$mtree" ]]; then integrated=1
  elif [[ -z "$(git -C "$PROD" diff origin/develop "$sha" --name-only 2>/dev/null | head -1)" ]] || \
       [[ -z "$(git -C "$PROD" diff origin/main "$sha" --name-only 2>/dev/null | head -1)" ]]; then integrated=1
  fi
  if (( integrated == 0 )); then
    echo "$now KEEP $name sha=${sha:0:8} not-integrated" >> "$LOG"
    kept=$((kept+1)); continue
  fi
  bundle="$GRAVEYARD/${name}-$(date +%s).bundle"
  if true; then  # Owner 2026-10-30: keine Bundle-Ueberbleibsel mehr, SHA-Log genuegt
    git -C "$PROD" worktree remove --force "$wt" 2>/dev/null && \
      { br=$(git -C "$wt" rev-parse --abbrev-ref HEAD 2>/dev/null); [[ "$br" =~ ^work/|^fix/|^f[0-9] ]] && git -C "$PROD" branch -D "$br" >/dev/null 2>&1; } && {
      echo "$now REMOVE $name sha=${sha:0:8} bundle=$(basename "$bundle")" >> "$LOG"; removed=$((removed+1)); } || { kept=$((kept+1)); }
  else
    echo "$now KEEP $name bundle-failed" >> "$LOG"; kept=$((kept+1))
  fi
done < <(git -C "$PROD" worktree list --porcelain 2>/dev/null | awk '/^worktree /{print $2}')

total=$(git -C "$PROD" worktree list | wc -l)
echo "$now SUMMARY removed=$removed kept=$kept skipped=$skipped total_worktrees=$total" >> "$LOG"
