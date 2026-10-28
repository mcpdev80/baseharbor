# LANE qrv — unabhängiger Review der Q-Bindung (Q)

ROLLE: LANE qrv (flash), READ-ONLY Produkt-Review. Kein Commit, kein Push, kein PR-Kommentar.

## PLAN

1. [x] QRV-Pficht: basics-check ausgeführt (FLAGS: 1 un-acked Event, 27 ungeleste Reminders, Pool-Defizite flash/ornith/qwen, 1 offener PLAN-Schritt). Events/Reminders sind Coordinator-Sache, hier nicht geackt/behauptet.
2. [x] worktrees/qrv per `git fetch origin work/r27-ci-bootstrap-qx && git checkout --detach FETCH_HEAD` frisch gestellt; HEAD = d94d051e (SPEC-Tip). Nachweis: `git log --oneline -1` → d94d051e q5: candidate binding constants.
3. [x] SPEC evidence/board-2026-10-27/q5-spec.md + CODEX-DECISION-register.md Abschnitt C gelesen; LANE qrv.txt (K1–K4, END-Formate) gelesen.
4. [x] K1: diff develop..HEAD betrifft NUR scripts/pr-gate-plan.py; RECEIPT_CANDIDATE_REVIEWED_FILES exakt 40-Pfade-Inventar von 4f0ce579d (MATCH).
5. [x] K2: trusted_binding_checks liest nur targetseitiges Register + pr-diff-base; PR-Inhalts-Bindung geprüft, keine Fundstelle die PR-Files vertraut.
6. [x] K3: Fail-closed über dict==dict, 40/64-char Regex, Ancestry, rohe Blob-Hashes — sed-Belege gesichert.
7. [x] K4: `python3 -m unittest test_pr_gate_plan -q` OK; `python3 -m unittest discover -p 'test_pr_check*' -q` OK (22 Tests).
8. [x] evidence-r27/qrv.md geschrieben.

## K1 — diff + Inventar-Gleichheit

DIFF develop..d94d051e (Produkt-Dateien):
```
M scripts/pr-gate-plan.py
```
Einzig geänderte Produktdatei = scripts/pr-gate-plan.py. Keine andere Produkt Datei im qrv-Diff.

40-Pfade-Inventar von H (4f0ce579d) vs. develop, Abgleich `git diff --name-status`:
```
$ git -C baseharbor diff --name-status origin/develop..4f0ce579d | wc -l
40
$ git -C baseharbor diff --name-status origin/develop..4f0ce579d | awk '{print $1}' | sort | uniq -c
22 A
18 M
```

Hardcoded RECEIPT_CANDIDATE_REVIEWED_FILES vs. Git-Inventar (Python-Abgleich, Auszug):
```
expected count: 40
hardcoded count: 40
hardcoded unique paths: 40
keys differ: set()
missing: set()
extra: set()
changed status: {}
RESULT: MATCH
```
HEAD-Konstante: `4f0ce579d3865b0fe86b431071ecf2af31203543` (len 40, valid40 True). Alle 40 Pfade mit exakt korrektem Status (18 modified, 22 added) present, keine fehlende/zusätzliche/umbenannte/statusgeänderte Stelle.

## K2 — trusted_binding_checks vertraut nie PR-Inhalt

Code (d94d051e:scripts/pr-gate-plan.py, Zeilen 737–870):
```python
def trusted_binding_checks(diff, ...):
    valid, _ = validated_diff(diff)
    if not valid or diff['broad_required'] or not diff.get('integration_sha'):
        return None
    ...
    if (context_repository != 'mcpdev80/baseharbor' or
            context_target_branch != 'develop'):
        return None
    def git(*args):
        return subprocess.run(['git', '-C', str(repository), *args], ...).stdout
    try:
        raw = git('show', diff['target_base_sha'] +
                  ':docs/internal/ci-binding-qualifications.md').decode('utf-8')
```
`receipt_files` kommt aus der eingefrorenen Konstante `RECEIPT_CANDIDATE_REVIEWED_FILES`, nicht aus dem PR. Der Registerinhalt wird aus `target_base_sha` (seitenständiges, unverändertes develop) per `git show` gelesen. Der echte Diff wird über den neben dem Planner geladenen Resolver `pr-diff-base.py` (`resolver.resolve_pr_diff(repository, target_base_sha, head_sha, integration_sha)`) neu berechnet (`actual != diff → None`). Fundstelle: im qrv-Diff wird PR-Inhalt nirgends als Autorität gelesen; der einzige neue Inhalt ist die Ziel-inventar-Konstante + die `receipt_files`-Erweiterung der Zulässigkeitsmenge.

## K3 — Fail-closed (sed-Belege)

```python
        receipt_files = RECEIPT_CANDIDATE_REVIEWED_FILES
        workflow_files = {**script_files, '.github/workflows/ci.yml': 'modified'}
        for block in blocks:
            record = json.loads(block, object_pairs_hook=unique_object)
            ...
            record['files'] not in (script_files, receipt_files, workflow_files) or
            ...
            set(record['file_hashes']) != set(record['files']) or
            any(not isinstance(value, str) or
                re.fullmatch(r'[0-9a-f]{64}', value) is None ...):
                return None
```
```python
        changes = actual['changes']
        for record in records:
            if (len(changes) != len(record['files']) or
                    {c.get('path'): c.get('status') for c in changes} != record['files']):
                continue
            if not core752_reviewed_merge_chain(...):
                continue
            for path, digest in record['file_hashes'].items():
                if (... hashlib.sha256(git('cat-file', 'blob',
                                           record['reviewed_head'] + ':' + path)).hexdigest() != digest):
                    break
            else:
                return (CI_CHECKS + WORKFLOW_SYNTAX_CHECKS ... else CI_CHECKS)
    except (...):
        return None
    return None
```
Fehlender Record → loop durchläuft 0 Records → `return None`. Andere files-Menge → `{c.path:c.status} != record['files']` → `continue` → kein `return` → `None`. PR-eigener Record / 39-Pfade-Record / Unsupported → `record['files'] not in (script_files, receipt_files, workflow_files)` → `return None`. Manipulierter Hash → `[0-9a-f]{64}`-Regex / Blob-Hash-Mismatch → `break` → kein `else` → `None`.

## K4 — Tests

```
$ cd baseharbor/scripts && python3 -m unittest test_pr_gate_plan -q
...
Ran 7 tests ... OK   (plus reviewed_utility_tests … Ran 5 tests … OK)
$ python3 -m unittest discover -p 'test_pr_check*' -q
...
Ran 22 tests in 14.473s
OK
```
Hinweis: Lane fragte `-m discover` an; das ist ungültig, korrekt ist `python3 -m unittest discover` (oben ausgeführt, 22 Tests OK).

## repo-rule compliance

Read-only Review im isolierten worktrees/qrv (detached HEAD d94d051e). Kein Produkt-Change, kein Commit/Push/PR, kein Planning-Statuswechsel, kein Claim. basics-check-Flags dokumentiert, nicht behauptet als behoben. Exaktes SHA-Inventar (40 Pfade) statt Zahl 39 erzwungen; Candidate-HA weicht im SPEC-Plan nicht vom beobachteten Kopf ab. qrv-Diff enthält kein Produkt-Receipt-Code und kein zusätzliches Evidence-Verzeichnis — nur scripts/pr-gate-plan.py.

END QRV-ACCEPT
