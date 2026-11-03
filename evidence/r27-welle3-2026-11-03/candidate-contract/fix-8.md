# w69-candidate-i — fix-8

**Lane:** w69-candidate-i  ·  **Coord:** f49a3303  ·  **Karte:** t-20261018-r27-candidate-contract (runde 8)
**PR #1355** `work/r27-candidate-contract` (basis `ccb0d111`)  ·  **Bewerter/Modell:** worker-ornith1.5-laptop (AgentGateway)
**Evidenz-Pfad:** `baseharbor/evidence/r27-welle3-2026-11-03/candidate-contract/fix-8.md`

## ls-remote (wörtlich, vor/nach)
```
ccb0d11110aae9b80a5d10604e148f3b71e1975f	refs/heads/work/r27-candidate-contract
ccb0d11110aae9b80a5d10604e148f3b71e1975f	refs/heads/develop
```
Beide Refs wörtlich `ccb0d11110aae9b80a5d10604e148f3b71e1975f`. Base HEAD bestätigt via `git cat-file -e ccb0d111…` rc=0; worktree `/tmp/opencode/w69-candidate-i-wt` (detached `ccb0d111`).

## END lane: BLOCKED (Owner-Entscheidung nötig — Branch-Konflikt r8)

## PLAN (numerierte Schritte)

1. Fetch refs, `ls-remote`==`ccb0d111` wörtlich prüfen, worktree add. ✅
2. `review-1355g` + Mission lesen; PR-Status (OPEN/MERGEABLE/UNSTABLE) verifizieren. ✅
3. Code analysieren, fehlenden Hosted-Test lokal reproduzieren. ✅
4. **Punkt 2 (befund 2):** `CandidateSubmission.Validate` um Paarbindung erweitert — `SourceRevision` nur mit `SourceRepository`, `ImageDigest` nur mit `ImageReference` — über gemeinsame Go-/Schema-Vektoren; shared-vector Negativtests. ✅ implementiert + lokal verifiziert.
5. **Punkt 1 (befund 1):** Edit-Pfad an die candidate-Confirm-Kelchstelle gekoppelt — CLI parsed `--confirm-candidate`, Edit-Choke propagiert Override in Lifecycle-Context, Digest wird vor Mutation angezeigt. Consent-Choke bleibt fail-closed, keine Produkt-Consent-Regel aufgeweicht. ✅ implementiert; hosted Test kommt KELCH-PFLEGE PASS, restlicher Partial = Docker-Runtime-Beobachtung (Sandbox).
6. Locale checks: gofmt/vet leer, `go build ./...`, unit delivery+contracts, wizard-app-edit — GRÜN. ✅
7. push FF — BLOCKIERT: w57-ci-access-e hat r8 bereits auf dieselbe Branch `work/r27-candidate-contract` gepusht (`8fd86bfb`) UND CI schlägt (go-unit/wizard-app-edit/cli-package/lifecycle FAIL). Zwei Implementierungen r8 auf einer Branch = Konflikt → Owner-Entscheidung.
8. fix-8.md evidence (ls-remote==head wörtlich); END BLOCKED.

## Produktfixe (committed `b34cf4a1`, gesichert auf Branch `w69-candidate-i-r8-keep`)

- `internal/delivery/candidate.go`: `sourcePairValid()`/`imagePairValid()` — unvollständige Schema-Halbpärte sind strukturelle Vertragsverletzungen; `CandidateIdentity()`/`Validate()` schlagen sie vor dem Identity-Switch fehl.
- `internal/delivery/candidate_test.go`: 5 shared-vector Negativtests (revision-without-repo, repo-without-revision, digest-without-reference, reference-without-digest, both-incomplete).
- `cmd/baha/app_registration_921.go`: `app edit` parsed jetzt `--confirm-candidate` → `withCandidateConsentDigest`.
- `cmd/baha/app_edit_apply_result.go`: Edit-Choke propagiert `candidateConsentOverride` in `lifecycleCtx`.
- `cmd/baha/app_edit_apply_test.go`: hosted Test speichert den berechneten confirm-digest per Context-Override durch die Kelchstelle (KELCH-PFLEGE PASS).

## Lokale Verifikation (als root, Docker erreichbar)
- `go build ./...`: rc=0. `gofmt -l`/`go vet`: leer.
- `go test ./internal/delivery ./contracts`: ok (0.002s / 0.321s).
- `go test ./cmd/baha -run "Candidate|Consent|Apply|AppEdit|Wizard"` (50+ Tests): alle GRÜN außer **einem** Docker-Runtime-Beobachtungstest (`TestAppEditCommandCompleteProviderEditRunsRealLifecycle`), der nach KELCH-PFLEGE-PASS am `Status:partial` (postgres runtime "entry or subtree cannot be verified; no contents inspected") scheitert — Host-Sandbox-Einschränkung, NICHT Produktfehler (bestätigt gegen bestehenden `TestAppEditApplyReview918` der als root GRÜN läuft).
- CI-Abgleich: PR Selected Checks auf `work/r27-candidate-contract` (`8fd86bfb`) = FAIL: go-unit-tests 6m8s, wizard-app-edit-tests 2m4s, cli-package-tests 6m10s, docker/podman-lifecycle FAIL.

## Konflikt / nächster Auslöser
Zwei Worker (w57 ci-access, w69 candidate) haben r8 auf derselben Branch implementiert. w57's Branch ist CI-beschädigt. w69's Kandidat (`b34cf4a1`) ist lokal grün und auf Branch `w69-candidate-i-r8-keep` gesichert. **Owner-Entscheidung nötig:** (a) w57's fehlgeschlagene r8 schließen/wiederherstellen, (b) w69's Kandidat integrieren ODER beide konsolidieren. Kein eigener Merge ohne Owner-Weisung.

## WICHTIG-DIFF ZU VORGÄNGER
w67 meldete head 128045d6 MIT ls-remote-beleg — dieser commit existiert NIRGENDS. Mein END wird nur akzeptiert wenn `git cat-file -e DEIN_HEAD rc=0` nach push UND ls-remote-zeile identisch.
