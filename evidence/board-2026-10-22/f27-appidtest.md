# F27 — appid manifest test regression (env-robust fix)

Card: t-20261020-develop-appid-manifest-test-regression
Origin: Auftrag von o71 uebernommen (2x Lane-Ausfall).
Caveat (lokal): TestExternalProviderRegistrationAndRemoval failt auf dieser Maschine
(provider-registry-ownership-unverifiable) auch auf origin/main; CI gruen.
Ziel: Test UMGEbungsrobust machen (deterministisch im tmp-dir, KEIN skip-if),
kleiner PR gegen develop (mcpdev80/baseharbor). Beweis = lokal PASS + CI PASS.

## PLAN
- [x] 0. Evidence-Datei + TODOS angelegt (diese Datei)
- [x] 1. Realitaetsabgleich: origin/develop 1861ed07, origin/main 54acc4b6, Karte gelesen (state review, F27-Caveat-NOTIZ unten auf der Karte)
- [x] 2. Test gefunden: internal/application/external_provider_registry_test.go:11; lokaler FAIL reproduziert auf origin/develop (9 Failures im Paket, identischer Mechanismus)
- [x] 3. Ursache: t.TempDir() erzeugt das nummerierte Verzeichnis mit os.Mkdir(dir, 0o777) -> bei Maschine-umask 002 entsteht 0775; die Protected-Owner-Guards (verifyExternalRemovalRegistryOwner external_provider_scope.go:200, ownedWorkloadProjectionDirectory runtime_binding_projection_reset.go:48, ownedOTLPProjectionParent telemetry_projection_owner.go:52) verwerfen group-writable dirs fail-closed. CI-umask 022 -> 0755 -> gruen. Produktions-Guards sind korrekt; Test-Umgebung war nicht deterministisch.
- [x] 4. Fix: geteilter Helper protectedTempDir(t) (t.TempDir + explizit Chmod 0700) in internal/application/protected_tempdir_test.go, eingesetzt an den 7 umask-sensiblen creation-Stellen in 5 Testdateien. KEIN skip-if, keine Umweltabfrage, Parser/Guard-Semantik unveraendert; Rejection-Tests (scope_parent 0770, chmod 0644) bleiben unveraendert aktiv.
- [x] 5. Lokal PASS: go test ./internal/application/ -count=1 -> ok (3.1s, komplettes Paket); Zieltests PASS unter umask 002 UND 022 UND 077; go vet + gofmt clean.
- [x] 6. PRs: mcpdev80/baseharbor#1046 (Testfix, develop) + #1047 (gate-plan Lane, develop) — Begruendung unten bei CI.
- [x] 7. CI: #1047 komplett gruen (run 39998286306; Trusted Plan, Workflow policy, pr-planner-tests, ci-policy, executor-verifier, Focused Python, PR Selected Checks alle pass). #1046-head 35582508... laeuft aktuell noch fail-closed gegen den develop-planner (coverage-unresolved, vorbestehende Policy-Luecke, exakt wie auf der Karte am 2026-10-20 dokumentiert); wird nach #1047-Merge + base-refresh (gh pr update-branch) ueber den neuen exakten Source-Lane gruen. Merge + Review sind Maintainer/Codex-Aktionen, nicht Worker-Selbstfreigabe.
- [x] 8. Evidence + Handoff + END (diese Datei, delegation-inbox.txt)

## repo-rule compliance
- Isolierte Worktrees/Branches pro Task: worktrees/f27-appid-env-robust (fix/f27-appid-env-robust), worktrees/f27-gateplan-lane (fix/f27-gateplan-apptest-lane); kein paralleles Schreiben in dieselben Dateien; Main-Checkout (q54-docsfix) unberuehrt.
- Bewusst kleinster PR + ein schmaler Policy-PR nach exakt dem akzeptierten #1008/#1009-Präzedenz (kein generischer _test.go- oder internal/application-Exemption, Merge-Chain-Bindung an SHA 3558250837759113b760a112e99994135d31bc00).
- CI-Effizienz: kleinste passende Gates (source-contracts laeuft die geaenderten Tests direkt); keine Docker-/Podman-Gates auf betroffenen Pfaden noetig ( reine Go-Unit-Tests, kein Runtime-Eingriff).
- Temporäre Beweismittel: eine Probe-Testdatei (zz_probe_test.go) wurde nur zur Diagnose angelegt und VOR dem Commit entfernt; nichts ungeplantes im Produktbranch.
- Kein skip-if, kein Force-Push, keine Schutzregel-Bypass, keine Behauptung gruener Gates.

## Evidence
- Root cause probe (machine, umask 002): dataDir /tmp/Test.../001 mode drwxrwxr-x; verify err=provider-registry-ownership-unverifiable. GOROOT testing.go makeTempDir: os.Mkdir(dir, 0o777).
- PR (testfix): https://github.com/mcpdev80/baseharbor/pull/1046 — head 3558250837759113b760a112e99994135d31bc00 — diff genau 6 Testdateien (5 modified, 1 added).
- PR (policy lane): https://github.com/mcpdev80/baseharbor/pull/1047 — head 79e0aa2b03f66e1a056976d018f4f97949e51417 — scripts/pr-gate-plan.py + scripts/test_pr_gate_plan.py (72 added lines).
- CI #1047: run 39998286306 — alle required checks pass.
- CI #1046 (vor Lane-Merge): run 39995102290 — source-contracts/Trusted Plan/workflow policy/focused python PASS; PR Selected Checks FAIL coverage-unresolved (https://github.com/mcpdev80/baseharbor/actions/runs/39995102290/job/120557200731) —预期的 fail-closed Policy-Luecke, nicht Testfehler.
- Lokal: go test ./internal/application/ PASS; Zieltests PASS bei umask 002/022/077; planner regressions neu+alt PASS (python3 -m unittest scripts.test_pr_gate_plan.AppIDEnvRobustTestExactQualificationTests → ok).
- Hinweis: lokale Planner-Vollsuite zeigt 3 vorbestehende Disk-Quota-Fehlschlaege (/tmp 85%) in unrelated nested go/shell regressions; keine Logikfehlschlaege durch dieses Diff.

## Findings / Notes
- BASICS turn-start FLAGS verarbeitet: disk /tmp 1308MB (kein Spawn noetig), events keine un-acked,
  reminders 98 unread (nur dieses Lane-Prompt relevant), o56 busy>40min und POOL/PLAN-Flags sind
  Coordinator-Themen, nicht Blocker fuer F27.
