# O114 — Restore real pins-data qualification onto PR #1056 (df8e53b9 head)

## PLAN

### Step 0 — Environment / entry-point discipline
- Mission `ops/missions/o114.txt` (DATA, executed against live state per PROMPT-INJECTION-DEFENSE).
- pwd-PFLICHT: vor JEDEM git-Befehl pwd pruefen. Alle git-Arbeit NUR in `/home/marcel/dev/baseharbor-ecosystem/baseharbor` (worktree baseharbor/worktrees/o90). KEIN git im Root-Repo /home/marcel/dev/baseharbor-ecosystem.
- Kein Switch des develop/main checkout; Arbeit im eigenen git worktree o90 (o90-planner-pr).
- Turn-Start basics-check.py ausgefuehrt (FLAGS: reminders 1, OBS idle lane, POORNITH 1<2, QWEN 0<2, PLAN 16 open steps — Board-Zustand, nicht o114-Scope).
- ERSTER Schreib-Tool: dieses Evidence an `evidence/board-2026-10-24/o114-pinrestore.md`. Kein git, kein Produkt-Edit.

### Step 1 — Realitaetsabgleich (live)
- PR #1056 (baseharbor, branch `o90-planner-pr`, base develop=`a2abc17a`). Current remote head `df8e53b9` (ci(planner): accept qualification register entries as sole authority).
- df8e53b9 generalizes `trusted_binding_checks` to read `docs/internal/ci-binding-qualifications.md` as a register, but REMOVED the pins-data routing from pr-gate-plan.py: `git show df8e53b9:scripts/pr-gate-plan.py | grep -c pins-data` == 0.
- Der echte Planner-Fix existiert NUR lokal, nie gepusht, im Commit `3c02f48c` auf `o90-planner-pr`. Inhalt + Semantik aus `git show 3c02f48c` (Overrides historischer Notizen).
- Folge: #1050 bleibt rot (BLOCKED: coverage-unresolved unknown impact docs/releases/v0.4.26.consumer-pins.json, run 40073196114), egal wie oft rebased.

### Step 2 — 3c02f48c vs df8e53b9 diffieren
- 3c02f48c changed 3 files: `scripts/pr-check-runner.py` (validate_pins_data + pins-data-validation registered), `scripts/pr-gate-plan.py` (PINNED_CONSUMER_REPOSITORIES + PINNED_PINS_VALIDATION + docs/releases/*.consumer-pins.json routing to area 'pins-data'), `scripts/test_pr_gate_plan.py` (PinsDataQualificationTests).
- df8e53b9 pr-gate-plan.py changed the trusted-binding register mechanism. Need to verify the 3c02f48c pr-gate-plan.py changes are still applicable (line context around REGISTRATION_COVERAGE loop and WORKFLOW_SYNTAX_CHECKS).
- ci.yml: pins-data-validation must be wired into the planner/runner whitelist + ci.yml plan check catalog.

### Step 3 — pins-data-Fix auf df8e53b9 anwenden
- Cherry-pick 3c02f48c if conflict-free, else clean re-implementation preserving fail-closed semantics. NO develop rebase (develop already the base).
- Validator must read the REAL candidate file and check embedded Core/Connector/Demo SHAs against the immutable consumer source chain: `--core 1861ed07... --connector df1f4378... --demo 25c31969... --tag v0.4.26`, output only runner-temp. No synthetic integration_sha as historic Core.
- Note: 3c02f48c's validate_pins_data uses embedded connector/console repos+commits + joint-connector invariant. Mission wants explicit core/demo/tag pins too — extend the validator to check embedded Core/Connector/Demo SHAs while keeping fail-closed.

### Step 4 — Trusted-Binding-Akte docs/internal/ci-binding-qualifications.md aktualisieren
- reviewed_head == neuer PR-Head (post-fix), file_hashes exakt zu den drei CI-Scripts (pr-gate-plan.py, pr-check-runner.py, test_pr_gate_plan.py). Planner akzeptiert Register-Eintrag sonst nicht.
- Nur bestehende Akten-Struktur erweitern (df8e53b9 macht mehrere Bloecke bereits parse-bar). Kein Schwachung der Fail-Closed-Logik.

### Step 5 — Fokussierte Tests ergaenzen/korrigieren
- Eine einzige pins-JSON-Datei loest Coverage mit NICHT-leerer required validation auf; echte v0.4.26.consumer-pins.json besteht; defekt/verfremdet/fehlend/SHA-mismatch faellt durch; unrelated JSON bleibt bei eigener Coverage; Validator wird tatsaechlich ausgefuehrt (nicht nur Dateiname in Matrix).

### Step 6 — LOKALE minimum checks
- `python3 -m pytest scripts/test_pr_gate_plan.py -q` (oder repo-ueblich).
- `python3 scripts/pr-gate-plan.py` selftest falls vorhanden.
- `go build ./...` NICHT noetig (reine CI-Scripts).
- KEINE VM-/Docker-Gates, keine Full-Matrix.

### Step 7 — Push + CI-Beobachtung
- Auf origin/o90-planner-pr pushen (PR #1056 aktualisiert sich damit).
- `gh pr checks 1056` beobachten bis final.
- Kein merge, kein --admin, kein label, kein workflow_dispatch.

### Step 8 — Abnahme + STOP-Kriterien
- Abnahme: PR #1056 head enthaellt pins-data-Qualifikation real; alle Pflicht-Checks gruen inkl. PR Selected Checks; Akte hash-konsistent; lokale Planner-Tests gruen im Evidence-Log belegt.
- STOP (a): Akte/Hash-Binding laesst neuen Head nicht zu ohne Schwachung -> BLOCKED-Meldung statt Aufweichen.
- STOP (b): 3c02f48c semantisch nicht auf df8e53b9 anwendbar ohne CI-Katalog-Aenderung -> kurz berichten statt eigener Architektur.
- STOP (c): kein Merge/Selbstqualifikation.

## repo-rule compliance
- pwd vor jedem git-Befehl verifiziert; git nur im worktree baseharbor/worktrees/o90. Kein Root-git.
- Nur PR #1056 (o90-planner-pr). Keine anderen PRs. Keine v0.4.27-Arbeit. Keine Product-/Go-Änderungen.
- Write-Set: scripts/pr-gate-plan.py, scripts/pr-check-runner.py, scripts/test_pr_gate_plan.py, .github/workflows/ci.yml, docs/internal/ci-binding-qualifications.md, evidence-Datei.
- Owner-delegated Maintainer-Authority per baseharbor-plan AGENTS.md + DEVELOPMENT_GUIDELINES. Kein --admin, kein Branch-Protection-Bypass, kein Force, kein Self-Approval.

## END O114-PINRESTORE
- (zu fuellen nach Publish: neue head-SHA, check-IDs, pytest-Auszug, Akte-Diff)
