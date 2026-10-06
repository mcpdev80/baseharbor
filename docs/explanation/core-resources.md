# Core resource observations

Core capabilities are mandatory; provider placement may be shared or
application-isolated. Additional isolation can add provider instances and
resource consumption. The optional Console and application workloads are outside
the Core totals below.

## Measured Docker reference topology

The [rootless Docker bootstrap qualification](https://github.com/mcpdev80/baseharbor/actions/runs/37492547982)
passed for public Core `b25c4e25f0244648cc6a7641828e238005925933` on
2026-10-06. Both rows measure PostgreSQL + OpenBao + Keycloak, including the
running provider dependencies, gateways and administrative helpers.

| Machine-role default | Stabilized idle total, median | Observed idle range | Sampled startup/convergence peak |
| --- | ---: | ---: | ---: |
| Development | 2.62 GiB | 2.60–2.62 GiB | 3.79 GiB |
| Deployment | 2.57 GiB | 2.56–2.58 GiB | 4.73 GiB |

These are **installation-shared observations**, not universal minimums or
application-isolated measurements. The two roles use the same provider topology;
their differing observations do not establish a role-specific memory budget.

This existing reference realization has one SQL member and one OpenBao member,
plus three Keycloak members backed by three PostgreSQL members and three etcd
members. The retained idle inventory contains 17 running containers. Three
mandatory capabilities therefore do not imply three containers. These figures
must not be presented as measurements of a single-Keycloak realization.

Images include PostgreSQL 18, OpenBao 2.7.0 and Keycloak 26.8.0. Original JSON
logs retain exact image IDs, per-service and per-capability memory, simultaneous
Core totals, host memory/swap/PSI and timestamps. Provider memory limits in this
run were unset. Container memory uses native Docker/cache accounting; it is not
host RSS or a guaranteed allocation requirement.

Sampling targets a three-second interval. Each idle window contains eleven
complete observations spanning at least 30 seconds, with total variation within
10%. Startup peaks are sampled observations and can miss shorter peaks.

## Measured Podman reference topology

The [rootless Podman bootstrap qualification](https://github.com/mcpdev80/baseharbor/actions/runs/37498498737)
passed for Core `55f7b1c4c72e57c5858a3476c097ca6e244c8d5d` on 2026-10-06.
It uses the same installation-shared 17-container reference realization.

| Machine-role default | Stabilized idle total, median | Observed idle range | Sampled startup/convergence peak |
| --- | ---: | ---: | ---: |
| Development | 2.51 GiB | 2.51–2.65 GiB | 4.65 GiB |
| Deployment | 2.56 GiB | 2.55–2.60 GiB | 4.59 GiB |

These figures come from eleven complete native observations per idle window.
Runtime-specific memory/cache accounting prevents treating differences between
Docker and Podman as a performance comparison. Original JSON and image identities
remain in the qualification artifact; application isolation is not measured.

## Qualification limits

Isolated placement and further host/topology calibration remain pending. Source tests and a provider count cannot supply missing figures.
Host preflight continues to distinguish planning estimates from unavailable
measurements for the selected installation.

Core preflight uses the largest observed Identity startup sample (4,976,065,638
bytes, including its dependencies) as a reference planning estimate on other
hosts. It adds the existing topology-aware SQL/Secrets startup budgets. This
estimate is explicitly `ESTIMATED`; it establishes no reliable minimum and does
not claim that the selected host or Podman was measured. Reused owned capabilities
are not budgeted as additional instances.
