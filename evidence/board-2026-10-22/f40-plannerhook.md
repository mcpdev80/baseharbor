# f40-plannerhook — PR #1056: Qualifikations-Hook auf docs/internal/ci-binding-qualifications.md

Task-Vertrag: ops/missions/f40.txt (Coordinator/Marcel-Pool, Owner-freigegeben).
PR: baseharbor #1056, branch o90-planner-pr. Umsetzung des F39-Rezepts (f39-meta1056.md).
git NUR in baseharbor/, ROOT-git verboten. KEIN Merge durch diese Lane.

## PLAN

- [x] 0. Evidence-Datei + native Todos ([wip:f40]) anlegen
- [x] 1. Realitätsabgleich: PR #1056 / branch o90-planner-pr HEAD, develop-Stand, Status der Akte docs/internal/ci-binding-qualifications.md (Referenz o102-ciakten.md existiert lokal NICHT -> Remote via gh prüfen)
- [x] 2. Code-Basis lesen: trusted_binding_checks (scripts/pr-gate-plan.py:615-701) + Klassifikator-Fallback (Zeilen ~1175-1180) am aktuellen PR-HEAD
- [ ] 3. Fail-first-Test in test_pr_gate_plan.py: PR-only-CI-Scripts OHNE Akten-Eintrag bleibt unresolved; MIT Eintrag resolved (erst ROT nachweisen)
- [ ] 4. Hook in pr-gate-plan.py: Akte als Authorität für die drei CI-Script-Pfade, Wiederverwendung trusted_binding_checks, kein Neubau des Mechanismus
- [ ] 5. Fokussierte Tests lokal ausführen (grün)
- [ ] 6. Abhängigkeits-Gate: Akte in develop? PR-HEAD der Akten-PR prüfen. Push nur wenn Akte in develop liegt; Arbeit sonst vorbereiten und zurückhalten
- [ ] 7. Commit auf o90-planner-pr (nur baseharbor-git), Push gemäß Schritt 6, PR #1056 aktualisiert
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
