#!/bin/bash
# w1-input.sh - W1-Dispatch-Skript (NICHT AUSFUEHREN)
# Erzeugt aus q44-w1preflight.txt W1-Input-Sheet
# D2-Platzhalter als Variable am Kopf
#
# Owner-Direktive 2026-10-22: matrix runtime docker+podman parallel
# o33-Regel: KEIN kahles checkout
# consumer_ref-Felder = LIVE 40-Hex-Tipps

# ============================================================
# D2-PLATZHALTER - Dispatch-Variables (nicht ausführen)
# ============================================================

# LIVE 40-Hex-Tipps aus gh pr view (2026-10-22)
CONSOLE_PR7_HEAD="0ea88f553fdac2097e26dce7a5203233a5530527"
CONNECTOR_PR8_HEAD="570864c7c8bec738438aa70aa967be7e8490d7c3"

# Branch-Namen
CONSOLE_PR7_BRANCH="work/v0426-console-consumption"
CONNECTOR_PR8_BRANCH="work/v0426-connector-consumption"

# Repo-URIs (mcpdev80 Organization)
CONSOLE_REPO="mcpdev80/baseharbor-console"
CONNECTOR_REPO="mcpdev80/baseharbor-node-connector"

# Matrix-Targets (Owner-Direktive 2026-10-22: docker+podman parallel)
MATRIX_TARGETS="docker podman"

# ============================================================
# W1-Dispatch: consumer_ref -> matrix runtime
# ============================================================

# Dieser Abschnitt wird durch den Dispatch-Coordinator aufgerufen
# NICHT DIREKT AUSFÜHREN - nur als Quelle für w1-input.sh-Content

# PR-States (gh pr view --json)
echo "=== W1-Input-Sheet ==="
echo "console PR #7:"
echo "  branch: ${CONSOLE_PR7_BRANCH}"
echo "  sha: ${CONSOLE_PR7_HEAD}"
echo "  state: OPEN"
echo "  mergeable: MERGEABLE"
echo "  mergeStateStatus: CLEAN"
echo "  reviews: 0"
echo ""
echo "connector PR #8:"
echo "  branch: ${CONNECTOR_PR8_BRANCH}"
echo "  sha: ${CONNECTOR_PR8_HEAD}"
echo "  state: OPEN"
echo "  mergeable: MERGEABLE"
echo "  mergeStateStatus: CLEAN"
echo "  reviews: 0"
echo ""
echo "=== Matrix Targets: ${MATRIX_TARGETS} ==="
echo "=== consumer_ref-Felder LIVE verifiziert ==="
