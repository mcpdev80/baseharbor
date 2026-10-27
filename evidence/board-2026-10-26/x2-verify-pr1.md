# X2 – Unabhängige Verifikation PR1-Abnahme (read-only)

- Mission: `ops/missions/x2.txt`
- Gegenstand: `ACCEPTANCE-PR1.md` (baseharbor-plan @ `66fd8ed`) vs. Wirklichkeit
- Rolle: unabhängige Verifikation; es wurden ausschließlich diese Evidence-Datei geschrieben. Keine Produkt-/Planning-Dateien verändert.
- Zeitpunkt der Prüfung: 2026-10-26T23:47Z (Host-UTC)
- baseharbor-plan: HEAD == origin/main == 66fd8ed, Worktree clean (verifiziert via git status/rev-parse)

## PLAN-Checkliste

- [x] Schritt 0: pwd + Evidence-Datei
- [x] ACCEPTANCE-PR1.md @ 66fd8ed gelesen (`evidence/r27-ci-replacement-2026-10-26/ACCEPTANCE-PR1.md`)
- [x] (1) gh run 40419597576 conclusions
- [x] (2) gh pr diff 1090 vs FREEZE-Pfadliste + ec82d476..e12bd1fd Hash-/Diff-Check
- [x] (3) T0/Deadline UTC-Mathematik (+72h)
- [x] (4) Karten-Metadaten der 3 accepted-Karten + plan.py check
- [x] VERDICT

## Befunde

### (0) Turn-Start-Pflichtprogramm (AGENTS.md)

`basics-check.py` FLAGS: 905 unread reminders; POOL ornith 1<2, qwen 1<2 (nur bei vorhandener sinnvoller Arbeit);
PLAN 4 open steps. `events-check.py`: 1 WATCHER EVENT (lane 41e61806 idle), kein Quitting -> kein --ack erforderlich.
Die POOL-/REMINDERS-FLAGS sind Coordinator-Pool-Ereignisse; diese Lane ist per Mission auf read-only-Verifikation
begrenzt, daher hier nur dokumentiert, nicht abgearbeitet (Konfliktregel AGENTS.md: Mission = autoritativ für diese Lane).

### (1) gh run 40419597576 — WIRKLICHKEIT KONFORM

- Workflow: "CI · Source & Quality", headBranch `work/r27-minimal-receipt`, headSha `e12bd1fd80b96a7849f8ce759825a2cc18b7d4d0` (== aktueller PR-#1090-Kopf, identisch mit ACCEPTANCE-Kandidat), status completed, createdAt 2026-10-26T23:24:58Z.
- Gesamtconclusion: **success**. Job "PR Selected Checks": **success**.
- Keine erfundenen Success: success-Jobs = Trusted Plan, Workflow policy, Focused Python, mkdocs de/en-strict, pages-candidate-build, documentation-audit, ci-executor-verifier-tests, ci-actionlint, ci-policy, pr-planner-tests, PR Selected Checks.
- Runtime-/HA-/Exact-Jobs: **skipped** (nicht success behauptet) — konsistent mit ACCEPTANCE-Eintrag "Runtime gates not required by the trusted plan".

### (2) FREEZE vs. Kandidat e12bd1fd — TEILWEISE ABWEICHUNG

a) FREEZE-Integrität: die 5 sha256-Werte in `evidence/board-2026-10-26/FREEZE-PR1.md` stimmen exakt mit den
   ec82d476-Blobs überein (d6f830d9… / 17e955d6… / 2f864f34… / 33d26eaa… / 480beaca…). FREEZE-Record ist wahr.

b) Mission-Check `git diff --stat ec82d476..e12bd1fd -- scripts/receipt_*.py docs/development/minimal*`:
   NICHT leer — 2 Dateien geändert:
   - `scripts/receipt_writer.py` +11 (W2-Härtung `bb2ed8c8` + Safe-Fast-Forward-Divergenz-Guard) -> hashgeändert (e12: e7295425…)
   - `docs/development/minimal-ci-receipts.md` 1 Zeile (Bootstrap-Mapping-Absatz) -> hashgeändert (e12: 64efd04a…)
   Zusätzlich außerhalb des Mission-Globs geprüft (sha256 ec82d476 vs e12bd1fd, alle 5 FREEZE-Pfade):
   - `scripts/receipt_verifier.py` **identisch** (17e955d6…) ✓
   - `scripts/test_receipt_chain.py` **identisch** (2f864f34…) ✓
   - `docs/de/development/minimal-ci-receipts.md` **hashgeändert** (e12: 9b9a2437…, DE-Pendant des EN-Absatzes)

   -> Die Mission-Prämisse "writer/verifier/tests/docs unverändert" ist nur für verifier+chain-Test erfüllt;
      writer + docs EN/DE sind NICHT unverändert gegenüber ec82d476.
      Kontext: `pr1-acceptance-DRAFT.md` hält dieselbe Tatsache ausdrücklich fest ("receipt_writer.py is
      byte-changed … verifier + chain hash-identical zu FREEZE") und verweist auf eine offene
      Owner-Entscheidung zum Freeze-Pinning. Die Abnahme-Akte ist also ehrlich, aber der Freeze-Zustand
      ist nicht byte-treu in den Kandidat überführt worden (koordinierte Revision gemäß FREEZE-Klausel
      "changes require coordinated revision" — die Revision ist durch bb2ed8c8/e12bd1fd-Commits + Reviews
      belegt, aber nicht als FREEZE-Amendment dokumentiert).

c) `gh pr diff 1090 --name-only`: 12 Dateien. Alle in FREEZE-Reservierungen (receipt-*.yml dormant, W2/W3-Skripte+Tests,
   docs EN/DE) außer zwei: `scripts/ci-workflow-policy.sh`, `scripts/test_ci_workflow_policy_receipts.py`
   (Policy-Allowlist-Sync, Commits 29115985/cb01d090, in ACCEPTANCE als "b1rv findings folded in" vermerkt,
   aber nicht in der FREEZE-Pfadliste enumerated).

### (3) T0/Deadline-Mathematik — KONFORM

- ACCEPTANCE: T0 = 2026-10-26T23:35:50Z, Deadline = T0+72h = 2026-10-29T23:35:50Z.
- Nachrechnung (Python, timezone-aware, UTC): 2026-10-26T23:35:50+00:00 + 72h == 2026-10-29T23:35:50+00:00 → korrekt.
- Anmerkung: der 26.10.2026 ist der EU-DST-Umstelltag; die Rechnung ist rein UTC und damit offset-sicher. ✓

### (4) Karten-Metadaten der 3 accepted-Karten — KONFORM (check läuft fehlerfrei)

- `plan.py check` (read-only, bei HEAD=66fd8ed): **325 tasks, 0 errors, 8 warnings**.
- contract / publisher / verifier: `resolution: accepted`, PR-Referenzen auf baseharbor#1090
  (publisher/verifier zusätzlich #1048), Evidence-Links gesetzt (contract/verifier → ACCEPTANCE-PR1.md;
  publisher → o68-publisher.md). Karten-Metadaten untereinander konsistent.
- Die 8 warnings sind reine Abhängigkeitsreihenfolge-Meldungen ("accepted/active although dependencies are
  not accepted", z. B. Upstream t-20261017-release-0-4-27-updates, evidence-resolver noch active) —
  kein Metadaten-Widerspruch in den 3 Karten selbst.
- Beobachtung (nicht blockierend): PR #1090 ist auf GitHub noch **OPEN** (mergedAt=null); die Karten sind
  mit `#status/closed`/`#view/closed` gerendert, während resolution=accepted. Merge-Aktion ist laut
  ACCEPTANCE-PR1 ("Owner approval for later stages … separate") noch nicht erfolgt — Zustandsübergang
  accepted→closed bleibt an den bestätigten Merge gebunden (AGENTS.md Lifecycle).

### Nebenbefunde (nicht blockierend)

- Bootstrap-Prerequisite verifiziert: PR #1091 MERGED, mergeCommit eb617cf2fefea0b… (23:22:34Z), passt zum ACCEPTANCE-Eintrag.
- Uhrzeitpfad konsistent: run createdAt 23:24Z < acceptance 23:35Z < Prüfung 23:47Z (UTC).

## VERDICT

**Abweichungsliste** — die ACCEPTANCE-PR1-Akte ist gegenüber GitHub vollständig wahrheitsgetreu (Checks 1, 3, 4 BESTANDEN),
aber die Mission-Prämisse aus Check (2) ist nicht erfüllt:

1. `scripts/receipt_writer.py`, `docs/development/minimal-ci-receipts.md` und `docs/de/development/minimal-ci-receipts.md`
   sind gegenüber dem FREEZE-Baseline ec82d476 **verändert** (sha256-abweichend in e12bd1fd); nur
   `receipt_verifier.py` und `test_receipt_chain.py` sind hash-identisch. Die Änderungen sind in
   pr1-acceptance-DRAFT.md offengelegt, aber das FREEZE- Dokument enthält kein koordiniertes Amendment dazu.
2. PR-#1090-Diff umfasst mit `scripts/ci-workflow-policy.sh` + `scripts/test_ci_workflow_policy_receipts.py`
   zwei Pfade außerhalb der FREEZE-Pfadliste (Allowlist-Sync, in ACCEPTANCE narrativ gedeckt).
3. Statusbeobachtung: Karten bereits `status/closed`-gerendert, PR #1090 jedoch noch OPEN — closed erst nach
   bestätigtem Merge publizieren (Lifecycle-Regel), andernfalls akzeptiert-als-Zwischenstand kennzeichnen.

Keine erfundenen Success-Werte gefunden; keine Produktdateien geändert.

END X2-DONE
