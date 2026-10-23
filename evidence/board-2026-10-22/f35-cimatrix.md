# F35 – CI-Matrix Analyse PR #1050 / #1052 / #1053 (v0.4.26)

[wip:f35] — READ-ONLY Analyse, KEINE Aenderung an Produkt oder PRs.

## Auftrag
CI-Kompatibilitaet der drei Rest-PR-Pfade fuer v0.4.26 analysieren:
- PR #1050 (data, docs/releases/*.consumer-pins.json)
- PR #1052 (vault retry, internal/vault)
- PR #1053 (cli wizard, internal/cli)

Fuer jeden PR: aktuelle Required-Check-Status live (gh pr checks) + welche Checks
laufen/skippen/failen und WARUM (Rule-Kategorisierung via scripts/pr-gate-plan.py
nachvollziehbar). Ziel: Tabelle, die nach o90-Merge voraussagt, welche PRs sofort
gruen werden.

## PLAN
- [x] Evidence-Datei mit PLAN angelegt (pwd-PFLICHT: gh/git nur in baseharbor/)
- [ ] Live-Erfassung gh pr checks #1050 / #1052 / #1053 (Required-Checks)
- [ ] pr-gate-plan.py Rule-Tracing: docs/releases/*.consumer-pins.json / internal/vault / internal/cli
- [ ] Je PR: Checks running/skipped/failing + Begruendung
- [ ] o90-Merge-Kontext klären (was aendert o90 an den Gates)
- [ ] Vorhersage-Tabelle: welche PRs nach o90 sofort gruen
- [ ] Abschluss F35-CIMATRIX-DONE mit Tabelle

## Live-Status (gh pr checks)

(TBD)

## Rule-Tracing pr-gate-plan.py

(TBD)

## Vorhersage-Tabelle post-o90

(TBD)

## repo-rule compliance

READ-ONLY: keine Write-Aktionen in baseharbor/, kein Checkout/Reset fremder
Arbeitsbaeume; gh nur lesend.
