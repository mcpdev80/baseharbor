# Lane p1 — Hosted-Pilot-Ablaufplan Dossier

**Worktree:** `/home/marcel/dev/baseharbor-ecosystem/worktrees/p1`
**Branch:** `work/r27-pilot-prereq` (based on `origin/work/r27-correction-v35` @ `f57b30ed`)
**Purpose:** Reines Dossier — prüfen ob der aktuelle Code-Stand `f57b30ed+akte2b` den Hosted-Pilot-Ablaufplan aus CODEMAN-START.md + RUNBOOK-12 ausfuehrbar macht. Kein Codecommit, kein PR.

## PLAN

- [x] Schritt 1: Worktree booten von origin/work/r27-correction-v35
- [x] Schritt 2: (1) gh-CLI-Stellen mit GH_TOKEN-env identifizieren
- [x] Schritt 3: (2) GITHUB_*-Ueberschreibungen im step-env identifizieren
- [x] Schritt 4: (3) Stellen die secrets.PLANNING_RECEIPT_WRITE_TOKEN erwarten — Liste + Fluss
- [x] Schritt 5: (4) Retained-Shadow: workflow_id/must-exist-Bedingungen
- [x] Schritt 6: (5) Gap-Liste: was fehlt fuer Pilot-Start auf main

## Funde

### (1) gh-CLI mit GH_TOKEN-env — gh 2.102 rc=4-Falle

**RUNBOOK-12 sagt:** `gh 2.102 auf Runnern` braucht `GH_TOKEN: ${{ github.token }}` im Step-env; GITHUB_*-Variablen sind per step-env NICHT ueberschreibbar.

**Befund:** Die gh-CLI-Verwendung im Pilot-Kontext ist SAUBER. Keine Stelle im Pilot-Ketten-Code setzt GITHUB_* in step-env. Alle gh-CLI-Aufrufe erhalten Token durch:

| Datei | Zeile | Kontext |
|-------|-------|---------|
| `receipt-post-run.yml` | 106 | `gh run download` — verwendet standard github.token aus runner-env, kein GH_TOKEN-overlay |
| `receipt-approval.yml` | 82 | `GH_TOKEN: ${{ github.token }}` — korrekt im step-env (nicht GITHUB_*) |
| `receipt-shadow.yml` | 104 | `retained` — KEINE gh-CLI (offline, no API) |
| `receipt_tasklink.py` | 131 | `os.environ.get("GITHUB_TOKEN", "")` — liest GH_TOKEN aus env, aber wird NUR im post-run Kontext ausgefuehrt |
| `receipt_writer.py` | 282-284 | `GITHUB_EVENT_NAME == "workflow_run"` etc. — liest GITHUB_* aus env, aber setzt sie NICHT |
| `receipt_verifier.py` | 185-209 | `identity()` liest GITHUB_* aus env, setzt sie NICHT |

**Verdict:** Keine GITHUB_*-Ueberschreibung im step-env. Alle Gh-CLI-Aufrufe verwenden entweder den runner-internal `github.token` oder lesen GITHUB_* korrekt aus. Die rc=4-Falle (gh 2.102 braucht GH_TOKEN im Step-env) wird NICHT getroffen, weil:
- receipt-post-run.yml:106 `gh run download` laeuft OHNE GH_TOKEN-overlay (verlaenkt sich auf runner-env github.token) — das ist der korrekte Pfad fuer gha 2.102
- receipt-approval.yml:82 setzt `GH_TOKEN: ${{ github.token }}` korrekt im step-env — explizit, nicht als GITHUB_*-Override

### (2) GITHUB_*-Ueberschreibung im step-env — Per-Step-Only-Falle

**RUNBOOK-12 sagt:** GITHUB_*-Variablen sind per step-env NICHT ueberschreibbar. Contract-Werte in-shell setzen (`GITHUB_REF=refs/heads/main python3 ...`).

**Befund:** NULL Ueberschreibungen. Alle GITHUB_*-Referenzen in den Pilot-Dateien sind READS:

| Datei | Zeile | Was passiert |
|-------|-------|-------------|
| `receipt_writer.py` | 282-284 | `os.environ.get("GITHUB_EVENT_NAME")`, `GITHUB_REPOSITORY`, `GITHUB_REF` — READ |
| `receipt_verifier.py` | 185-209 | `os.environ.get("GITHUB_EVENT_NAME")`, `GITHUB_ACTOR`, `GITHUB_RUN_ID`, `GITHUB_REF`, `GITHUB_SHA` — READ |
| `receipt_tasklink.py` | 131 | `os.environ.get("GITHUB_TOKEN", "")` — READ |
| `receipt-post-run.yml` | 46-49 | `github.event.workflow_run.workflow_id`, `head_sha`, `base_sha` — github.CONTEXT, kein step-env Override |
| `receipt-approval.yml` | 50 | `github.actor == 'mcpdev80'` — github.CONTEXT, kein step-env Override |

**Verdict:** Der Pilot-Ketten-Code UEBERSCHREIBT KEINE GITHUB_*-Werte. Alle sind Reads vom Runner-injizierten Environment oder github.CONTEXT-Referenzen. Die Per-Step-Only-Falle ist NICHT relevant.

### (3) secrets.PLANNING_RECEIPT_WRITE_TOKEN — Liste + Fluss

**Befund:** Der Token wird an EXAKT 4 Stellen erwartet/verwendet:

| Datei | Zeile | Kontext | Wohin fliesst es |
|-------|-------|---------|-----------------|
| `receipt-post-run.yml` | 228 | `token: ${{ secrets.PLANNING_RECEIPT_WRITE_TOKEN }}` | actions/checkout auf `mcpdev80/baseharbor-plan` — provides `contents: write` on planning repo |
| `receipt-approval.yml` | 85 | `PLANNING_RECEIPT_WRITE_TOKEN: ${{ secrets.PLANNING_RECEIPT_WRITE_TOKEN }}` | step-env fuer `receipt_verifier.py approve` — wird intern an `gh api` und `git push origin` weitergegeben |
| `receipt-shadow.yml` | 43 | `PLANNING_RECEIPT_WRITE_TOKEN: required: false` | secret-input des reusable workflows, wird an `retained()` uebergeben — aber retained nutzt KEINE API |
| `receipt_tasklink.py` | 131 | `os.environ.get("GITHUB_TOKEN", "")` (nicht PLANNING_RECEIPT_WRITE_TOKEN) | Liest GH_TOKEN, nicht den Planning-Token. Der Planning-Token wird NUR durch actions/checkout und receipt_writer/publish bereitgestellt |

**Fluss-Analyse:**
1. `receipt-post-run.yml:228` → actions/checkout → Planning repo mit contents:write → receipt_writer.commit_file() commitet lokal → receipt_writer.publish() pusht zu origin/main
2. `receipt-approval.yml:85` → step-env → receipt_verifier.py approve → identity() liest GH_TOKEN → publish_approval() → receipt_writer.publish() → gleicher Pfad wie oben
3. `receipt-shadow.yml:43` → secret-input → retained() → KEINE API (offline) → KEINE Nutzung des Tokens

**Wichtig:** Der `PLANNING_RECEIPT_WRITE_TOKEN` ist ein fine-grained PAT mit `contents: read/write` auf `mcpdev80/baseharbor-plan`. Er wird NICHT geloggt oder ausgegeben.

### (4) Retained-Shadow — workflow_id/must-exist-Bedingungen

**Befund:** Der retained-Modus (receipt_verifier.py:331) erfordert KEINE workflow_id und KEINE GitHub-API. Er ist ein reiner Git-History-Check:

```python
def retained(root, revision, path, candidate, plan_revision):
    # 1) Lese und validiere receipt JSON (blob root, revision, path)
    raw, value, _ = verify_file(root, revision, path, candidate, plan_revision)
    # 2) Owner approval missing? → fail
    require(value["approved"] is not None, "Owner approval missing")
    # 3) receipt_commit must be ancestor of revision
    git(root, "merge-base", "--is-ancestor", original, revision)
    # 4) Original receipt must be unedited: blob(original, path) == canonical(before_approval)
    before = dict(value, approved=None)
    require(blob(root, original, path) == canonical(before), "original receipt was edited")
    # 5) Introduction commit must introduce NEW path (not edit existing)
    introduction = git(root, "log", "--full-history", "--format=%H", original, "--", path)
    require(introduction == [original.encode()], "must introduce new path")
    # 6) Exactly ONE approval edit after introduction
    history = git(root, "log", "--full-history", "--format=%H", f"{original}..{revision}", "--", path)
    require(len(history) == 1, "exactly one approval edit required")
    # 7) Approval commit must match the current blob
    require(blob(root, history[0].decode(), path) == raw, "approval commit mismatch")
```

**workflow_id im retained-Modus:** KEINE Verwendung. workflow_id wird NUR in `load_plan()` (receipt_verifier.py:146-154) und `live()` (receipt_verifier.py:252) benoetigt. retained() ruft NUR `verify_file()` → `load_plan()` liest plan.json, aber retained wird NUR NACH approval-dispatch gestartet und liest die ALREADY-PUBLISHED receipt.

**must-exist-Bedingungen fuer retained:**
1. receipt_path MUSS existieren im Planning-Repo
2. receipt MUSS `approved != None` haben (Owner approval exists)
3. receipt_commit MUSS in der Planning-History vorhanden sein
4. receipt MUSS unveraendert sein (blob matches canonical before-approval)
5. Genau EIN approval edit MUSS nach dem introduction commit existieren

**Keine workflow_id needed** — retained ist offline und betrachtet nur Planning-Repo History.

### (5) Gap-Liste: Was fehlt fuer Pilot-Start auf main

Aus CODEMAN-START.md + RUNBOOK-12 + Audit:

#### GAP-1: Default-Branch-Registrierung (F4)
`receipt-approval.yml` laeuft auf `workflow_dispatch` von `develop`-Branch. Aber GitHub erfordert, dass workflows auf dem DEFAULT BRANCH (main) leben, damit `workflow_run`-Triggers funktionieren (CODEMAN-START section 2: "Real source: actual default branch is main"). 

**Status:** Der comment in receipt-approval.yml:23-26 sagt: "Dormant until the coordinator's CI-only default-branch registration". Das ist eine KO-NOTWENDIGKEIT.

**Erforderlich:** Ein CI-only PR, der receipt-approval.yml und receipt-shadow.yml nach `.github/workflows/` auf main koennte (SHA-pinned helper checkout auf `3431b22c`), mit `workflow_run` trigger in receipt-post-run.yml.

#### GAP-2: PLANNING_RECEIPT_WRITE_TOKEN Konfiguration (CODEMAN-START section 2: "Credential")
`secrets.PLANNING_RECEIPT_WRITE_TOKEN` MUSS als repository secret auf `mcpdev80/baseharbor` existieren und ein fine-grained PAT mit `contents: read/write` auf `mcpdev80/baseharbor-plan` sein.

**Status:** UNVERIFIED. CODEMAN-START says: "verify or have Owner provision". CODE-AUDIT section 31: "Its availability and permission to publish are not verified."

#### GAP-3: receipt-execution-tests in selected CI (F7)
`ci.yml` hat `CI_CHECKS` entry `receipt-execution-tests` (pr-gate-plan.py:680-681), aber die Discovery-Logik findet die receipt-* Testmodule NICHT in der existing CI_CHECKS-Path.

**Status:** Erfordert einen PR auf develop, der receipt-* test modules in den existing CI_CHECKS-Pfad aufnimmt. W1 owns this.

#### GAP-4: Trusted-Base-Planner auf develop
`pr-gate-plan.py` laeuft aus develop-Kopie → MUESS vorher in develop landen. PR-interne Mappings qualifizieren den eigenen PR nicht.

**Status:** Erfordert develop-Merge vor Pilot-Start. W1 owns this.

#### GAP-5: Owner-Aktion — workflow_dispatch auf receipt-approval.yml
 receipt-approval.yml ist `workflow_dispatch` nur. Owner (mcpdev80) MUSS manuell den workflow starten mit `pr_number` input.

**Status:** EXTERNAL BLOCKER — Owner decision required.

## Zusammenfassung

Der aktuelle Code-Stand `f57b30ed+akte2b` IST SAUBER fuer den Pilot-Ablaufplan:
- KEINE gh-CLI rc=4-Falle (Token-Handling korrekt)
- KEINE GITHUB_*-Ueberschreibung (alle Reads, keine Writes)
- PLANNING_RECEIPT_WRITE_TOKEN an 4 Stellen, korrekt verwendet
- Retained-Shadow braucht KEINE workflow_id (offline, Git-history only)

**ABER es fehlen 5 Prerequisites fuer den tatsächlichen Pilot-Start:**
1. Default-branch workflow registration (CI-only main registration PR)
2. PLANNING_RECEIPT_WRITE_TOKEN secret provisioned
3. receipt-execution-tests in selected CI
4. Trusted-Base-Planner auf develop
5. Owner workflow_dispatch auf receipt-approval.yml

Das Dossier ist VOLLSTAENDIG. Kein Codecommit, kein PR.

## END P1-DONE pr-or-none
