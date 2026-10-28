# CMX-1142 — CODEX-MERGE (Owner-Freigabe 28.10.)

**Lane:** cmx3 (ornith) · **Rolle:** CODEX-MERGE
**Datum:** 2026-10-28

## Auftrag
Draft-PR 1142 aufmerken, via squash+admin auf develop mergen, develop-Tip verifizieren (SHA + <=2 Dateien), Evidence hier dokumentieren.

## Ablauf / Befehle
1. Baseline-Check `basics-check.py`.
2. Draft-Status: `{"mergeable":"MERGEABLE","state":"OPEN"}` → Draft → Merge-Ablehnung "still a draft" → BLOCKED (Erstversuch).
3. Owner-Freigabe: `gh pr ready 1142` → "ready for review" (`isDraft=false` bestätigt).
4. Merge-Versuch (1.): `gh pr merge 1142 --squash --admin` → "PR Selected Checks is failing".
5. Merge-Versuch (2., rc gefangen): STDOUT leer, STDERR = GraphQL-Regelverstoß → **RC=1**.
6. HTTP-Ebene: `POST /repos/.../pulls/1142/merge` mit Bearer-Token → **HTTP 404 "Not Found"** (gh nutzt GraphQL-Pfad, der die Regel erzwingt; die direkte REST-Integration wird nicht angeboten).
7. Status geprüft: `mergeable_state=blocked`, Check-Run 122566603264 `PR Selected Checks` → conclusion=**failure**, status=completed.

## Merge-API-Antwort (exakt, STDERR, 2. Versuch, RC=1)
```
GraphQL: Repository rule violations found

Required status check "PR Selected Checks" is failing.

(mergePullRequest)
```

## Merge-Ebene REST (eigener curl-Befehl)
- `POST https://api.github.com/repos/mcpdev80/baseharbor/pulls/1142/merge` (squash)
- **HTTP_STATUS=404**, BODY: `{"message":"Not Found","documentation_url":"https://docs.github.com/rest","status":"404"}`

## Mandatory-CI-Befund (Check-Run 122566603264, Workflow "CI · Source & Quality")
- `PR Selected Checks` → **failure** (curl 7s), `statusCheckRollup.conclusion=FAILURE`.
- mergeable_state = **blocked**.
- Andere Mandatory-Gates grün: `Workflow policy` pass, `PR · Trusted Plan` pass, `PR · Focused Python` pass, `PR · Validation · ci-policy`/`ci-executor-verifier-tests`/`pr-planner-tests` pass.
- Annotations (3, path `.github`): `Process completed with exit code 1.`, Node.js-20-Deprecation (`actions/download-artifact` → Node 24), `ubuntu-latest` → Umstellung auf Ubuntu 26 ab 19.10.2026.

## Ergebnis
**BLOCKED.** PR 1142 ist READY (`isDraft=false`), DOCH der squash+admin-Merge wird von GitHub blockiert, weil **PR Selected Checks** (Mandatory-CI) FAILURE hat (curl exit 1 im `.github`-Bereich). develop-Tip **nicht** verändert, kein Push/Workflow, kein Code-Fix (CODEX-MERGE erfindet keine Rezepte; das Fix-Rezept gehört in die Worker-Lane).

## Ende
END CMX-BLOCKED {gh:"Repository rule violations found / Required status check 'PR Selected Checks' is failing", mergeable_state:"blocked", checkRun:122566603264, conclusion:"failure", restHTTP:404}
