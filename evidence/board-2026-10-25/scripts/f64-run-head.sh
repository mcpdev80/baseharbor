#!/bin/bash
# F64 head full-suite json runs (same env/config as baseline).
EV=/home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-25/f64-driftfix-data
WT=/home/marcel/dev/baseharbor-ecosystem/worktrees/f64-driftfix
export GOCACHE=/home/marcel/tmp-ops/f64-go-cache
export TMPDIR=/home/marcel/tmp-ops
rm -f "$EV/F64-HEAD-DONE" "$EV/head-cmd-exit" "$EV/head-int-exit"
cd "$WT" && go test ./cmd/baha/ -count=1 -json > "$EV/head-cmd.jsonl" 2> "$EV/head-cmd.stderr"
echo "exit=$? epoch=$(date +%s)" > "$EV/head-cmd-exit"
cd "$WT" && go test ./internal/application/ -count=1 -json > "$EV/head-int.jsonl" 2> "$EV/head-int.stderr"
echo "exit=$? epoch=$(date +%s)" > "$EV/head-int-exit"
echo done > "$EV/F64-HEAD-DONE"
