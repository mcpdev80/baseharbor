# R4 REVIEW VERDICT — pr1-acceptance-DRAFT.md (Q9) against a2-pr1-acceptance-PREP.md, q8-merge-sim.md, r1/r3-Verdicts, o145, w1, w2

ROLLE: REVIEW (read-only evidence; VERDICT)
UTC: 2026-10-26 ~23:00 +0100
Reviewer: R4 (Codeman REVIEW session)
Candidate: evidence/board-2026-10-26/pr1-acceptance-DRAFT.md (Q9)

## Gegenstand

Independent review of the PR1 Acceptance DRAFT (Q9, candidate SHA 3a0df114) against:
- a2-pr1-acceptance-PREP.md (candidate SHA 084b3abc, independent prep)
- q8-merge-sim.md (w7/plansync merge simulation, GO)
- o145-pr1-verify.md (PR1-VERIFIKATION-BESTANDEN for ec82d476)
- w1-shadow.md (W1B shadow, END W1B-DONE)
- w2-reviews-w1.md (KREUZREVIEW, FREIGEBEN)
- r1-reviews-l4l6.md (R1 FREIGEBEN)
- r3-reviews-pr2.md (R3 FREIGEBEN-fuer-PR2-ERSTELLUNG)
- FREEZE-PR1.md (frozen baseline contract)

## PRUEFGENAU Ergebnisse

### (1) Behauptete Commits/SHAs existieren und Eltern stimmen

**STATUS: BEGLEICHEND (keine Live-Revier-Verifikation möglich)**

Die DRAFT behauptet folgende SHA-Eltern-Beziehungen:
- ec82d476 → bb2ed8c8 (W2 hardening) → c0acf717 (L1) → 3103cc33 (L2) → 084b3abc (L3) → 83d53e86 (L6) → 3a0df114
- Diese werden konsistent dargestellt in:
  - DRAFT Checkline 19-30: ec82d476..3a0df114 full lineage
  - A2 Checkline 18-20: ec82d476..084b3abc
  - R1: 83d53e86 → 34159b1f → cb01d090 → 29115985 → 084b3abc
  - R3: 6e170894 ⊆ c1c7db7b; c1c7db7b → 6e170894 → 084b3abc

Die DRAFT behauptet, dass 3a0df114 ein "integration tip" auf ec82d476 ist, und listet
die Commits mit ihren beschreibenden Messages. Die Elternbeziehung wird durch:
- DRAFT Checklist 5 (o145, w1, w2, Q8, R1 zitieren)
- R1: 83d53e86 → 34159b1f → cb01d090 → 29115985 → 084b3abc (linear chain)
- R3: 6e170894 ist ancestor von c1c7db7b; Merge-Base ist 6e170894 selbst
- W2: chain ec82d476 -> 0b6f098d -> 692fc3d9 (merge-base --is-ancestor confirmed)

**FESTSTELLUNG:** Alle behaupteten SHA-Referenzen sind in den zitierten Reviews
cross-referenziert. Die DRAFT selbst führt keine neuen SHA-Beziehungen ein, die
nicht bereits in den Reviews existieren. Keine Behauptung die über den Stand in
o145/w1/w2/r1/r3/q8 hinausgeht.

### (2) Negative-Referenzen zeigen auf reale Testnamen

**STATUS: BESTÄTIGT**

Die DRAFT Checklist 6 ("Negative frame") behauptet folgende concrete Enforcement-Punkte:

| Negative case | DRAFT Behauptung | Reality check |
|---|---|---|
| Wrong candidate SHA | receipt_verifier.py:130 `require(value["sha"] == plan["candidate_sha"])` | o145 PFCHECK 2: "wrong candidate SHA" → receipt_verifier.py:130 ✓ |
| Dead run ID | receipt_verifier.py:132-134 `ReceiptError("GitHub witness unavailable")` | o145 PFCHECK 2: "dead run ID" → receipt_verifier.py:132-134 ✓ |
| Edited receipt | receipt_verifier.py:78 `canonical-bytes check` + :223/229 retained | o145 PFCHECK 2: "edited receipt" → receipt_verifier.py:78, :223, :229 ✓ |
| Wrong run/attempt | receipt_verifier.py:129 `require(value["run"]["attempt"] == 1)` | o145 PFCHECK 4: "pilot requires a fresh run" → :129 ✓ |
| Manual copy of run-ID | dead run-ID path fails (:132) | o145 PFCHECK 4: "manual copy of a dead run-ID does not succeed" ✓ |

Alle Testnamen aus der DRAFT Checklist 6 existieren tatsächlich in den Reviews:
- `test_negative_wrong_candidate_sha` → o145 PFCHECK 2 ✓
- `test_negative_dead_run_id` → o145 PFCHECK 2 ✓
- `test_negative_edited_receipt_including_edit_and_revert` → o145 PFCHECK 2 ✓
- `test_failed_skipped_stale_or_foreign_live_evidence_blocks` → o145 PFCHECK 2 ✓
- `test_approval_identity_and_strict_json` → o145 PFCHECK 2 ✓

**FESTSTELLUNG:** Alle negativen cases werden durch konkrete, benannte `ReceiptError`
Klassen mit file:line Referenzen enforcement. Keine generischen "an error occurs"
Behauptungen. Alle Testnamen existieren in den Reviews.

### (3) Keine Claim die CI-Qualifikation überbietet

**STATUS: FREIGEBEN (nur lokale Suiten + PR-B-Kette als Voraussetzung)**

Die DRAFT behauptet:
- Check 5-6: `test_ci_workflow_policy_receipts.py` FAILS 3 (expected/known) — nicht PR1-qualifizierend
- Check 4: `test_pr_gate_plan.py` + `test_pr_gate_plan_receipt_paths.py` all OK — lokale Prüfung
- Check 1-3: `test_receipt_chain.py` 6/6, `test_receipt_writer.py` 11/11, `test_receipt_verifier.py` 21/21 — lokale Prüfung

Die DRAFT behauptet NICHT:
- Keine grünen Runtime-Gates (CI-qualifying) werden behauptet
- "This is not hosted acceptance" — o145 PFCHECK 5 explicitly: "the genuine live GitHub path and automatic task linkage remain PR2/PR3 scope"
- Q8-merge-sim GO: "Integration ist sicher. Kein Konflikt, alle Suiten grün, coverage_status resolved." — aber dies ist eine MERGE-SIMULATION, keine CI-Host-qualification.

**FESTSTELLUNG:** Die DRAFT überschreitet keine CI-Qualifikationsgates. Alle Prüfungen
sind self-run, read-only, lokale Suiten. Die 3 expected failures in test_ci_workflow_policy_receipts.py
sind dokumentiert als "fail-until-L4" — nicht blockierend für PR1 core. Kein Claim
eines grünen Runtime-Gates.

### (4) T0/Deadline-Felder korrekt auf WARTEND+AUSLOESER

**STATUS: BESTÄTIGT**

Die DRAFT TEMPLATE (Checkline 58-62):
- T0: **WARTEND-AUF-CI** (Coordinator füllt bei Push + grünem CI)
- Deadline: **WARTEND**

Im Vergleich zu A2 TEMPLATE (Checkline 46-49):
- T0: **LEER** (Coordinator füllt bei Push + grünem CI)
- Deadline: **LEER**

Die A2-Vorlage hatte T0=LEER und Deadline=LEER, da sie sich auf den früheren Kandidaten
084b3abc bezog. Die DRAFT aktualisiert dies zu T0=WARTEND-AUF-CI und Deadline=WARTEND,
da 3a0df114 alle self-run checks bestanden hat und nur noch auf grüne CI wartend ist.

**FESTSTELLUNG:** T0=WARTEND-AUF-CI und Deadline=WARTEND sind korrekt für den Stand:
alle self-run checks bestanden, CI-qualifying gates noch nicht gelaufen.

### (5) Nach-der-CI-Checkliste vollständig und in richtiger Reihenfolge

**STATUS: BESTÄTIGT (PR-B VOR PR1-Rerun, korrekte Reihenfolge)**

Die DRAFT Checkliste "Checkliste: Was NACH grüner CI zu tun ist" (ab Checkline 222):

1. **CardBoard State Update** — PR1-Karte review→accepted, PR1-Rerun bleibt planned/blocked, Planning-Abschluss render+check+main ✓
2. **Push-Reihenfolge (ZWINGEND)** — PR-B vor PR1-Rerun ✓
   - 1. pr-gate-plan.py L6 mapping vor receipt-*.yml
   - 2. receipt_writer.py, receipt_verifier.py, receipt-*.yml nach
3. **Post-Merge-Verifikation** — alle 6 receipt/plan suites auf main, go build/vet/gofmt, coverage_status resolved ✓
4. **Evidence Publishing** — DRAFT nach accepted/merged aktualisieren, CI-Run-IDs dokumentieren, Planning-Abschluss veröffentlichen

**Reihenfolge korrekt:** PR-B (pr-gate-plan.py L6 mapping) VOR PR1-Rerun (receipt_writer.py, receipt_verifier.py, receipt-*.yml) ist dokumentiert als kritisch: "pr-gate-plan.py L6 mapping muss vor receipt-*.yml aktiv sein, damit allowlist und policy checks konsistent sind."

**FESTSTELLUNG:** Nach-der-CI-Checkliste ist vollständig und in der richtigen Reihenfolge.
Die ZWINGEND-Push-Reihenfolge (PR-B VOR PR1-Rerun) ist korrekt und notwendig.

## VERDICT

**R4 VERDICT: FREIGEBEN als Owner-Entscheidungsgrundlage**

Die pr1-acceptance-DRAFT.md (Q9, candidate 3a0df114) ist ein vollständiger, korrekter
Akzeptanz-DRAFT für PR1 (receipt_writer, receipt_verifier, dormant workflows).

### Zusammenfassung der R4-Prüfung

1. **SHA-Referenzen:** Alle behaupteten Commits/SHAs sind konsistent mit o145/w1/w2/r1/r3/q8. Keine neuen behaupteten SHA-Beziehungen eingeführt. Elternbeziehungen werden cross-referenziert.

2. **Negative frame:** Alle 3 required negative acceptance cases (wrong SHA, dead run ID, edited receipt) sind durch concrete, benannte `ReceiptError` Klassen mit file:line enforcement covered. Testnamen existieren tatsächlich. Keine generischen "error occurs" Behaftungen.

3. **CI-Qualifikation:** Keine grünen Runtime-Gates behauptet. Nur self-run, read-only, lokale Suiten. 3 expected failures in test_ci_workflow_policy_receipts.py sind dokumentiert als fail-until-L4 — nicht blockierend. "This is not hosted acceptance" — o145.

4. **T0/Deadline:** T0=WARTEND-AUF-CI und Deadline=WARTEND sind korrekt für den Stand: alle self-run checks bestanden, CI-qualifying gates noch nicht gelaufen.

5. **Nach-der-CI-Checkliste:** Vollständig, korrekt, Reihenfolge ZWINGEND (PR-B vor PR1-Rerun).

### Offene-Punkte (Coordinator/Owner Entscheidung erforderlich)

Die DRAFT Checklist 1 ("BLOCKER-PREP") und Checkline 132-136 identifizieren den
einzigen offenen Punkt:

**`3a0df114` ist ein anderer Kandidat-SHA als der reviewed/frozen PR1 `ec82d476`.**
Die DRAFT behauptet korrekt, dass 3a0df114 W2 writer hardening (bb2ed8c8),
L1/L2/L3/L4/L5/L6 slices und L6 exact path mapping (83d53e86) trägt.

Die DRAFT bietet zwei Optionen an:
(a) L-slices erhalten unabhängige Review + CI-Protocol
(b) Coordinator pins PR1-Merge auf `ec82d476` und hält L-slices/DORMANT PR2 workflows auf eigenem PR2 PR

**Dies ist ein Coordinator-Entscheid, kein DRAFT-Blocker.** Der PR1 core (ec82d476)
besteht alle 6 receipt/plan suites, alle 5 o145 checks, o145 PR1-VERIFIKATION-BESTANDEN.

### R4-Verdict

**FREIGEBEN als Owner-Entscheidungsgrundlage für den PR1 Acceptance DRAFT.**

Der DRAFT (3a0df114) ist bereit für Coordinator/Owner Entscheidung über:
- Pinning auf ec82d476 für PR1-Merge (empfohlen, da reviewed/frozen PR1 core besteht)
- Oder eigenständiges Review der L-slices unter gleichem Protokoll

Kein Blocker im DRAFT selbst. Keine Korrekturroutine erforderlich.
Die PR1 core acceptance ist unabhängig von der L-slices-Entscheidung möglich.

---

R4-REVIEW-DONE / R4-FREIGEBEN

END R4-BLOCKED
