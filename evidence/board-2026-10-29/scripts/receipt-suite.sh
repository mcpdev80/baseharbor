#!/bin/bash
# receipt-suite.sh - Runde K2 (o246): local receipt test suite for the
# card-evidence publication path (receipt_tasklink.py + receipt_verifier.py).
#
# Runs the two production receipt test suites against the o246 worktree at
# commit 545eaca9 (detached) and exits 0 only when every test passes.
#
# Write-Set of THIS file: evidence/board-2026-10-29/scripts/ (only).
# It reads the baseharbor worktree read-only; it never writes product code.
#
# Usage:
#   receipt-suite.sh [WORKTREE] [--list]
# WORKTREE defaults to /home/marcel/dev/baseharbor-ecosystem/worktrees/o246
#
# Exit codes:
#   0  all suites passed
#   1  one or more suites failed
#   2  usage error (unknown worktree / missing test files)

set -uo pipefail

DEFAULT_WORKTREE="/home/marcel/dev/baseharbor-ecosystem/worktrees/o246"
WORKTREE="${1:-$DEFAULT_WORKTREE}"
LIST_ONLY=0
if [[ "${1:-}" == "--list" ]]; then
  LIST_ONLY=1
  WORKTREE="$DEFAULT_WORKTREE"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$LIST_ONLY" -eq 1 ]]; then
  echo "=== receipt-suite.sh (Runde K2 / o246) ==="
  echo "Worktree : $WORKTREE (detached HEAD @ 545eaca9)"
  echo "Suites:"
  echo "  1. scripts/test_receipt_tasklink.py    (72 tests: card-evidence publish path)"
  echo "  2. scripts/test_receipt_verifier.py    (119 tests: receipt verifier + approve_pr)"
  echo "  3. scripts/test_receipt_chain.py        (chain integration)"
  exit 0
fi

if [[ ! -d "$WORKTREE" ]]; then
  echo "ERROR: worktree not found: $WORKTREE" >&2
  exit 2
fi

TASKLINK="$WORKTREE/scripts/test_receipt_tasklink.py"
VERIFIER="$WORKTREE/scripts/test_receipt_verifier.py"
CHAIN="$WORKTREE/scripts/test_receipt_chain.py"

for f in "$TASKLINK" "$VERIFIER" "$CHAIN"; do
  if [[ ! -f "$f" ]]; then
    echo "ERROR: required test file missing: $f" >&2
    exit 2
  fi
done

cd "$WORKTREE"

echo "=== receipt-suite.sh (Runde K2 / o246) ==="
echo "Worktree: $WORKTREE"
echo "HEAD    : $(git -C "$WORKTREE" rev-parse HEAD 2>/dev/null || echo unknown)"
echo ""

fail=0

echo "--- Suite 1: test_receipt_tasklink.py ---"
if python3 -m pytest "$TASKLINK" -q; then
  echo "[PASS] test_receipt_tasklink.py"
else
  echo "[FAIL] test_receipt_tasklink.py"
  fail=1
fi
echo ""

echo "--- Suite 2: test_receipt_verifier.py ---"
if python3 -m pytest "$VERIFIER" -q; then
  echo "[PASS] test_receipt_verifier.py"
else
  echo "[FAIL] test_receipt_verifier.py"
  fail=1
fi
echo ""

echo "--- Suite 3: test_receipt_chain.py ---"
if python3 -m pytest "$CHAIN" -q; then
  echo "[PASS] test_receipt_chain.py"
else
  echo "[FAIL] test_receipt_chain.py"
  fail=1
fi
echo ""

if [[ "$fail" -eq 0 ]]; then
  echo "=== EXIT 0: all receipt suites passed ==="
  exit 0
else
  echo "=== EXIT 1: one or more receipt suites failed ==="
  exit 1
fi
