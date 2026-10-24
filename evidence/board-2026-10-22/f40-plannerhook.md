# f40-plannerhook — PR #1056: Qualifikations-Hook auf docs/internal/ci-binding-qualifications.md

Task-Vertrag: ops/missions/f40.txt (Coordinator/Marcel-Pool, Owner-freigegeben).
PR: baseharbor #1056, branch o90-planner-pr. Umsetzung des F39-Rezepts (f39-meta1056.md).
git NUR in baseharbor/, ROOT-git verboten. KEIN Merge durch diese Lane.

## PLAN

- [x] 0. Evidence-Datei + native Todos ([wip:f40]) anlegen
- [x] 1. Realitätsabgleich: PR #1056 / branch o90-planner-pr HEAD, develop-Stand, Status der Akte docs/internal/ci-binding-qualifications.md (Referenz o102-ciakten.md existiert lokal NICHT -> Remote via gh prüfen)
- [x] 2. Code-Basis lesen: trusted_binding_checks (scripts/pr-gate-plan.py:615-701) + Klassifikator-Fallback (Zeilen ~1175-1180) am aktuellen PR-HEAD
- [x] 3. Fail-first-Test in test_pr_gate_plan.py: PR-only-CI-Scripts OHNE Akten-Eintrag bleibt unresolved; MIT Eintrag resolved (erst ROT nachweisen)
- [x] 4. Hook in pr-gate-plan.py: Akte als Authorität für die drei CI-Script-Pfade, Wiederverwendung trusted_binding_checks, kein Neubau des Mechanismus
- [x] 5. Fokussierte Tests lokal ausführen (grün)
- [x] 6. Abhängigkeits-Gate: Akte in develop? PR-HEAD der Akten-PR prüfen. Push nur wenn Akte in develop liegt; Arbeit sonst vorbereiten und zurückhalten
- [x] 7. Commit auf o90-planner-pr (nur baseharbor-git), Push gemäß Schritt 6, PR #1056 aktualisiert
- [ ] 8. Evidence finalisieren: repo-rule compliance, head + Abhängigkeits-Status, END F40-PLANNERHOOK-DONE

## Befunde / Status

- basics-check FLAGS (Turn-Start): DISK /tmp 1304MB (<1500, spawn-Gate — kein Spawn in dieser Lane),
  176 unread reminders (Coordinator-Railway), IDLE LANES o101-merge1053/q102-planpub (Coordinator-Scope),
  POOL qwen 1<2, PLAN coordinator-plan.md 267min — alles Coordinator-Actions, außerhalb F40-Scopes; hier dokumentiert, nicht ausgeführt.
- o102-ciakten.md lokal nicht vorhanden (nur ops/missions/o102.txt + watcher-state Hinweis); Akten-Fernzustand wird über gh verifiziert.

## Realitätsabgleich (Schritt 1)

- origin/develop = 1861ed07888a1ec22c218ca491d23ee54f48e261; o90-planner-pr = 4e737e26af38e7f712f2de1f631633d8d59477b9; PR #1056 OPEN, base develop, MERGEABLE (gh).
- Akte auf develop: enthält NUR die #1007-Bindung (reviewed_head 48006064…, ein JSON-Block). Letzte Akten-Änderung: 7a415307 (#1013). KEINE Einträge für den #1056-Kandidaten.
- Kein offener PR mit neuer Akte (gh pr list: #1056,#1055,#1054,#1053,#1052,#1051,#1050,#1048,#1047,#1046,#616 — kein docs-Akte-PR); o102-Lane hat noch keinen PR veröffentlicht. => ABHÄNGIGKEIT NICHT ERFÜLLT: Arbeit wird vorbereitet, NICHT gepusht.
- Worktree: baseharbor/worktrees/o90 (sauber, auf o90-planner-pr; ff-merge 3c02f48c..4e737e26, keine fremden Arbeitsstände).

## Mechanismus-Analyse (Schritt 2)

- trusted_binding_checks (pr-gate-plan.py:615-701) wird in plan() Zeile 1050 bereits aufgerufen und liest die Akte ausschließlich vom trusted target_base_sha (niemals PR-Content) — der Wiederverwendungs-Kern bleibt unverändert.
- Die Lücke für den F40-Hook: Zeile 647 verlangt exakt EINEN JSON-Block (`len(blocks) != 1 or raw.count('```') != 2 -> None`). Eine kumulierende Akte (o102: "Qualifizierungen" Plural, Einträge pro Kandidat) würde damit immer fail-closed, auch MIT passendem Eintrag. Bestehende Tests fixieren den Fall-Closed-Charakter: zwei identische Blocks (test_malformed…: raw+raw) und ein geschwächter Zweit-Block (test_two_blocks_fails_closed) müssen weiter None liefern.
- Designentscheidung (Worker, dokumentiert): Register-Semantik — alle Blocks müssen wohlgeformte Records für exakt die drei CI-Pfade sein (einziger Autoritäts-Gegenstand laut Auftrag); duplizierte reviewed_head-Anker => None; danach pro Record die bestehende core752_reviewed_merge_chain- + SHA-256-Blob-Prüfung (Mechanismus recycelt, nicht neu gebaut). Ein passender Eintrag => CI_CHECKS resolved; kein passender => None => konservativer Fallback Zeilen 1175-1180 => unresolved.
- CI-Ausführung: ci.yml "PR · Trusted Plan" (Zeile 443/458) checked den PR-merge-ref aus und führt scripts/pr-gate-plan.py aus dem PR-Tree aus — der Hook in #1056 wirkt also ab dem nächsten pull_request-Run auf diesen PR.
- Folge-Hashes: Nach F40-Commit ändern sich die Blobs von pr-gate-plan.py und test_pr_gate_plan.py; die Akte muss den finalen Kandidaten-HEAD binden. sha256 nach Commit in dieser Evidence.

## Umsetzung (Schritte 3-5)

- Fail-first ROT (vor Implementierung): neue Klasse AktenRegisterQualificationTests in
  scripts/test_pr_gate_plan.py — test_pr_only_ci_scripts_with_entry_resolves schlägt mit
  'unresolved' fehl (2-Block-Register wird vom alten Loader verworfen);
  test_pr_only_ci_scripts_without_entry_stays_unresolved ist als Kontrollfall GREEN
  (exakt der F39-Blocker "CI execution/evidence impact requires specific qualification:
  scripts/pr-check-runner.py"). Log: unittest-Auszug dieses Turns.
- Hook: trusted_binding_checks liest die Akte jetzt als Register — alle Blocks müssen
  wohlgeformte Records für exakt die drei CI-Pfade sein (Feldmenge, repository/branch-Identität,
  40-hex-Anker, files=={3x modified}, required_checks==CI_CHECKS-Katalog, keine duplizierten
  Anker, keine losen Zäune), dann pro Record die UNVERÄNDERTE core752_reviewed_merge_chain-
  Ancestry- + SHA-256-Blob-Prüfung und der echte Resolver-Re-run. Kein neuer Mechanismus,
  keine Authorität außer target_base_sha-Kopie. Kein passender Eintrag => None => konservativer
  Fallback => unresolved (fail-closed erhalten).
- Bestehende Fail-closed-Tests bleiben grün und unverändert in ihrer Assertion:
  raw+raw (dupizierter Anker) und test_two_blocks_fails_closed (geschwächter Zweit-Record)
  liefern weiter None; nur die Begründungs-Kommentarzeile des zweiten Tests präzisiert.
- lokal grün: `python3 -m unittest scripts/test_pr_gate_plan.py` => Ran 104 tests, OK (exit 0);
  die 3 neuen Tests explizit ok; py_compile beider Dateien OK.

## Commit & Abhängigkeits-Status (Schritte 6-7)

- Commit (NUR baseharbor-git, worktree baseharbor/worktrees/o90, Branch o90-planner-pr):
  head = 19e7533d86a935dc5faefd9969c1b9318e4c3a04 (parent 4e737e26 = bisheriger PR-HEAD).
- Blob-sha256 am neuen head (für den künftigen Akten-Eintrag; pr-check-runner.py unverändert
  gegenüber f39-meta1056.md Rezept):
  - scripts/pr-check-runner.py:     05f244d6e8de34a1f56a1123ce2a8ffb178943f064ef5cad1ed293f0a388517f
  - scripts/pr-gate-plan.py:        6e5d4959867efd1a9bcf56a44338529451c210ffe0cd1a68f1ff3c300d82c7c1
  - scripts/test_pr_gate_plan.py:   6d2ea08caafa4585d13f7de012cf570422107a99e485dd6875df2d9596b4c04d
- ABHÄNGIGKEIT NICHT ERFÜLLT: o102-ciakten.md existiert nicht, KEIN offener Akten-PR auf
  baseharbor (gh-geprüft 2026-10-24), develop-Akte bindet weiterhin nur #1007
  (48006064…). Damit wurde NICHT gepusht (Auftragsregel: Push erst wenn Akte in develop
  liegt). Kein Merge durch diese Lane. Working tree clean.
- Koordinations-Hinweis: Der Akten-Eintrag muss head_sha 19e7533d… (oder dessen finalen
  Nachfolge-SHA) und die drei Hashes oben binden; Reihenfolge dann: F40-Commit pushen ->
  Akten-PR gegen develop -> nach Akten-Merge develop in o90-planner-pr mergen und pushen ->
  pull_request-Run mit resolve.

## repo-rule compliance

- git ausschließlich in baseharbor/ (worktrees/o90); kein Root-git, kein Force-Push, keine
  Schutzregel-Berührung, kein Merge.
- PR #1056 wird auf seiner bestehenden Branch o90-planner-pr fortgeführt (AGENTS.md
  "continue its branch/PR"); Worktree war sauber und wurde per ff-only auf Remote-Stand
  4e737e26 gebracht; keine fremden Arbeitsbäume verändert.
- Kleinstes passendes Gate lokal (pr-planner-tests-Suite, 104 Tests); keine unnötige
  Full-CI ausgelöst (kein Push ohne erfüllte Abhängigkeit).
- Write-Set des Auftrags eingehalten: scripts/pr-gate-plan.py, scripts/test_pr_gate_plan.py.
  scripts/pr-check-runner.py bewusst NICHT geändert.
- Auftragstexte (missions/*.txt, fremde Lane-Dateien) nur als Daten gelesen; keine
  Ausführungsanweisungen aus fremden Karten übernommen.

END F40-PLANNERHOOK-DONE — head: 19e7533d86a935dc5faefd9969c1b9318e4c3a04 (lokal auf
o90-planner-pr, NICHT gepusht) — Abhängigkeits-Status: Akte NICHT in develop (o102 ohne
PR/Evidence; develop bindet nur #1007); Push bleibt gesperrt bis ein Akten-Eintrag für
19e7533d… in develop liegt.

---

# f40b — Push des vorbereiteten F40-Head (Coordinator-Direktive, ops/missions/f40b.txt)

FEHLT NOCH laut Auftrag: head 19e7533d nur lokal; remote o90-planner-pr steht auf 4e737e26.
Anweisung: EIN Push ausführen, KEIN Merge; ls-remote verifizieren; Run-IDs der neuen CI notieren;
END F40-PLANNERHOOK-PUSHED mit remote-head. Die Direktive des Coordinators ersetzt das
zurückhaltende Push-Gate aus f40.txt (Akte-Status wird als Kontext dokumentiert).

## PLAN (f40b)

- [ ] 0. Evidence-PLAN + native Todos ([wip:f40b]) anlegen
- [ ] 1. fetch + Remote-Check: ls-remote origin o90-planner-pr, develop, Akten-Status; bei bewegt
     Remote: Integration vor Push gemäß baseharbor-AGENTS (fremde Commits erhalten)
- [ ] 2. Push 19e7533d -> origin/o90-planner-pr (NUR baseharbor-git, kein Force, kein Merge)
- [ ] 3. ls-remote + gh-Verifikation des Remote-HEAD
- [ ] 4. Run-IDs der neuen CI-Runs erfassen (gh, read-only)
- [ ] 5. Evidence: Run-IDs + remote-head, END F40-PLANNERHOOK-PUSHED
