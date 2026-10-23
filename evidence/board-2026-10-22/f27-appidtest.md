# F27 — appid manifest test regression (env-robust fix)

Card: t-20261020-develop-appid-manifest-test-regression
Origin: Auftrag von o71 uebernommen (2x Lane-Ausfall).
Caveat (lokal): TestExternalProviderRegistrationAndRemoval failt auf dieser Maschine
(provider-registry-ownership-unverifiable) auch auf origin/main; CI gruen.
Ziel: Test UMGEbungsrobust machen (deterministisch im tmp-dir, KEIN skip-if),
kleiner PR gegen develop (mcpdev80/baseharbor). Beweis = lokal PASS + CI PASS.

## PLAN
- [ ] 0. Evidence-Datei + TODOS angelegt (diese Datei)
- [ ] 1. Realitaetsabgleich: baseharbor repo state, origin/main + origin/develop, Karte lesen
- [ ] 2. Test TestExternalProviderRegistrationAndRemoval finden, Fehlschlag lokal reproduzieren
- [ ] 3. Ursache analysieren (warum machine-abhaengig; provider-registry ownership)
- [ ] 4. Fix implementieren: deterministisch im tmp-dir, kein skip-if, isolierter Branch/Worktree
- [ ] 5. Lokal PASS nachweisen (Test + betroffene Gate-Pfade), auf Regressionsrisiko pruefen
- [ ] 6. Commit, Push, PR gegen develop eroeffnen
- [ ] 7. CI PASS ueberwachen und verifizieren
- [ ] 8. Evidence komplettieren, Handoff, END F27-APPIDTEST-DONE mit PR-Nr + local-Nachweis

## repo-rule compliance
(pending)

## Evidence
(pending)

## Findings / Notes
- BASICS turn-start FLAGS verarbeitet: disk /tmp 1308MB (kein Spawn noetig), events keine un-acked,
  reminders 98 unread (nur dieses Lane-Prompt relevant), o56 busy>40min und POOL/PLAN-Flags sind
  Coordinator-Themen, nicht Blocker fuer F27.
