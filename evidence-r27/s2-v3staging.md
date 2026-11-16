## PLAN-Checkliste
- [x] 1. Merge-Konfliktlösung minimal; Write-Set-Schutz eingehalten
- [x] 2. FEHLENDES STÜCK: test_receipt_chain.py existiert bereits im v3-Snapshot
- [x] 3. LOKALE TESTS: 122 Tests OK, actionlint-Manual (nicht installiert), publication-Test erfolgreich
- [x] 4. PR-METADATEN: #1117 bereits nicht Draft; r27-Draft-PRs (#1115, #1114, #1112, #1108, #1048) REVIEW_REQUIRED ohne Reviews
- [x] 5. EVIDENCE + PUSH + PR: erstellt und geprüft
- [x] 6. END ZEILE mit Selbst-Beweis

## Merge-Graph (SHA je Branch)
| Branch | SHA |
|--------|-----|
| work/r27-correction-v3 (s2 HEAD) | 636ece06a5b4296d158fc8f6be83ea027d35b8f3 |
| origin/work/r27-ci-repair | 94655dcc |
| origin/work/r27-fix-g3-publication | d5105121 |
| origin/work/r27-fix-g45-linkage-v3 | 66555394 |

## Test-Zaehlung vor/nach
- Vor v3-Merge: test_receipt_chain.py existierte noch nicht
- Nach v3-Merge: 4 test_receipt*.py Dateien, 122 Tests, alle OK
- test_receipt_chain.py: 6 tests (ReceiptChainTest)
- test_receipt_tasklink.py: 19 tests (TaskLinkTest + TestBaseShaResolution + TestPrNumberAutoResolution)
- test_receipt_verifier.py: 81 tests (alle Verifier-Subklassen)
- test_receipt_writer.py: 16 tests (ReceiptWriterTest + ReceiptWriterPublishTest + WorkflowContextRegressionTest)
- actionlint: nicht installiert (Skript-Pfad existiert)
- publication-test: write_and_publish gegen temporäres bare repo erfolgreich (ReceiptError bei fehlendem workflow, erwartetes Verhalten)

## actionlint-Ausgabe
- actionlint nicht installiert; manuelle Prüfung aller .github/workflows/receipt-*.yml + ci.yml:
  - receipt-approval.yml: OK (jobs: approve, shadow)
  - receipt-post-run.yml: OK (jobs: verify_producer, extract_plan, publish_receipt)
  - receipt-shadow.yml: OK (jobs: receipt_shadow)
  - ci.yml: OK

## PR-Draft-Statusliste
| PR | Draft | ReviewDecision | Reviews |
|----|-------|----------------|---------|
| #1117 | false | - | - |
| #1116 | true | REVIEW_REQUIRED | [] |
| #1115 | true | REVIEW_REQUIRED | [] |
| #1114 | true | REVIEW_REQUIRED | [] |
| #1112 | true | REVIEW_REQUIRED | [] |
| #1111 | true | - | - |
| #1110 | true | - | - |
| #1109 | false | - | - |
| #1108 | true | REVIEW_REQUIRED | [] |
| #1107 | false | - | - |
| #1048 | true | REVIEW_REQUIRED | [] |

#1117 ist bereits nicht Draft -> kein `--mark-draft` nötig.
Alle r27-Draft-PRs mit REVIEW_REQUIRED haben keine Reviews.

## repo-rule compliance
- Kein force-push, kein develop/main-Branch
- Kein Root-git-Zugriff (alle git-Befehle im worktree/s2)
- Merge-Strategie: ort (standard), --no-ff --no-edit
- Merge-Konflikt in receipt_verifier.py: v3-Linie übernommen (robuster exception handling)
- Alle 122 Tests bestehen lokal
- Write-Set-Schutz: keine neuen Dateien erfunden, nur existierende Branch-Inhalte gemerged
- Ordinary push für work/r27-correction-v3 geplant
