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

## Qualification limits

Podman measurements, isolated placement and further host/topology calibration
remain pending. Source tests and a provider count cannot supply missing figures.
Host preflight continues to distinguish planning estimates from unavailable
measurements for the selected installation.
