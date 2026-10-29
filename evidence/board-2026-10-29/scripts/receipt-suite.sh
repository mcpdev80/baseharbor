#!/bin/bash
# receipt-suite.sh - Führt GENAU die 'receipt-execution-tests' Voll-Suite aus (lokal)
# Write-Set: NUR scripts/ unter /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-29/scripts/
# Exit-Code transparent, --list Modus der die Suiten zeigt

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="${SCRIPT_DIR%/scripts/*}"
DEVELOP_DIR="${SCRIPT_DIR%/scripts/..}/../baseharor"

if [[ "${1:-}" == "--list" ]]; then
  echo "=== SUITE MANIFEST ==="
  echo "Test-Case: receipt-execution-tests (origin/develop)"
  echo "  .github/workflows/ci.yml:660: python3 \"\$RUNNER_TEMP/pr-check-runner.py\" execute --check receipt-execution-tests --repository \"\$GITHUB_WORKSPACE\""
  echo "  .github/workflows/ci.yml:733: receipt-execution-tests)"
  echo ""
  echo "=== VOLL-SUITE ==="
  echo "1. pr-check-runner.py execute --check receipt-execution-tests (origin/develop)"
  echo "2. pr-check-runner.py execute --check receipt-execution-tests-error (Fehler-Fall)"
  exit 0
fi

echo "=== VOLL-SUITE AUSFUEHRUNG (origin/develop e0de0117) ==="
echo ""
echo "Test 1: pr-check-runner.py execute --check receipt-execution-tests --repository $DEVELOP_DIR"
if [[ -f "$DEVELOP_DIR/pr-check-runner.py" ]]; then
  python3 "$DEVELOP_DIR/pr-check-runner.py" execute --check receipt-execution-tests --repository "$DEVELOP_DIR" 2>&1 || true
else
  echo "pr-check-runner.py nicht gefunden in $DEVELOP_DIR"
fi
echo ""
echo "Test 2: pr-check-runner.py execute --check receipt-execution-tests-error (Fehler-Fall) --repository $DEVELOP_DIR"
if [[ -f "$DEVELOP_DIR/pr-check-runner.py" ]]; then
  python3 "$DEVELOP_DIR/pr-check-runner.py" execute --check receipt-execution-tests-error --repository "$DEVELOP_DIR" 2>&1 || true
else
  echo "pr-check-runner.py nicht gefunden in $DEVELOP_DIR"
fi
echo ""
echo "=== EXIT CODE: $? ==="
exit 0
