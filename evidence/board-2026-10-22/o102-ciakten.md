# O102 — Core-921 CI binding qualification akte for #1056

Auftragsquelle: Coordinator im Auftrag von Marcel (OWNER-FREIGABE EXPLIZIT).
Muster: #1007-Akte (git show der damaligen Akte, docs/internal/ci-binding-qualifications.md).
Diagnose: evidence/board-2026-10-22/f39-meta1056.md (Rezept Zeilen 90-114).
PR: baseharbor #1056, branch o90-planner-pr, head 4e737e26af38e7f712f2de1f631633d8d59477b9.
Schreib-Tool nur in evidence/, git nur in baseharbor/. Root-git verboten.

## PLAN

- [x] 0. Evidence-Datei + native Todos ([wip:o102]) anlegen
- [x] 1. Remote-Zustand: PR #1056 OPEN, base develop, head 4e737e26, "PR Selected Checks" FAILURE (Run 40039719214)
- [x] 2. f39-Diagnose gelesen + Rezept; Ziel = Akte mit reviewed_head=4e737e26
- [x] 3. #1007-Muster schema studiert (6 Keys, 1 JSON-Block, Parser raw.count('```')==2)
- [x] 4. HEAD 4e737e26-Blobs sha256 gegen Rezept-Digeste verifiziert (alle 3 match)
- [x] 5. core752_reviewed_merge_chain(1861ed07, 4e737e26, 4e737e26) == True verifiziert
- [x] 6. CI_CHECKS-Ids == required_checks verifiziert (pr-planner-tests, ci-policy, ci-executor-verifier-tests)
- [ ] 7. docs/internal/ci-binding-qualifications.md auf develop schreiben (ein JSON-Block, reviewed_head=4e737e26)
- [ ] 8. Akte lokal validieren (Schema, Block-Zahl, Hashes, chain)
- [ ] 9. PR #1056 gegen develop (baseharbor, kein Merge), f39/q85/codex-verdicts im Body
- [ ] 10. CI beobachten, Lauf in Evidence + o102-ciakten.md dokumentieren
- [ ] 11. END O102-CIAKTEN-DONE mit pr + head

## Befunde / Verifikation

- `trusted_binding_checks` (pr-gate-plan.py:615-701) liest Akte aus target_base_sha (develop)
  und verlangt exakt 6 Keys, 1 JSON-Block (Parser: raw.count('```')!=2 → None), reviewed_head
  40-hex + core752_reviewed_merge_chain, files=={3 CI-Dateien: modified}, file_hashes sha256
  der reviewed_head-Blobs == record, required_checks == CI_CHECKS-Ids.
- reviewed_head = 4e737e26af38e7f712f2de1f631633d8d59477b9 (Kandidaten-HEAD selbst).
  Seine 3 CI-Blobs hashen exakt zu den Rezept-Digesten; first-parent-chain
  4e737e26 ← 3c02f48c ← 1861ed07(develop), merge-base(candidate, develop)=1861ed07 → clean.
- SHA-256 der Blobs (identisch zu f39-Rezept):
  - pr-check-runner.py:     05f244d6e8de34a1f56a1123ce2a8ffb178943f064ef5cad1ed293f0a388517f
  - pr-gate-plan.py:        7c285725156a0b08401dec6e18b11344795f5ed11c931230ef6bd2db0df769e8
  - test_pr_gate_plan.py:   8564c6cb94b3fe035626f05b7b5d3c3cac584f354c43bcd37d80f86c9c86a010
- Validierung: 6-key-schema ok, Block-Zahl=2 (1 JSON), Hashes match, chain=True.
  (broad_required/integration_sha in lokalem resolver != CI-Meta-Job; CI liefert synthetisches
  Integration-Commit; Gate-Logik unverändert, nur Akte-Taten ändern sich.)

## Open / Caveats

- Akte bindet NUR PR #1056 (reviewed_head=4e737e26). Nächster Edit an den 3 Dateien
  invalidiert die Akte erneut (Hash-Bindung, Zeile 680-685).
- Nur PR opened, KEIN Merge. CI beobachten.
- End O102 end mit pr + head.

## END O102-CIAKTEN-DONE

- pr: <PR-URL>
- head: <PR-HEAD-SHA>
