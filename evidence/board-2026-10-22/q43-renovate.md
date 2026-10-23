# q43-renovate: Renovate-Update-Inventar v0.4.27

## PLAN

1. Read task t-20261017-release-0-4-27-updates.md (Claim q43)
2. basics-check.py turn-start (from AGENTS.md)
3. Scan all 4 product repos for Renovate PRs and dependency dashboards
4. Classify by risk: deps-major / deps-minor / deps-patch / lock-only / docker-base / actions / toolchain
5. Derive recommended integration order per repo
6. Report evidence only - no merges/closes
7. END Q43-RENOVATE-DONE

## Status

[ wip:q43 ]

## Evidence

### Step 0: basics-check.py turn-start

Result: `BASICS: FLAGS` - 9 unread reminders, 1 idle lane, pool under target, plan untouched 125min, 16 open steps.

---

### Step 1: baseharbor — Renovate-Update-Inventar

**Quelle:** GitHub Issue #889 "Dependency Dashboard" (mcpdev80/baseharbor)

#### Status: ERRORED

Renovate encountered a branch update failure: `⚠️ WARN: Error updating branch: update failure`.

The grouped update `chore(deps): update dependencies` (retry-branch=renovate/all-dependencies) covers:
- 25+ Go modules (`go.mod`, `go 1.26.9`, charm.land, github.com, golang.org/x, go.mongodb.org, rabbitmq/amqp091-go)
- 50+ GitHub Actions (actions/checkout, actions/setup-go, actions/setup-python, actions/upload-artifact, actions/download-artifact, actions/deploy-pages, actions/configure-pages, actions/upload-pages-artifact, docker/setup-*, docker/login-action, docker/metadata-action, docker/build-push-action, renovatebot/github-action, goreleaser/goreleaser-action, ubuntu, go)
- 80+ Docker images via regex/quadlet/compose (postgres, haproxy, openbao, alpine, caddy, mongo, valkey, spilo, keycloak, loki, alloy, prometheus, seaweedfs, redisinsight, redpanda, tempo, otel)
- 1 Python pip (mkdocs-material/mkdocs)

**Keine offenen Renovate-PRs in baseharbor** -- alle PRs mit `renovate/` prefix sind MERGED oder CLOSED. Die einzige aktive Renovate-Aktivitaet ist das Error-gepraagte Dependency Dashboard #889.

#### baseharbor: Risk-Classification (unique deps only)

| Risk Class | Updates | Priority |
|---|---|---|
| **docker-base** | `alpine 3.24@sha256`, `ghcr.io/mcpdev80/baseharbor-runtime edge` | HOECHST -- lock-only, SHA-pinned, build-Infra |
| **deps-major** | `go 1.26.9 -> 1.27.2`, `python 3.14 -> 3.15`, `golang.org/x/crypto v0.55.0 -> v0.57.0`, `golang.org/x/sys v0.47.0 -> v0.49.0` | HOCH -- Go runtime + stdlib |
| **deps-major** | `mongo 7.0.43 -> 9.0.4`, `keycloak 26.7.0 -> 26.8.1`, `keycloak 26.8.0 -> 26.8.1` | HOCH -- major/minor image bumps |
| **deps-minor** | `caddy 2.11.4-alpine -> 2.11.6-alpine`, `haproxy 3.2.23-alpine -> 3.4.7-alpine`, `spilo 4.1-p2`, `valkey 8 -> 9`, `seaweedfs 4.47 -> 4.48`, `loki 3.7.8 -> 3.7.9`, `alloy v1.19.2 -> v1.20.1`, `prometheus v3.14.0 -> v3.15.0`, `redisinsight 3.8.0`, `redpanda v26.2.3 -> v26.2.4`, `tempo 3.1.0 -> 3.1.1`, `otel 0.161.0 -> 0.163.2`, `etcd v3.7.2 -> v3.7.3`, `rabbitmq 4.3.6 -> 4.4.0`, `mongo-driver/v2 v2.9.1 -> v2.9.2`, `swgui v1.8.5 -> v1.8.9`, `bubbletea/v2 v2.0.9 -> v2.1.0`, `amqp091-go v1.15.0 -> v1.16.0` | MITTEL -- minor bumps, no API changes implied |
| **actions** | `actions/checkout v4 -> v7`, `actions/checkout v7 -> v7.0.1`, `actions/setup-go v6 -> v7`, `actions/setup-python v5 -> v7`, `actions/upload-artifact v4 -> v7.0.2`, `actions/upload-artifact v6 -> v7`, `actions/download-artifact v4.3.0 -> v8.0.2`, `actions/download-artifact v6 -> v8`, `actions/deploy-pages v5`, `actions/configure-pages v6`, `actions/upload-pages-artifact v3 -> v5`, `docker/setup-qemu-action v3 -> v4`, `docker/setup-buildx-action v3 -> v4`, `docker/login-action v3 -> v4`, `docker/metadata-action v5 -> v6`, `docker/build-push-action v6 -> v7`, `renovatebot/github-action v46.3.6 -> v46.3.7`, `goreleaser golang.org/x/crypto v0.55.0 -> v0.57.0` | MITTEL -- CI toolchain, lockfile-only in most cases |
| **toolchain** | `goreleaser ~> v2` (unchanged), `ubuntu 26.04` (unchanged), `python 3.12 -> 3.14` | MITTEL |

#### baseharbor: Empfohlene Integrationsreihenfolge

1. **docker-base / lock-only** -- `alpine 3.24@sha256`, `baseharbor-runtime edge` -- kein Versionsbump, nur lockfile-consistency, risikoarm
2. **deps-minor (Docker images)** -- `caddy 2.11.6`, `haproxy 3.4.7`, `valkey 9`, `seaweedfs 4.48`, `loki 3.7.9`, `alloy v1.20.1`, `prometheus v3.15.0`, `redpanda v26.2.4`, `tempo 3.1.1`, `otel 0.163.2`, `etcd v3.7.3`, `spilo 4.1-p2`, `rabbitmq 4.4.0`, `mongo-driver v2.9.2`, `swgui v1.8.9`, `bubbletea v2.1.0`, `amqp091-go v1.16.0` -- minor version bumps, low API risk
3. **actions** -- `actions/checkout v7/v7.0.1`, `actions/setup-go v7`, `actions/setup-python v7`, `actions/upload-artifact v7`, `actions/download-artifact v8` -- CI toolchain, well-tested, lockfile-heavy
4. **deps-major** -- `go 1.26.9 -> 1.27.2`, `python 3.14 -> 3.15`, `golang.org/x/crypto v0.57.0`, `golang.org/x/sys v0.49.0` -- Go runtime, requires test verification
5. **deps-major (Docker images)** -- `mongo 7.0.43 -> 9.0.4` (major), `keycloak 26.8.1` (minor) -- higher risk, needs compatibility checks
6. **toolchain** -- `ubuntu 26.04`, `goreleaser v2`, `python 3.12 -> 3.14` -- infra changes, verify build matrix

---

### Step 2: baseharbor-demo -- Renovate-Update-Inventar

**Pruefung:** `gh pr list --repo mcpdev80/baseharbor-demo --state all --search "renovate"` -> 0 Ergebnisse.
**Pruefung:** `gh issue list --repo mcpdev80/baseharbor-demo --state all --search "Dependency"` -> keine Dependency Dashboard Issues.

**Ergebnis:** baseharbor-demo hat KEINE offenen Renovate-PRs und KEINE Dependency-Dashboard-Eintraege. Alle 118 PRs sind feature/test/ci/docs bezogen.

**Empfohlene Integrationsreihenfolge:** N/A -- keine Renovate-Aktivitaet vorhanden.

---

### Step 3: baseharbor-console -- Renovate-Update-Inventar

**Pruefung:** `gh pr list --repo mcpdev80/baseharbor-console --state all --search "renovate"` -> 0 Ergebnisse.
**Pruefung:** `gh issue list --repo mcpdev80/baseharbor-console --state all --search "Dependency"` -> keine Dependency Dashboard Issues.

**Ergebnis:** baseharbor-console hat KEINE offenen Renovate-PRs und KEINE Dependency-Dashboard-Eintraege. Alle 7 PRs sind feature/test/docs/consumption bezogen.

**Empfohlene Integrationsreihenfolge:** N/A -- keine Renovate-Aktivitaet vorhanden.

---

### Step 4: baseharbor-node-connector -- Renovate-Update-Inventar

**Pruefung:** `gh pr list --repo mcpdev80/baseharbor-node-connector --state all --search "renovate"` -> 0 Ergebnisse.
**Pruefung:** `gh issue list --repo mcpdev80/baseharbor-node-connector --state all --search "Dependency"` -> keine Dependency Dashboard Issues.

**Ergebnis:** baseharbor-node-connector hat KEINE offenen Renovate-PRs und KEINE Dependency-Dashboard-Eintraege. Alle 8 PRs sind feature/test/docs/consumption bezogen.

**Empfohlene Integrationsreihenfolge:** N/A -- keine Renovate-Aktivitaet vorhanden.

---

### END Q43-RENOVATE-DONE
