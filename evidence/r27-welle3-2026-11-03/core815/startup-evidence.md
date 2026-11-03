# core-815 — Greenfield stack startup + READY evidence (Docker, current candidate)

**Task:** core-815 (Tests: start all greenfield stacks on native runtimes)
**Lane:** w35 (coord f49a3303) — release v0.4.27
**Candidate SHA (develop HEAD at run):** `1706272c` (r27 authority-contract: installation authority)
**Binary built from:** `go build -buildvcs=false -o baha ./cmd/baha` in worktree `work/r27-core815`
**Runtime:** rootless Docker (context `rootless`, socket `/run/user/1000/docker.sock`), Docker Engine 29.8.1
**Reference (existing, not sufficient per card):** PR #809 / v0.4.23 delivered the Go/Next.js/Quarkus build corrections (merged). Native template run 37508074031 at `9865135c613c2c5cb73f88f95c60a65685ee6227` passed fresh image builds for all four stacks and actual Python startup, but did not qualify startup of all four stacks. This run qualifies startup + READY for all four.

## Method (authoritative pattern = repo's own nextjs-greenfield-e2e.yml + go adapter test)
1. `baha app new <name> --directory <dir> --stack <stack> --http --sql --require-secret APP_SECRET`
2. `docker compose up --build --wait` (native image build + container start)
3. HTTP GET on the app health endpoint → must be `200` (READY)
4. `docker compose down --remove-orphans --volumes` (clean container rooms)

The generated compose `expose`s the app port internally (not published), so the external
readiness probe runs inside the container via `docker compose exec`. Probe helper:
`python - "<EP>" < probe.py` (Go/Python slim images); `wget -q -O - "<EP>"` (Next.js/Quarkus images).

## Per-stack evidence

### Go — READY (HTTP 200 at /healthz)
- Command: `baha app new go-app --directory /tmp/opencode/w35-native/go --stack go --http --sql --require-secret APP_SECRET`
- Output: `created go-app (dev) with generated/go` (+ go.mod, main.go, Dockerfile, compose.yaml, .env.example, baseharbor.yaml, baseharbor.repository.yaml)
- `docker compose up --build --wait`: built `wf-native-go-app`, container `wf-native-go-app-1` → `Healthy`
- Probe (inside container): `curl/wget http://127.0.0.1:8080/healthz` → `raw: ok`, `status: HTTP 200`
- `docker compose ps`: `wf-native-go-app-1 … Up … (healthy) 8080/tcp`
- Teardown: `docker compose down --remove-orphans --volumes` → networks removed
- RESULT: `STACK=go RUNTIME=docker RESULT=READY_HTTP200 EP=http://127.0.0.1:8080/healthz`

### Next.js — READY (HTTP 200 at /healthz)
- Command: `baha app new nextjs-app --directory /tmp/opencode/w35-native/nextjs --stack nextjs --http --sql --require-secret APP_SECRET`
- Output: `created nextjs-app (dev) with generated/nextjs`
- `docker compose up --build --wait`: built `wf-native-nextjs-app`, container → `Healthy`
- Probe (inside container): `wget -q -O - http://127.0.0.1:8080/healthz` → `raw: {"status":"ok"}`, `status: HTTP 200`
- `docker compose ps`: `wf-native-nextjs-app-1 … Up … (healthy) 0.0.0.0:8080->8080/tcp` (compose publishes port)
- Teardown: `docker compose down --remove-orphans --volumes`
- RESULT: `STACK=nextjs RUNTIME=docker RESULT=READY_HTTP200 EP=http://127.0.0.1:8080/healthz`

### Python — READY (HTTP 200 at /healthz)
- Command: `baha app new python-app --directory /tmp/opencode/w35-native/python --stack python --http --sql --require-secret APP_SECRET`
- Output: `created python-app (dev) with generated/python`
- `docker compose up --build --wait`: built `wf-native-python-app`, container → `Healthy`
- Probe (inside container): `python - "http://127.0.0.1:8080/healthz" < probe.py` → `raw: 200`, `status: HTTP 200`
- `docker compose ps`: `wf-native-python-app-1 … Up … (healthy) 8080/tcp`
- Teardown: `docker compose down --remove-orphans --volumes`
- RESULT: `STACK=python RUNTIME=docker RESULT=READY_HTTP200 EP=http://127.0.0.1:8080/healthz`

### Quarkus — READY (HTTP 200 at /q/health)
- Command: `baha app new quarkus-app --directory /tmp/opencode/w35-native/quarkus --stack quarkus --http --sql --require-secret APP_SECRET`
- Output: `created quarkus-app (dev) with generated/quarkus`
- `docker compose build` + `docker compose up -d` (Quarkus generated compose has no healthcheck, so `--wait` is not used): built `wf-native-quarkus-app`
- Probe (inside container): `wget -q -O - http://127.0.0.1:8080/q/health` → `raw: {"status":"UP","checks":[]}`, `status: HTTP 200`
- `docker compose ps`: `wf-native-quarkus-app-1 … Up 0.0.0.0:8080->8080/tcp`
- Teardown: `docker compose down --remove-orphans --volumes`
- RESULT: `STACK=quarkus RUNTIME=docker RESULT=READY_HTTP200 EP=http://127.0.0.1:8080/q/health`

## Acceptance mapping
- Go fresh scaffold builds and reaches READY without manual `go mod tidy` → PASS (build ran clean in Dockerfile, no tidy needed).
- Next.js builds with bindings resolved at runtime, not build import time → PASS (/healthz 200, no build-time secret required).
- Quarkus scaffold produces the artifact layout its Dockerfile consumes (`target/quarkus-app/quarkus-run.jar`) → PASS (build + health 200).
- Python remains green → PASS (/healthz 200).
- Focused Docker scaffold acceptance, including READY for all four stacks → PASS.

## Write-set compliance
- Only added files under `evidence/r27-welle3-2026-11-03/core815/startup-evidence.md`.
- No edits to `.github/scripts/` or any CI gate file (grep-clean). The runner scripts (`scripts/run-native.sh`, `scripts/probe.py`, `scripts/run-startup.sh`) are local dev helpers in the worktree and are NOT added to the repo; the `baha` binary is a build artifact, not committed.
- No product source changes: evidence is generated-application startup/READY on the current candidate, no fix required.

## Repo-rule compliance
- Branch: `work/r27-core815` off `origin/develop` (`1706272c`).
- Merge `origin/develop` + ancestry `rc=0` + `ls-remote`==head before push.
- Push own branch only; PR DRAFT.
- Evidence-only change (no product files) → no product full-CI trigger.

## END
END lane DONE w35-greenfield-startup: core-815 all four greenfield stacks READY on Docker (candidate 1706272c) #1706272c
