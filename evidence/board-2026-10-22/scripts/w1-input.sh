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

# ============================================================
# W1-Dispatch: AUSFUEHR-TEIL (nur aus w1-input.sh-Content)
# ============================================================
# W1-Matrix-Vertrag LIVE verifiziert 2026-10-23:
#   Orchestrator .github/workflows/pre-release.yml (workflow_dispatch, id 362658451)
#   reusable       .github/workflows/pre-release-runtime-suite.yml (workflow_call, id 371960202)
#   inputs  dispatch: tag|required, ref|default=develop, demo_ref|required(40-hex),
#             consumer_ref|optional(40-hex), only_gates|optional
#   source-job loest consumer_ref fuer tag v0.4.24/v0.4.25 via ecosystem_release_pins.py;
#     demo_ref aus docs/releases/${tag}.demo-ref (40-hex-Pflicht).
#   docker+podman parallel: runtime_light_gates_docker + runtime_light_gates_podman
#     (both needs [source, runtime_candidate], matrix runtime:[docker|podman], max-parallel 2).
#   runtime-suite inputs: ref, demo_ref, runtime(static|docker|podman), gate, candidate_artifact.
#
# D2-Platzhalter: NICHT real dispatchen (Run 39977865544 = 404 "Not Found").
# consumer_ref/demo_ref/ref = LIVE 40-hex / Branch-Referenz, kein erfundener D2-Pin.
# o33-Regel: KEIN kahles checkout, alle Referenzen remote (gh/git ls-remote).

W1_REPO="mcpdev80/baseharbor"
W1_WORKFLOW_NAME="Release · Pre-Release Validation"
W1_WORKFLOW_PATH=".github/workflows/pre-release.yml"
W1_REF_DEFAULT="develop"

DRY_RUN="${DRY_RUN:-1}"
W1_CONFIRM="${W1_CONFIRM:-}"
W1_TAG="${W1_TAG:-v0.4.26}"
W1_DEMO_REF="${W1_DEMO_REF:-}"

resolve_consumer_ref() {  # <repo> <branch> <expected_head>
  local repo="$1" branch="$2" expected="$3" actual
  actual="$(git ls-remote "https://github.com/${repo}" "refs/heads/${branch}" 2>/dev/null | awk '{print $1}')"
  if [[ -z "${actual}" ]]; then
    echo "  ERROR: consumer branch ${branch} in ${repo} nicht via ls-remote findbar" >&2
    return 1
  fi
  if [[ -n "${expected}" && "${actual}" != "${expected}" ]]; then
    echo "  WARN: ls-remote head ${actual} != erwarteter head ${expected} (pruefen)" >&2
  fi
  echo "${actual}"
}

verify_ref() {  # <repo> <ref>  (remote 40-hex-Check, o33 read-only)
  local repo="$1" ref="$2" sha
  sha="$(git ls-remote "https://github.com/${repo}" "refs/heads/${ref}" 2>/dev/null | awk '{print $1}')"
  if [[ "${sha}" =~ ^[0-9a-f]{40}$ ]]; then
    echo "  OK: ${repo}@${ref} = ${sha} (remote, kein kahl-Checkout)"
  else
    echo "  WARN: ${repo}@${ref} nicht als 40-hex remote sichtbar (Branch/Ref pruefen)" >&2
  fi
}

dispatch_matrix() {  # <tag> <ref> <demo_ref> <consumer_ref> <mode>
  local tag="$1" ref="$2" demo_ref="$3" consumer_ref="$4" mode="$5"

  verify_ref "${W1_REPO}" "${ref}"

  if [[ -n "${demo_ref}" ]]; then
    if [[ "${demo_ref}" =~ ^[0-9a-f]{40}$ ]]; then
      echo "  OK: demo_ref 40-hex (${demo_ref:0:12}…)"
    else
      echo "  ERROR: demo_ref muss 40-hex sein, got: ${demo_ref}" >&2
      return 1
    fi
  else
    echo "  HINWEIS: demo_ref nicht gesetzt (Owner-Wert, aus docs/releases/${tag}.demo-ref)" >&2
  fi

  local consumer_label="${consumer_ref:-<leer, owner-loest via ecosystem_release_pins.py>}"
  echo "  consumer_ref: ${consumer_label}"

  local label
  for label in ${MATRIX_TARGETS}; do
    echo "--- Matrix target: ${label} ---"
    local cmd=(gh workflow run "${W1_WORKFLOW_NAME}" --repo "${W1_REPO}" --ref "${ref}" \
      --field tag="${tag}" --field ref="${ref}" --field demo_ref="${demo_ref}" \
      --field consumer_ref="${consumer_ref}")
    local cmd_str="gh workflow run \"${W1_WORKFLOW_NAME}\" --repo \"${W1_REPO}\" --ref \"${ref}\" \\
      --field tag=\"${tag}\" --field ref=\"${ref}\" --field demo_ref=\"${demo_ref}\" \\
      --field consumer_ref=\"${consumer_ref}\""
    echo "  DRY: ${cmd_str}"

    if [[ "${mode}" == "execute" ]]; then
      echo "  EXEC: ${cmd_str}"
      "${cmd[@]}"
    fi
  done
}

echo ""
echo "=== W1-Dispatch-Precheck ==="
resolve_consumer_ref "${CONSOLE_REPO}" "${CONSOLE_PR7_BRANCH}" "${CONSOLE_PR7_HEAD}" >/dev/null
resolve_consumer_ref "${CONNECTOR_REPO}" "${CONNECTOR_PR8_BRANCH}" "${CONNECTOR_PR8_HEAD}" >/dev/null

if [[ "${DRY_RUN}" != "0" ]]; then
  echo "MODE: DRY_RUN (Standard) — es wird nichts real dispatchet."
  echo "Genaue gh-Workflow-Dispatch-Kommandos:"
  dispatch_matrix "${W1_TAG}" "${W1_REF_DEFAULT}" "${W1_DEMO_REF}" "${CONSOLE_PR7_HEAD}" "dry"
  echo ""
  echo "=== D2-Hinweis: Run 39977865544 = 404 -> D2 NICHT real dispatchen ==="
  exit 0
fi

if [[ "${W1_CONFIRM}" != "GO" ]]; then
  echo "ERROR: EXECUTE-Modus erfordert W1_CONFIRM=GO (Owner-Gate)." >&2
  echo "  Realer W1-Dispatch (docker+podman parallel) nur mit: W1_CONFIRM=GO DRY_RUN=0 bash w1-input.sh" >&2
  echo "  D2 wird nicht dispatchet." >&2
  exit 1
fi

echo "MODE: EXECUTE (W1_CONFIRM=GO) — docker+podman PARALLEL dispatching."
# docker + podman parallel (Owner-Direktive 2026-10-22), niemals serialisiert.
dispatch_matrix "${W1_TAG}" "${W1_REF_DEFAULT}" "${W1_DEMO_REF}" "${CONSOLE_PR7_HEAD}" "execute" &
pid_docker=$!
dispatch_matrix "${W1_TAG}" "${W1_REF_DEFAULT}" "${W1_DEMO_REF}" "${CONSOLE_PR7_HEAD}" "execute" &
pid_podman=$!
wait "${pid_docker}" "${pid_podman}"
echo "=== W1-EXECUTE finished (docker+podman parallel) ==="
