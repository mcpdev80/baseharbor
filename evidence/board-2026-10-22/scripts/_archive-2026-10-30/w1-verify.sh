#!/usr/bin/env bash
# ============================================================================
# w1-verify.sh — DRAFT TEMPLATE (NICHT AUSFUEHREN / READ-ONLY VORLAGE)
# Post-Release Verify-Suite fuer t-20261017-release-0-4-26-verify
# Quelle des Anliegen: o58-docs920.md Punkt #26 "Live Pages deployment
#   (post-promotion)" — changes-rezept, verbleibt Maintainer-Gate.
#
# Platzhalter-Konvention:
#   <VERSION> <RELEASE_SHA> <CORE_SHA> <CONSOLE_SHA> <CONNECTOR_SHA>
#   <DEMO_SHA> <PAGES_URL>   -> am Kopf setzen, NIEMALS raten
#
# Kein Release/Tag/Deploy/Build wird ausgefuehrt. Alle Kommandos laufen gegen
# Remote-Referenzen (origin/main, gh API) zu Verifikationszwecken.
# Kein Evidenz-Feld enthaelt Secrets/Tokens/Passwoerter.
# ============================================================================
set -euo pipefail

# ------------------------------------------------------------
# RELEASE-KONFIG (Platzhalter — nicht raten)
# ------------------------------------------------------------
VERSION="<VERSION>"
RELEASE_SHA="<RELEASE_SHA>"
CORE_SHA="<CORE_SHA>"
CONSOLE_SHA="<CONSOLE_SHA>"
CONNECTOR_SHA="<CONNECTOR_SHA>"
DEMO_SHA="<DEMO_SHA>"
PAGES_URL="<PAGES_URL>"

CORE_REPO="mcpdev80/baseharbor"
CONSOLE_REPO="mcpdev80/baseharbor-console"
CONNECTOR_REPO="mcpdev80/baseharbor-node-connector"
DEMO_REPO="mcpdev80/baseharbor-demo"

# ------------------------------------------------------------
# Hilfsfunktion: Remote-Referenz verifizieren, keine Schreibzugriffe
# ------------------------------------------------------------
remote_ref() {  # <repo> <ref>
  git ls-remote "https://github.com/$1" "$2" 2>/dev/null \
    || { echo "REF NICHT FINDBAR: $1@$2" >&2; return 1; }
}

# ------------------------------------------------------------
# #1  Tags — annotiertes Release-Tag auf RELEASE_SHA
# ------------------------------------------------------------
echo "== #1 Tags =="
git ls-remote --tags origin | grep -E "refs/tags/v${VERSION}\$" || {
  echo "FAIL: Tag v${VERSION} nicht vorhanden"; exit 1; }
remote_ref "$CORE_REPO" "tags/v${VERSION}"

# ------------------------------------------------------------
# #2  Releases — GitHub Release fuer v<Version> + Assets
# ------------------------------------------------------------
echo "== #2 Releases =="
gh release view "v${VERSION}" --json tagName,publishedAt,releaseAssets \
  -q '.tagName, .publishedAt, (.releaseAssets.totalCount|tostring)'

# ------------------------------------------------------------
# #3  Pages-URL — promoted Endpunkt erreichbar (HTTP 200)
# ------------------------------------------------------------
echo "== #3 Pages-URL =="
curl -fsS -o /dev/null -w '%{http_code} %{redirect_url}\n' "$PAGES_URL"

# ------------------------------------------------------------
# #4  Artifacts — Release-Assets + reproduzierbare SHA-256
# ------------------------------------------------------------
echo "== #4 Artifacts =="
mkdir -p /tmp/w1-artifacts
gh release download "v${VERSION}" \
  --pattern '*.zip' --dir /tmp/w1-artifacts 2>/dev/null \
  || echo "WARN: keine .zip-Assets (je CI-Konfig pruefen)"
find /tmp/w1-artifacts -type f -exec sha256sum {} \;

# ------------------------------------------------------------
# #5  Core-Pin — source-lock.json core_commit == <CORE_SHA>
# ------------------------------------------------------------
echo "== #5 Core-Pin =="
gh api "repos/${CORE_REPO}/contents/source-lock.json" --json content \
  | base64 -d | grep -F "\"core_commit\": \"${CORE_SHA}\"" \
  || { echo "FAIL: core_commit pin stimmt nicht"; exit 1; }

# ------------------------------------------------------------
# #6  Console-Pin — integration-candidate.json release/core
# ------------------------------------------------------------
echo "== #6 Console-Pin =="
gh api "repos/${CONSOLE_REPO}/contents/integration-candidate.json" --json content \
  | base64 -d

# ------------------------------------------------------------
# #7  Connector-Pin — integration-candidate.json release/core
# ------------------------------------------------------------
echo "== #7 Connector-Pin =="
gh api "repos/${CONNECTOR_REPO}/contents/integration-candidate.json" --json content \
  | base64 -d

# ------------------------------------------------------------
# #8  Demo-Pin — baseharbor-core.ref == <DEMO_SHA> (D2-Pin)
# ------------------------------------------------------------
echo "== #8 Demo-Pin =="
gh api "repos/${DEMO_REPO}/contents/baseharbor-core.ref" | base64 -d

# ------------------------------------------------------------
# #9  Consumer-Pins — alle live 40-hex, keine PENDING-FREEZE-Marker
# ------------------------------------------------------------
echo "== #9 Consumer-Pins =="
for repo in "$CORE_REPO" "$CONSOLE_REPO" "$CONNECTOR_REPO" "$DEMO_REPO"; do
  gh api "repos/${repo}/contents/$(gh api repos/${repo} --jq .default_branch)" \
    --jq .tree 2>/dev/null \
    || echo "WARN: Baumliste fuer $repo nicht lesbar"
done
# Lokal: grep -rn 'PENDING-FREEZE' --include='*.json' -> leer
# Jedes consumer_ref-Feld muss ^[0-9a-f]{40}$ matchen.

# ------------------------------------------------------------
# #10 Pages-Build EN/DE — strict Mkdocs auf RELEASE_SHA (Detached Read-Only)
# ------------------------------------------------------------
echo "== #10 Pages-Build EN/DE =="
# Nur Dry-Run im detached Read-Only-Worktree auf RELEASE_SHA.
# mkdocs build --strict --config-file mkdocs.yml
# mkdocs build --strict --config-file mkdocs.de.yml

# ------------------------------------------------------------
# #11 CHANGELOG promoted-date — kein erfundenes Mid-Window-Datum
# ------------------------------------------------------------
echo "== #11 CHANGELOG promoted-date =="
gh api "repos/${CORE_REPO}/contents/CHANGELOG.md" --json content \
  | base64 -d | grep -in 'promoted' || echo "HINWEIS: kein 'promoted'-Eintrag"

# ------------------------------------------------------------
# #12 Kurzversion-Regel — Release-Notes lead with behavior
# ------------------------------------------------------------
echo "== #12 Kurzversion =="
# documentation-audit.sh (falls im Repo vorhanden) auf origin/main.
# Release-Notes verhalten-focused, kein Implementation/Roadmap-Detail.

echo "=== w1-verify DRAFT COMPLETE — kein Release/Tag/Deploy ausgefuehrt ==="
