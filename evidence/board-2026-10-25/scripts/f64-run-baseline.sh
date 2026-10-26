#!/bin/bash
# F64 baseline (2f9afafd) full-suite json runs, sequential (same config as head run).
EV=/home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-25/f64-driftfix-data
BASE=/home/marcel/tmp-ops/f64-base
export GOCACHE=/home/marcel/tmp-ops/f64-go-cache
export TMPDIR=/home/marcel/tmp-ops
mkdir -p "$EV"
rm -f "$EV/F64-BASE-DONE" "$EV/base-cmd-exit" "$EV/base-int-exit"
cd "$BASE" && go test ./cmd/baha/ -count=1 -json > "$EV/base-cmd.jsonl" 2> "$EV/base-cmd.stderr"
echo "exit=$? epoch=$(date +%s)" > "$EV/base-cmd-exit"
cd "$BASE" && go test ./internal/application/ -count=1 -json > "$EV/base-int.jsonl" 2> "$EV/base-int.stderr"
echo "exit=$? epoch=$(date +%s)" > "$EV/base-int-exit"
echo done > "$EV/F64-BASE-DONE"
