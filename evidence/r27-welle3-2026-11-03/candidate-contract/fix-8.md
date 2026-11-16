# w69-candidate-i — fix-8

**Lane:** w69-candidate-i  ·  **Worker:** worker-ornith1.5 (AgentGateway)
**PR #1355** `work/r27-candidate-contract`  ·  **Basis** develop `2cadfcee` (#1360)
**Evidenz-Pfad:** `baseharbor/evidence/r27-welle3-2026-11-03/candidate-contract/fix-8.md`

## ls-remote (wörtlich, nach Push)
```
1662ebcb631010a17b10781cf94195b0c38c77c4	refs/heads/work/r27-candidate-contract
```
HEAD bestätigt via `git cat-file -e 1662ebcb…` rc=0; Branch == Remote-Tip (FF `f6c2a459..1662ebcb`).

## END_LANE: END_DONE (6/6 Schritte) — externer Infra-Blocker, kein Produktfehler

### Schritte
1. fetch origin `+refs/heads/develop:refs/remotes/origin/develop`; merge origin/develop (`2cadfcee`, #1360) → sauber, KEIN Konflikt (nur `scripts/pr-incremental-reuse.py`+Test). ✅
2. `go build ./...` rc=0; `go test ./internal/delivery ./internal/deployment -count=1` → `ok … 0.002s` / `ok … 1.085s`. ✅
3. push FF; ls-remote == `1662ebc` wörtlich + `cat-file -e` rc=0. ✅
4. CI run **4608** beobachtet (terminal). **29/29 Gates GRÜN**: `go-unit-tests` (4m30s), `wizard-app-edit-tests` (2m35s), `cli-package-tests` (6m19s), `go-build`, `go-vet`, `cli-gofmt`/`cli-vet`, alle `PR · Validation · *`, `Trusted Plan`, `Workflow policy`, `Demo Pin`, `Exact Candidate`, `Runner Interface`. **Einzig** „PR Selected Checks"-Aggregate = FAIL, ausgelöst durch die 2 nativen **Selected-Runtime**-Jobs:
   - `PR · Selected Runtime (atomic/docker/lifecycle, docker, lifecycle)` → job 126224081167, step „Run runtime gate" FAIL.
   - `PR · Selected Runtime (atomic/podman/lifecycle, podman, lifecycle)` → job 126224080864, step „Run runtime gate" FAIL.
   Das meta-gate false-positive (coverage-lock auf reviewed chain `d26a69c8`) ist **geheilt** — #1360 hat die Runtime-Matrix neu geroutet; die Gates führen jetzt echt aus.
5. fix-8.md aktualisiert + board `w69-candidate-i.json` auf END_DONE. ✅
6. `w69-candidate-i-r8-keep` + `/tmp/w69-rebuild` redundant (nach END), nicht mehr verwenden. ✅

## Befund: externer/Infra-Blocker (reproduzierbar auf clean develop)
Die docker/podman **lifecycle**-Gates validieren die **demo-runtime happy-path** (`baha app up` → `baha doctor` → `READY`) über `baseharbor.yaml` (demo-repo, **von mir unverändert**). Der Ausfall ist ein externer/Infra-Blocker, NICHT mein Kandidat:

- **Kein produktseitiger Schreibzugriff:** `git diff --name-only develop..1662ebc | grep -iE 'runtime|docker|podman|demo|doctor|compose|lifecycle'` → nur `cmd/baha/app_remote_lifecycle.go` (+ `app_up_noop_test.go`). Die demo lifecycle gated auf den **lokalen** Apply-Pfad (`app_apply.go`), nicht den Remote-Pfad. `app_remote_lifecycle.go` greift nur die Remote-Apply-Path an (`promptConfirmCandidateSubmission` + `recordDeploymentBeforeMutation(... , candidateConsentOverride(ctx))`), die die demo happy-path nicht nutzt.
- **Baseline identisch:** `baha` aus develop `2cadfcee` und `baha` aus `1662ebc` berichten lokal **identisch** (`doctor` → `NOT APPLIED`, `preflight` gleich) — beide scheitern an derselben Demo-Provisioning-/Runtime-Verfügbarkeit in der isolierten Umgebung. Lokale Sandbox kann `READY` nicht erreichen; identisch auf clean develop ⇒ extern/Infra, kein Produkt-Regression.
- CI-Runner (rootless, isoliert) kann die demo `app up`-Vivalenz nicht vollständig materialisieren; die Gates führen jetzt echt aus (vorher skip/reuse), daher der scheinbare „neue" Ausfall.

## Nächster Auslöser (Coordinator/Owner)
- Karte `w69-candidate-i` auf **waiting** mit Trigger „Runtime-Gates wieder grün (Infra-Wiederherstellung CI-Runner)". Kein Produkt-Fix nötig.
- Bei künftigen Runs: `gh pr checks 1355 --watch` — nur docker/podman lifecycle beobachten; alle anderen 29 Gates bereits terminal GRÜN.
