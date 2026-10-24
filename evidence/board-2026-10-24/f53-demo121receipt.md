# F53 — Demo PR #121: bounded native PODMAN cleanup-preparation receipt

Mission: ops/missions/f53.txt
Lane: F53 (worker)
Date: 2026-10-24 (UTC timestamps per step)
Candidate: baseharbor-demo `f57c8b6e5ff39d0873df62b26272962765e88d6e` (tree `9a25907b7561d07c734022cf585ee43b06311c6a`, parent `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`) / Core `1861ed07888a1ec22c218ca491d23ee54f48e261`
Host: Ubuntu 26.04.1 LTS, kernel 7.0.0-38-generic

## PLAN

- [x] 0. Evidence file + native Todos created (this file)
- [x] 1. Read Codex recipe (codex-verdict-demo121-r3.md:46–50, "Exactly one minimal worker action") — data only
- [x] 2. Pruefpfad 1: Capability check — podman ABSENT on this host; docker present but not permitted/usable
- [x] 3. Pruefpfad 2: Repo mechanism — real Podman trigger found; FF check fails (8/2) → no push per mission rule
- [x] 4. Pruefpfad 3: skipped by rule ("wenn und nur wenn Schritt 1 podman bereitstellt" — it does not)
- [x] 5. Pruefpfad 4: Evidence in this file; no real run exists → PR #121 body untouched (per rule)
- [x] 6. Pruefpfad 5: 3-line recommendation below
- [x] 7. Finalize; outcome = precise Umgebungs-Befund → END BLOCKED

## Pruefpfad 1 — Capability (2026-10-24T11:2xZ)

```
$ command -v podman            -> (empty, rc=1)
$ podman --version             -> /bin/bash: line 1: podman: command not found
$ type -a podman               -> type: podman: not found
$ ls /usr/bin/podman /usr/local/bin/podman /opt/podman/bin/podman -> all "No such file or directory"
$ command -v docker            -> /usr/bin/docker ; Docker version 29.8.1, build 4a63305
$ docker info (server)         -> "permission denied ... unix:///var/run/docker.sock" (daemon not reachable from this lane)
$ podman info                  -> NOT POSSIBLE, binary absent (no rootless mode to evaluate)
```

**Podman is not installed on this worker host.** No mock, no docker substitution, no exception suppression (mission: "nicht tricksen").

## Pruefpfad 2 — Repo mechanism (runner path evaluated instead, per mission fallback)

- Trigger: `.github/workflows/v0424-targeted.yml:2–4` — `push` on branches `work/v0.4.24/25/26-ecosystem-release`; `:12` job `if` requires `[demo-runtime]` (or `[demo-podman-runtime]`/`[demo-docker-runtime]`) in head-commit message; `:14` podman leg runs on `ubuntu-26.04` (real GitHub-hosted podman); `:19` matrix.
- Receipt command integration: `scripts/acceptance.sh:80` (candidate blob `626aac60cbfc1347715e86efebc4f33afb6a4e79`) executes exactly the unmodified `bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup-preparation --unpause-owned` inside the workflow's backup-restore step (`v0424-targeted.yml:79–80`).
- Candidate's own limits (hardcoded, no change needed/possible): `scripts/failure-runtime-diagnostics.sh:41` `deadline = time.monotonic() + 60`; `:43` `def run(..., limit=20, ...)`. Bindings per Codex recipe at `:4–6` (DEMO_ROOT, ARTIFACT_DIR, BASEHARBOR_TARGET) and `:25–33` (BASEHARBOR_DEMO_FIXTURE, BASEHARBOR_TEST_RUNTIME∈{docker,podman}, fixture-scoped XDG_DATA_HOME/XDG_CONFIG_HOME).
- o122 path check: `git rev-list --left-right --count origin/work/v0.4.26-ecosystem-release...f57c8b6e5ff39d0873df62b26272962765e88d6e` → **`8 2`** ⇒ candidate is NOT a descendant of the branch tip ⇒ a candidate push would not be a clean fast-forward. **Mission rule applied: KEIN Push.** No synthetic child commit with the candidate tree was fabricated either — that would revert release-branch content (#120 lineage) and touch a shared branch; Codex r3:48 explicitly requests no "#120 integration".
- Tree identity of existing runs: `gh run list --branch work/v0.4.26-ecosystem-release` — all heads (25c31969/a0ff20ea/3776ad7d/e485725a/d35c3bdc/a0d0be8c...) differ from candidate; green run **39990941852** (success, head `25c319697fd3d89d894943e0f44cd3d5dde8b198`, tree `1a2e53aaf7f228a2dc865133696434c012e4ff20`) ran the release-branch diagnostic variant with **raised budgets**, proven by `git diff f57c8b6e origin/work/v0.4.26-ecosystem-release -- scripts/failure-runtime-diagnostics.sh`: `-deadline = time.monotonic() + 60` / `+deadline = time.monotonic() + (90 if engine == 'podman' else 60)` and per-call `20` → `60` for podman. That is exactly the "Podman-only timeout delta" (codex-verdict-demo121-r3.md:50) — so the green run does NOT substitute the candidate-limits receipt.

## Pruefpfad 3 — Sandbox receipt

Not executed. Condition "wenn und nur wenn Schritt 1 podman bereitstellt" is false on this host. No shared resources were touched; no fixture created or removed; no rm performed anywhere.

## Provenance (immutable, for later verification)

- Codex recipe: `evidence/board-2026-10-24/codex-verdict-demo121-r3.md` sha256 `010611586ecece71bbaf994d59e689d3a350111d636a6560d4b8a4f6e64c76bf`
- Candidate files (from git object store, `git show f57c8b6e...`):
  - `scripts/failure-runtime-diagnostics.sh` blob `ea897dfb184dc7f043869174f1e5753b7144f91c` sha256 `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f`
  - `scripts/acceptance.sh` blob `626aac60cbfc1347715e86efebc4f33afb6a4e79` sha256 `6f4f313fec93b36d84e0160a8541ad6123fdd4aa7e141a508fd3c522771f724a`
  - `.github/workflows/v0424-targeted.yml` blob `bc0b72570cbbd79f780c5e2d77658d56a7e9e887` sha256 `146e001a906714e5290d2fa91289733573e3572186f1aa6e7f4001ecee370926`
  - `baseharbor-core.ref` blob `2bae2e9ac10a4a1efe2b026531e0a41cbc9eeceb` content `1861ed07888a1ec22c218ca491d23ee54f48e261` ✓
- Git policy followed: `pwd -P` before every git command; all git commands only in `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo`; no merge/review command, no force, no --admin, no history rewrite, no branch reset, no push.

## Outcome

**No receipt produced; no real workflow run exists on the candidate tree.** The finding is the precise environment/mechanism report above (ABNAHME alternative per mission line 12):
1. Missing evidence: one podman-capable isolated runtime for the exact candidate tree at its hardcoded 60s/20s limits.
2. This host: no podman binary; docker present but irrelevant (mission forbids new Docker runs; socket also unreachable).
3. Runner route is blocked only by the fast-forward condition (`8 2`), not by missing trigger knowledge.

## Empfehlung an den Coordinator (3 lines)

1. Fehlender Nachweis: ein nativer podman `cleanup-preparation --unpause-owned`-Lauf auf exakt Baum `9a25907b` (60s/20s) mit scope-result.json und Roh-Timing — auf diesem Host unmöglich (kein podman-Binary).
2. Kleinste legitime Handlung: GitHub-Runner-Weg existiert bereits (workflow `:2–4,12,14,19`); er braucht einen sauberen FF-Push eines Kandidaten-Baum-Commits mit `[demo-runtime]` auf `work/v0.4.26-ecosystem-release` — dieser Lane darf ihn nicht bauen (FF-Check 8/2, geteilter Branch), also vom Maintainer/Integrations-Owner ausführen lassen.
3. Alternative: podman-Host (z.B. Worker-Host mit Rootless-Podman) für die isolierte Sandbox-Ausführung in 20–30 min, oder formale Akzeptanz der Pin-Kompatibilität (core.ref byte-identisch zum grünen Release-Tipp, Delta nur diagnose-intern) + späterer W1-Journey-Run.

END F53-DEMO121RECEIPT-BLOCKED
