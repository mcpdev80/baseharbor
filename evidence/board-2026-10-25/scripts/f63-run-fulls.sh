#!/bin/bash
EV=/home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-25/f63-r2-data
rm -f "$EV/F63-FULLRUNS-DONE" "$EV/base-exit" "$EV/head-exit"
cd "$EV/base" && GOCACHE=/tmp/f63-go-cache go test ./cmd/baha/ -count=1 -json > "$EV/base-full.jsonl" 2> "$EV/base-full.stderr"
echo "exit=$? epoch=$(date +%s)" > "$EV/base-exit"
cd /home/marcel/dev/baseharbor-ecosystem/worktrees/f63-r2 && GOCACHE=/tmp/f63-go-cache go test ./cmd/baha/ -count=1 -json > "$EV/head-full.jsonl" 2> "$EV/head-full.stderr"
echo "exit=$? epoch=$(date +%s)" > "$EV/head-exit"
echo done > "$EV/F63-FULLRUNS-DONE"
