#!/bin/bash
side="$1"; dir="$2"
D=/home/marcel/tmp-ops/f65-fulldata
export GOCACHE=/home/marcel/tmp-ops/f65-go-cache
export TMPDIR=/home/marcel/tmp-ops/f65-t-$side
mkdir -p "$TMPDIR"
cd "$dir"
go test ./cmd/baha/ -count=1 -json > $D/$side-cmd.jsonl 2> $D/$side-cmd.stderr
echo "exit=$? epoch=$(date +%s)" > $D/$side-cmd-exit
go test ./internal/application/ -count=1 -json > $D/$side-int.jsonl 2> $D/$side-int.stderr
echo "exit=$? epoch=$(date +%s)" > $D/$side-int-exit
echo done > $D/$side-DONE
