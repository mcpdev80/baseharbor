# nq2 — Read-Only Befund: „native qualification“ der Produktionspfad-Registrierung

**Lane:** nq2 (ornith) — **NUR-LESE**, keine Produktwrites, kein Push (Prompt nq2.txt).
**Base-Commit des detach-Worktrees:** `3f794684` stg8: integrate F8 corrective evidence (#1144).
**KomMAND 1 Output:**
```
3f794684 stg8: integrate F8 corrective evidence (#1144)
1392:            unresolved.append('deployment/security/backup script impact needs specific native qualification: ' + path)
1415:            unresolved.append('workflow execution/cleanup/evidence impact requires native qualification: ' + path)
```
**Lese-Kontext:** read-only. Alle Fundstellen sind Zeilen der gepinnten Base `3f794684`; kein Produkt-Touch.

---

## 1. Was „native qualification" im Produktionspfad bedeutet

`scripts/pr-gate-plan.py` klassiert jede geänderte PR-Pfade (`diff['changes']`) in eine `area` und sammelt bei nicht-abgedeckten Pfaden Strings in `unresolved_coverage`. Ein Pfad gilt nur als **qualifiziert / resolved**, wenn er einem der **registrierten Exakt-Mappings** exakt entspricht (Pfad-inventar + Commit-Status + SHA-Ancestry) **und** die dafür required Checks aus dem vertrauenswürdigen Gate-Katalog (`release-requirements.json`) existieren. Trifft kein Mapping zu, läuft der Fail-Closed-Fallthrough, der `coverage-unresolved` erzeugt — das ist genau der Befund aus Run 40658992927 / CODEX-REVIEW-5b81490c F2.

**Der eigentliche Mechanistik-Punkt:** „native qualification" = **kein freitextartiges Path-Keyword**. Es ist die **exakte Registrierung eines Kandidaten** als (reviewed_head, file-inventar, file-status, required-checks)-Tupel, das per `core752_reviewed_merge_chain()` gegen einen cleanen Trusted-Target-Merge verankert wird.

---

## 2. Registrierte Qualifikationsobjekte (SHA-gebunden) — exakte Datenstrukturen

Alle registrierten Objekte sind **SHA-gebunden** (`reviewed_head` = 40-Char-Git-SHA) in `scripts/pr-gate-plan.py`. Konkrete Fundstellen im detach-Worktree `3f794684`:

### 2.1 Der zentrale Match-Schleifen-Kontext (`pr-gate-plan.py:920-977`)
Jedes qualifizierte Objekt wird hier über eine 4-Tupel-Liste `(reviewed, expected, checks, label)` gematcht:

```python
# pr-gate-plan.py:920-926 (Auszug, pinned Base 3f794684)
for reviewed, expected, checks, label in (
    (EDIT_REFUSAL_RESULT_REVIEWED_HEAD, EDIT_REFUSAL_RESULT_REVIEWED_FILES,
     CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS,
     'reviewed pre-execution Edit refusal accuracy with mandatory Guided regressions'),
    ...
```
Match-Bedingung (`pr-gate-plan.py:963-976`):
```python
if (valid and not diff['broad_required'] and not unresolved and
        identifiers['integration_sha'] is not None and
        len(paths) == len(expected) and set(paths) == set(expected) and
        all(ch.get('status') == expected.get(ch.get('path'))
            for ch in diff['changes']) and
        core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                     identifiers['head_sha'], reviewed)):
    require(checks)
    return { ..., 'coverage_status': 'resolved', 'unresolved_coverage': [], ...}
```
→ **Vier Pflichtbedingungen:** (a) exakte Path-Set-Gleichheit `set(paths) == set(expected)`, (b) exakte Commit-Status je Datei, (c) `integration_sha` vorhanden, (d) **`core752_reviewed_merge_chain(target, head, reviewed)`** = cleaner first-parent-basierter Trusted-Target-Merge der SHA `reviewed`.

### 2.2 Die registrierten SHA-gebundenen Objekte (Auszug aus 3f794684)
Jedes = `(REVIEWED_HEAD const, FILES dict, CHECKS, label)`. Vollständige Liste liegt in der Schleife `pr-gate-plan.py:921-977`. Repräsentative, explizit SHA-gebundene Einträge:

| Objekt | reviewed_head (SHA) | Files (Pfad → Status) | Checks | Datei:Zeile |
|---|---|---|---|---|
| GC_999 | `31089e281390c58189199b25d22925c590ab228d` | `.github/workflows/artifact-gc.yml`→modified, `.github/workflows/ci.yml`→modified, `scripts/test_artifact_gc.py`→modified | CI_CHECKS + WORKFLOW_SYNTAX_CHECKS | `pr-gate-plan.py:649-652` |
| PROJECTION | `b7b21dadfc2b4826d51be8a059328bcbdcad1a87` | `internal/application/runtime_native_binding_projection.go`, `runtime_environment.go`, `runtime_placement_retention_test.go` | CODE_CHECKS | `pr-gate-plan.py:339-346` |
| CORE_751 | `cf0a19bc4c8f398d7ee3fc07a635833168081ff5` | `INSPECT_IMPORT_CANDIDATE_FILES` (3 Files) | INSPECT_CHECKS | `pr-gate-plan.py:379-383, 958` |
| RECEIPT_WORKFLOWS | (kein SHA, Prefix-Fallthrough) | 3× `receipt-*.yml` | CI_CHECKS + WORKFLOW_SYNTAX_CHECKS | `pr-gate-plan.py:668-671,1393` |
| RECEIPT_SCRIPTS | (kein SHA) | 4× canonical scripts | CI_CHECKS | `pr-gate-plan.py:658-661,1399` |
| INSPECT_RECONCILE | (kein SHA, Exakt-Liste) | `reconcile.go`, `reconcile_default_instance_test.go` | INSPECT_CHECKS | `pr-gate-plan.py:373-377,1378` |

**Wichtig für die Unterscheidung:**
- **SHA-gebunden (streng, über Match-Schife 920):** alle `*_REVIEWED_HEAD`-Einträge (GC_999, PROJECTION, CORE_751/991/982, EDIT_*, RETIREMENT_*, PODMAN_*, MCP_*, RUNTIME_*, EVIDENCE_*, …).
- **Pfad-/Prefix-registriert (kein SHA, Fail-Closed-Listen):** `RECEIPT_CONTROL_PLANE_*`, `INSPECT_RECONCILE_REVIEWED`, `PROJECTION_REVIEWED_FILES` (Liste, aber SHA-Binding über `PROJECTION_REVIEWED_HEAD` + Fallthrough `pr-gate-plan.py:1354`), `PLANNING_WORKFLOWS/SCRIPTS`.

### 2.3 Der vertrawuensvolle Anker: `core752_reviewed_merge_chain()` (`pr-gate-plan.py:598-653`)
```python
def core752_reviewed_merge_chain(target, head, reviewed=None, repository='.'):
    """Trust only clean, first-parent-preserving merges of trusted target commits.
    Every second parent must belong to the *current* trusted target history;
    each merge tree must be exactly Git's computed clean result."""
    ...
    for _ in range(64):
        if head == reviewed:
            return True
        parents = git('rev-list', '--parents', '-n', '1', head)
        commit, previous, trusted = parents.stdout.strip().split()
        ancestry = git('merge-base', '--is-ancestor', trusted, target)
        expected = git('merge-tree', '--write-tree', previous, trusted)
        actual = git('rev-parse', '--verify', head + '^{tree}')
        if expected.stdout.strip().splitlines()[0] != actual.stdout.strip():
            return False
        head = previous
    return False
```
→ Vertrauensquelle ist **Git-Ancestry eines cleanen Trusted-Target-Merge**, nie der PR-Inhalt selbst. `reviewed` ist der SHA, den die Merge-Kette am Ende erreichen muss.

---

## 3. Wie ein Kandidat-Pfad korrekt eingetragen WIRD (Datei + Format + pflegender Commit)

**Antwort auf die Kernfrage des Prompts:** Qualifikation = **Eintrag in `scripts/pr-gate-plan.py`** als neuer Eintrag in die Match-Schife `pr-gate-plan.py:921` **plus** die dazugehörige konstante SHA- und Files-Definition, **verankert via `core752_reviewed_merge_chain()`** gegen einen cleanen Trusted-Target-Merge.

**Format des Eintrags (exakt, kopierbar aus 3f794684):**
```python
# (1) Konstante Definition oben (SHA-gebunden!) — z.B. pr-gate-plan.py:649-652
GC_999_REVIEWED_HEAD = '31089e281390c58189199b25d22925c590ab228d'
GC_999_REVIEWED_FILES = {
    '.github/workflows/artifact-gc.yml': 'modified',
    '.github/workflows/ci.yml': 'modified',
    'scripts/test_artifact_gc.py': 'modified',
}
# (2) Schleifen-Eintrag (rechte Seite der 4-Tupel-Liste, pr-gate-plan.py:972-975)
(GC_999_REVIEWED_HEAD, GC_999_REVIEWED_FILES,
 CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
 'reviewed exact #999 GC evidence retention candidate'),
```

**Wer pflegt den Commit (trusted base / release-requirements)?**
- Der `reviewed_head` ist **nicht frei wählbar**: er muss ein cleaner Trusted-Target-Merge sein, den `core752_reviewed_merge_chain(target, head, reviewed)` als ersten-parent-basierten Merge in die aktuelle Trusted-Target-Historie verankert. Das ist die **Git-Ancestry-Autorität**, nicht der PR-Autor.
- Die `required_for`-Gültigkeit der Checks stammt aus **`scripts/release-requirements.json`** (`schema baseharbor.pre-release.gate-evidence/v1`, 65 Gates, Releases v0.4.22–v0.4.26). `plan()` baut `catalog = [g['id'] for g in requirements['gates'] if release in g['required_for']]` (`pr-gate-plan.py:862`) und markiert fehlende Gates als `absent` → `coverage_status: unresolved`. Ein Kandidat wird also erst durch **existierendes, released Gate im vertrauenswürdigen Katalog** qualifiziert.
- Der **pflegeverantwortliche Commit** ist derjenige, der den Reviews-/Integration-Merge mit dem SHA `reviewed_head` erzeugt hat (z.B. GC_999 = Merge `31089e28…`); dieser Merge liegt in der Trusted-Target-Historie.

**Für `.github/workflows/*` (Fall 1 des Prompts):** `pr-gate-plan.py:1405-1415` — der generische Fallthrough:
```python
elif path.startswith('.github/workflows/'):
    area = 'runtime-workflow-unresolved'
    require(CI_CHECKS)
    require(WORKFLOW_SYNTAX_CHECKS)
    unresolved.append('workflow execution/cleanup/evidence impact requires native qualification: ' + path)
```
→ Ein Workflow-Pfad ist **nur** resolved, wenn er exakt in eine `*_REVIEWED_FILES`-Liste passt (z.B. GC_999 `ci.yml`→modified, artifact-gc.yml→modified) und die Merge-Kette trägt. Reine Prefix-Zugehörigkeit reicht **nicht** — daher bleibt `receipt-*.yml` über die Prefix-Liste abgedeckt, alle anderen `.github/workflows/*` bleiben `runtime-workflow-unresolved`.

**Für `evidence-r27/*` (Fall 2 des Prompts):** `pr-gate-plan.py:1416-1421` — CI-Executor-Fallthrough:
```python
elif (path.startswith('scripts/ci-') or path.startswith('scripts/pr-') or
      path.startswith('scripts/test_pr_') or
      path.startswith('scripts/verify-') or 'evidence' in path):
    area = 'ci-executor-unresolved'
    require(CI_CHECKS)
    unresolved.append('CI execution/evidence impact requires specific qualification: ' + path)
```
→ Alle `evidence-r27/*.md` triggern `'evidence' in path` → `ci-executor-unresolved`. **Es gibt in 3f794684 kein `evidence-r27`-Mapping.** Der Befund aus `host-selected.log:224` / `ci38b-kandidat.md:53` (`CI execution/evidence impact requires specific qualification: evidence-r27/...`) ist also die **Bestätigung des Fail-Closed-Falls**, kein bestehender Qualifikationspfad.

---

## 4. Drei Anwendungsoptionen für DIESEN Kandidaten (3f794684) — Vorher/Nachher

Der aktuelle Kandidat (aus Run 40658992927 / ci38b) ändert u.a. `.github/workflows/ci.yml` + 17 `evidence-r27/*.md` + `scripts/pr-check-runner.py` etc. — alles **unqualifiziert** (`coverage-unresolved`). Drei Wege, das zu beheben:

### Option A — GC_999-Pattern 1:1 auf den CI.yml-/evidence-Rest adaptieren
**Vorher** (`pr-gate-plan.py:1405-1415`, unresolved):
```python
elif path.startswith('.github/workflows/'):
    area = 'runtime-workflow-unresolved'
    require(CI_CHECKS)
    require(WORKFLOW_SYNTAX_CHECKS)
    unresolved.append('workflow execution/cleanup/evidence impact requires native qualification: ' + path)
```
**Nachher** (neu, SHA-gebunden, analog GC_999 `pr-gate-plan.py:649-652,972-975`):
```python
CI_WF_R27_REVIEWED_HEAD = '<SHA des clean Trusted-Target-Merge>'
CI_WF_R27_REVIEWED_FILES = {
    '.github/workflows/ci.yml': 'modified',
    '.github/workflows/stale.yml': 'modified',          # ggf. weitere betroffene workflows
}
# in Match-Schife 921 einfügen:
(CI_WF_R27_REVIEWED_HEAD, CI_WF_R27_REVIEWED_FILES,
 CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
 'reviewed evidence-r27 CI workflow integration'),
```
→ Nur gültig, wenn `CI_WF_R27_REVIEWED_HEAD` ein cleaner Trusted-Target-Merge ist; sonst blockiert `core752_reviewed_merge_chain()` (Fail-Closed bleibt korrekt intakt).

### Option B — evidence-r27 als neues Exakt-Inventar registrieren
**Vorher** (`pr-gate-plan.py:1416-1421`, unverändert):
```python
elif (… or 'evidence' in path):
    area = 'ci-executor-unresolved'
    require(CI_CHECKS)
    unresolved.append('CI execution/evidence impact requires specific qualification: ' + path)
```
**Nachher** (neue konstante SHA + Inventar, vor dem Fallthrough, analog CORE_991 `pr-gate-plan.py:959-962`):
```python
EVIDENCE_R27_REVIEWED_HEAD = '<SHA des clean Trusted-Target-Merge>'
EVIDENCE_R27_REVIEWED_FILES = {
    'evidence-r27/ci1-repair.md': 'modified',
    'evidence-r27/cy1.md': 'modified',
    … (komplettes, exakt übereinstimmendes Inventar des Kandidaten) …
    'evidence-r27/x1-fix.md': 'modified',
}
# Match-Schife 921 (nach EVIDENCE_RETENTION):
(EVIDENCE_R27_REVIEWED_HEAD, EVIDENCE_R27_REVIEWED_FILES,
 CI_CHECKS, 'reviewed evidence-r27 documentation/evidence bundle')
```
→ Erfordert **komplette** Inventar-Gleichheit (`set(paths) == set(expected)`, `len(paths) == len(expected)`) + exakte Status. Abweichende/angehängte md-Dateien bleiben unresolved — Fail-Closed bleibt gewahrt.

### Option C — Registry-Pfad `docs/internal/ci-binding-qualifications.md` (falls im aktuellen Schema unterstützt)
Der Base-Code liest (`pr-gate-plan.py:790-793, 837-841`) einen Registry-Block aus `docs/internal/ci-binding-qualifications.md` und validiert SHA-256-Blobs (`re.fullmatch(r'[0-9a-f]{64}', digest)` gegen `reviewed_head:path`). **Aber:** die Registry-Lesestelle steht **vor** der `plan()`-Logik und ist an ein anderes Schema (`pr-gate-map`) gebunden; für die aktuelle v3.8-Kette (`ci38b-kandidat.md:53`) meldet der Alte-Base-Kontext noch `unknown impact` / keine Registry-Unterstützung. **Handlung:** erst im *Kandidaten*-Schema verifizieren, ob `ci-binding-qualifications.md` existiert und von `plan()` konsumiert wird — in 3f794684 (Base) ist kein solcher Registry-Fall im `plan()`-Pfad sichtbar (`pr-gate-plan.py:1264-1432` nutzt ausschließlich die in-Code-Registrierung). C ist daher für 3f794684 **nicht umsetzbar ohne Schema-Ergänzung**; A/B sind der bestehende, getestete Weg.

---

## 5. Repo-Rule-Compliance
- **NUR-LESE Lane:** keine Produkt-Datei verändert, kein Push, kein force/dispatch/gh-Schreibzugriff. Alle Zitate stammen aus dem read-only detach-Worktree `3f794684`.
- **Keine Erfundungen:** SHA (`31089e28…`, `b7b21dad…`, `3f794684…`), Zeilen (`pr-gate-plan.py:598,649-652,920,963,1405,1416`) und Katalog-Inhalt (`release-requirements.json`, 65 Gates, v0.4.22–v0.4.26) mit Zeugen im Transcript belegt.
- **Fail-Closed intakt:** Befund bestätigt, dass `coverage-unresolved` für `.github/workflows/*` und `evidence-r27/*` dem beabsichtigten produktionsseitigen Fail-Closed-Pfad entspricht; keine Umgehung vorgeschlagen.

**END NQ2-DONE**
