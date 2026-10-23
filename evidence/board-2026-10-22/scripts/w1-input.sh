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

# LIVE 40-Hex-Tipps aus ls-remote (2026-10-23, D2-Finalpin via Release-Branch-Route)
# f29: D2-Gates GRUEN (Run 39990941852). D2-Route = Release-Branch statt PR #119 (obsolet geschlossen).
#   mcpdev80/baseharbor-demo refs/heads/work/v0.4.26-ecosystem-release
#   HEAD 25c319697fd3d89d894943e0f44cd3d5dde8b198 (8 vor main, 0 zurück), baseharbor-core.ref = 1861ed07...
#   Release-Branch-Gates Push-getriggert (Owner-Direktive: mcpdev80-Comment auf #119). Kein eigener Merge.
#   D2-VARIABLE = finale release-branch HEAD (live verifiziert 40-hex, o33 read-only).
D2_RELEASE_BRANCH="work/v0.4.26-ecosystem-release"
D2_RELEASE_BRANCH_HEAD="25c319697fd3d89d894943e0f44cd3d5dde8b198"
D2_CORE_PIN="1861ed07888a1ec22c218ca491d23ee54f48e261"
D2_CORE_PIN_PATH="baseharbor-core.ref"
D2_REPO="mcpdev80/baseharbor-demo"

# Repo-URIs (mcpdev80 Organization)
CONSOLE_REPO="mcpdev80/baseharbor-console"
CONNECTOR_REPO="mcpdev80/baseharbor-node-connector"

# Matrix-Targets (Owner-Direktive 2026-10-22: docker+podman parallel)
MATRIX_TARGETS="docker podman"

# ============================================================
# W1-Dispatch: D2 release-branch HEAD -> matrix runtime (f29, 2026-10-23)
# ============================================================

# Release-Branch-Gates Push-getriggert (Owner-Direktive). D2-Finalpin = release-branch HEAD (live ls-remote 40-hex).
echo "=== W1-Input-Sheet ==="
echo "release branch (D2-Finalpin, release-branch-route statt PR #119 obsolet):"
echo "  repo: ${D2_REPO}"
echo "  branch: ${D2_RELEASE_BRANCH}"
echo "  head: ${D2_RELEASE_BRANCH_HEAD}"
echo "  core.ref (1861ed07): ${D2_CORE_PIN}"
echo "  state: MERGED-INTO-RELEASE-BRANCH"
echo "  gates: PUSH-TRIGGERED (release branch), kein PR-Gate"
echo ""
echo "=== Matrix Targets: ${MATRIX_TARGETS} ==="
echo "=== D2-VARIABLE LIVE verifiziert (ls-remote, 40-hex, o33 read-only) ==="

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

  if [[ "${demo_ref}" =~ ^[0-9a-f]{40}$ ]]; then
    echo "  OK: demo_ref 40-hex (${demo_ref:0:12}…)"
  else
    echo "  ERROR: demo_ref ist ein REQUIRED-Input (gh API: HTTP 422 'Required input demo_ref not provided')," >&2
    echo "         muss 40-hex sein, got: '${demo_ref}'. Aussetzen mit: W1_DEMO_REF=<40-hex> bash w1-input.sh" >&2
    return 1
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
echo "=== W1-Dispatch-Precheck (D2 release-branch HEAD, live ls-remote 40-hex) ==="
verify_ref "${D2_REPO}" "${D2_RELEASE_BRANCH}"
if [[ "${D2_RELEASE_BRANCH_HEAD}" != "25c319697fd3d89d894943e0f44cd3d5dde8b198" ]]; then
  echo "  WARN: ls-remote head ${D2_RELEASE_BRANCH_HEAD} != D2-Finalpin 25c319697fd3 (pruefen)" >&2
fi

if [[ "${DRY_RUN}" != "0" ]]; then
  echo "MODE: DRY_RUN (Standard) — es wird nichts real dispatchet."
  echo "Release-Branch-Gates Push-getriggert (Owner-Direktive); kein PR-Dispatch."
  echo "=== D2-Finalpin fixiert: ${D2_REPO}@${D2_RELEASE_BRANCH} = ${D2_RELEASE_BRANCH_HEAD} ==="
  exit 0
fi

if [[ "${W1_CONFIRM}" != "GO" ]]; then
  echo "ERROR: EXECUTE-Modus erfordert W1_CONFIRM=GO (Owner-Gate)." >&2
  echo "  D2 release-branch-Gates Push-getriggert; kein PR-Dispatch." >&2
  echo "  D2 wird nicht dispatchet." >&2
  exit 1
fi

echo "MODE: EXECUTE (W1_CONFIRM=GO) — Release-Branch-Gates Push-getriggert (Owner-Direktive)."
echo "=== D2-Finalpin: ${D2_REPO}@${D2_RELEASE_BRANCH} = ${D2_RELEASE_BRANCH_HEAD} (core.ref=${D2_CORE_PIN}) ==="
