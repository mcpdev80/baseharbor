# P3R — L5 Review: receipt_tasklink.py + test + dormant post-run-hook

ROLE: REVIEW (read-only; Evidence-only-Commit). Evidence-only, no product mutation.
UTC: 2026-10-26 (Local +0100)
Gegenstand: L5-Baustein `6e170894` (Commit `6e17089492c644fa36030b0124cfc8c490fbbf7e`), Parent `084b3abc`.
Worktree: `worktrees/l5-ustrack` (Branch `l5/ustrack`, base `084b3abc`).

Pruefung:
(1) Fail-closed-Eigenschaften des Task-Links
(2) Bound: schreibt NUR die eine Task-Referenz
(3) YML-Hook: `base_ref`-als-BASE_SHA-Problem + minimale SHA-Aufloesung fuer Aktivierung
(4) Haelt `test_receipt_tasklink.py` was es vorgibt
(5) Integrationshinweise fuer Coordinator-Merge (Konflikte L4/L6?)

---

## (1) Fail-closed — FREIGEBEN

Candidates-Vertrag in `receipt_tasklink.py` (`produce_plan_fixture`) und der
konsumierende Vertrag in `receipt_verifier.py:101-112` (`load_plan`) sind
**identisch**:
- exakt 7 Felder (`gate`, `repository`, `workflow_id`, `head_sha`, `candidate_sha`, `base_sha`, `task`)
- `full_sha`/`sha40`: `re.fullmatch(r"[0-9a-f]{40}", …)` fuer head/candidate/base
- `task` gegen `tasks/[a-z0-9-]+\.md`
- `gate == GATE` ("public-contracts"), `repository == REPOSITORY`
- `workflow_id` positive int

Fail-closed greift sauber:
- Alle 5 Eingaenge ueber `require()` -> `TaskLinkError`; ungueltig -> kein Commit/kein Karten-Touch.
- `main()` fangt `TaskLinkError/OSError/TypeError` ab und ruft `parser.exit(1, "tasklink blocked: …")` -> CLI exit != 0 (Test `test_cli_fails_on_invalid_task`).
- `test_fail_closed_missing_task_reference` (Task "invalid-task-ref" -> `TaskLinkError`),
  `test_fail_closed_wrong_sha_format` (head="abc", base="abc" -> `TaskLinkError`),
  `test_fail_closed_wrong_workflow_id` (workflow_id=-1 -> `TaskLinkError`).
- Kein Write ohne validierte Eingabe; Commit erst nach `target.write_bytes()`, und idempotenter Re-Hash (`unlink`/`add -f`/`commit`) ueberwrites nur bei identischem canonical JSON.

Befund (nicht-blockierend): `base_sha` wird im YML als `github.base_ref || 'main'` gesetzt (`receipt-post-run.yml:48`). In einem `workflow_run`-Trigger auf `github.base_ref` ist der Wert unzuverlaessig (PR-Kontext liefert oft keine Branch-Base-ShA; Fallback 'main' = eine feste ShA, kein echter PR-Base). Das ist ein **Aktivierungs-/Kontext-Problem**, kein Fail-closed-Defizit der Logik: `sha40('main')` faellt -> CLI blockiert. Das ist fail-closed korrekt, liefert beim Aktivieren aber ggf. eine falsche `base_sha`, die `load_plan` dann sauber verwerfen wuerde. Mindestens `target_base_sha`-Korrelation (`receipt_verifier.py:202`) bleibt so ungesichert. -> siehe (3).

## (2) Bound — FREIGEBEN

Script schreibt **ausschlichlich** die eine Task-Referenz bzw. den 7-Felder-Fixture:
- Ziel ist die fixierte Path `evidence/receipt-plans/public-contracts.json` (`PLAN_PATH`).
- `produce_plan_fixture` schreibt `raw = canonical(plan)`, `plan` enthaelt nur die 7 vertraglichen Felder; es werden keine weiteren Kartenfelder/Dateien beruehrt.
- Der YML-Schritt schreibt nur in `$GITHUB_STEP_SUMMARY` (Dokumentation des CLI-Pfads); kein Produkt-Side-Effect ausser dem aktivierten Producer (PR2).
- Idempotenz (Test `test_idempotent_same_fixture_returns_existing_commit`): gleiche Eingaben -> identischer Commit, kein Duplicate.

## (3) YML `base_ref`-als-BASE_SHA + minimale SHA-Aufloesung — Befund + Vorschlag (KEIN Code-Umbau jetzt)

Problem (dormanter Zustand):
- `receipt-post-run.yml:46-49`: `HEAD_SHA = CANDIDATE_SHA = github.event.workflow_run.head_sha` (korrekt),
  `BASE_SHA = github.base_ref || 'main'`.
- Der Job ist hinter einer **never-true Job-Guard**
  `receipt-post-run.yml:30` (`github.repository == 'mcpdev80/receipt-post-run-dormant'`)
  **dormant**. In diesem Zustand wird `BASE_SHA` nie an `receipt_tasklink.py` gefuettert -> **keine Auswirkung auf Live-Betrieb**, nur ein potenzieller Aktivierungsfehler.
- Bei Aktivierung (PR2 ersetzt die Guard): `github.base_ref` ist in `workflow_run` oft leer -> Fallback `'main'` -> `base_sha="main"` -> `sha40('main')`=False -> CLI blockiert (fail-closed ok, aber kein funktionsfaehiger Lauf).

Minimale SHA-Aufloesung fuer Aktivierung (Konkreter Vorschlag, KEIN Code-Umbau):
- `receipt-post-run.yml:48` Base-ShA aus dem workflow_run-Payload holen statt aus `base_ref`:
  `BASE_SHA: ${{ github.event.workflow_run.base.sha || github.event.workflow_run.base_ref }}`
  -> `base.sha` ist die echte PR-Base-ShA; Fallback auf die Branch-Ref, die `receipt_tasklink.sha40` dann zwar nicht akzeptiert, aber die eigentliche Base-ShA aus dem Payload ist 40-char hex.
- Damit ist die minimale gueltige `base_sha` eine **40-char hex ShA** aus `github.event.workflow_run.base.sha`; `'main'`-Fallback durchlaeuft `sha40` NICHT und bleibt blockiert — also im Aktivierungsfall zwingend die Payload-ShA verwenden.
- Ohne diese Zeilen-Andesung bleibt der aktive Pfad funktionsunfaehig (fail-closed, aber wertlos).

## (4) Tests — FREIGEBEN

Ausgefuehrt im l5-ustrack-Worktree:
- `python3 scripts/test_receipt_tasklink.py -v` -> **10/10 OK** (0.83s).
- `python3 scripts/test_receipt_chain.py` -> **6/6 OK** (0.56s, bestehende Suiten unversehrt).

Tests gueltig:
- `test_produce_plan_fixture_writes_correct_fixture`, `test_load_plan_consumes_fixture` (integriert `verifier.load_plan(root, commit)` mit der zurueckgegebenen Commit-ShA -> Link auf Task-Karte lesbar).
- Fail-closed-Tests decken Task/SHA/WorkflowId-Abwehr und CLI-Exit-Code.
- `test_idempotent_*` bestaetigt Re-Commit-Schutz; `test_dry_run_emits_to_stdout` bestaetigt kein Filesystem-Write.
- `test_fail_closed_different_receipt` prueft die mismatch-Detektion (Bestandteil der Idempotenz-/Write-Logik).

Bemerkung (nicht-blockierend): Test-Fixture-Strings (`tasks/receipt-pilot.md`, `abc`-ShAs, wf-id 17) sind realistisch und ueberpruefen beide Pfade (produzieren + konsumieren). Keine Gaps an der_fail-closed-Schwelle.

## (5) Integrationshinweise fuer Coordinator-Merge (L4/L6) — Befund

- Commit in **eigenem Worktree/Branch** `l5/ustrack` (base `084b3abc`), isolierter Write-Set: nur `receipt_tasklink.py`, `test_receipt_tasklink.py`, `receipt-post-run.yml`. Keine Beruehrung von L4-/L6-Dateien -> **kein Content-Konflikt mit L4/L6-Zweigen** zu erwarten; Integration ueber den PR2-Aktivierungscommit.
- Abhaengigkeit: L5 ist das **fehlende Building Block** fuer PR2 (p2-shadow-runbook §3). PR2 aktiviert die Dormancy-Guard und fuehrt `receipt_tasklink.py` als Producer ein; ohne PR2 bleibt L5 inert.
- Merge-Sequenz fuer Coordinator: PR1 (r27-minimal-receipt) -> L4 (Allowlist) -> **L5 (dormant, dieses Review)** -> PR2 (Aktivierung). L5 merge-inabel, solange die never-true-Guard intakt bleibt (dormant).
- Hinweis: `REPOSITORY_ID`/`API_VERSION`/`REPOSITORY`-Konstanten in `receipt_tasklink.py` sind 1:1 zu `receipt_verifier.py` -> bei Abweichung in PR2 Fail-closed-Bruch (`load_plan` wirft) -> in PR2-Aktivierung zwingend gegenpruefen.

---

## VERDICT: FREIGEBEN (mit Aktivierungs-Auflagen)

Der Kandidat erfuellt die Review-Pflicht:
- Fail-closed sauber und vertraglich zu `receipt_verifier.load_plan` (1).
- Bound eingehalten: nur die eine Task-Referenz (2).
- Tests halten was sie vorgeben; 10 fokaliert + 6 bestehend gruen (4).
- YML `base_ref`-als-BASE_SHA ist ein **Aktivierungs-Befund**, kein Fail-closed-Mangel; im dormanten Zustand ohne Live-Wirkung (3).

**Aktivierungs-Auflagen fuer PR2 (zwingend, vor Merge/Aktivierung pruefen):**
1. `receipt-post-run.yml:48` Base-ShA auf `github.event.workflow_run.base.sha` setzen (40-char hex), `'main'`-Fallback entfernen/ersetzen, sonst blockiert `sha40` den aktiven Lauf.
2. `REPOSITORY`/`REPOSITORY_ID`/`API_VERSION`/`GATE` in PR2 gegen `receipt_verifier.py` gegenpruefen (sonst `load_plan`-Fail-closed-Bruch).
3. Nach Aktivierung: `target_base_sha`-Korrelation (`receipt_verifier.py:202`) mit realer PR-Base-ShA in einem pilot-run verifizieren.

Reiner Review; kein Produkt-Umbau dieses Turns (Evidence-only).

END P3R-REVIEW-DONE
