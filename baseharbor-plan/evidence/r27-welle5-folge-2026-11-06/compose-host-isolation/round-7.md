# Round 7 evidence — w282 compose-host-isolation verify (card t-20261105-r27-compose-host-isolation)

## Summary

Round-7 verification at the refreshed merge head `f236b6610b885747f13ae12e148481c22ec368fb`
(PR #1383 `work/r27-compose-isolation`, develop-merged). Owner (2026-11-07) directed
merge develop into `work/r27-compose-isolation` and run the full isolation matrix.

Result: the isolation gate that previously blocked the four InCI lifecycle tests
(bootstrap Core-identity readiness timeout at `keycloak_driver.go:664`, 4-min window
over the test's 2/3-min context) is NOW GONE. The four tests now get PAST the
bootstrap/trust-plane gate and fail LATER, at a NEW genuine seam:
`internal/deployment/deployment_store.go:107` `ErrDeploymentOwnerMissing`
(`coreinstallation.Load` returns `ErrNotExist` — no Core installation owns the
per-test isolated target's data root). This is the failure the isolation refactor
was meant to surface. Per Owner 2026-11-07 ruling the defect is OUT of this card's
scope (moved to a different chain:
`t-20261106-r27-remote-up-podman-owner-missing`, lane w286); this card reports the
four tests honestly RED and ends DONE.

## Base attestation (provenance)

- Branch under verify: `work/r27-compose-isolation` (PR #1383). Same branch/PR as
  rounds 1-6. No second branch invented.
- Exact head BEFORE work (old ls-remote):
  `git ls-remote origin work/r27-compose-isolation` ==
  `b11eec12f90f50b76ba0854250395a7d25d37069` (verbatim).
- develop BEFORE work (old ls-remote):
  `git ls-remote origin develop` ==
  `f3db942859c68e78fe2e7c0a339be7eb0e380acb` (verbatim).
- NEW head AFTER merge (new ls-remote):
  `git ls-remote origin work/r27-compose-isolation` ==
  `f236b6610b885747f13ae12e148481c22ec368fb` (verbatim).
- Merge parents (canonical, from the merge commit itself):
  `git log --format='%p' -1 f236b661` == `b11eec12 8bda98c1`.
  - First parent `b11eec12f90` == the PR #1383 commit (its single parent is
    `5e4fde71`, the image-transfer PR #1402 — same lineage as develop).
  - Second parent `8bda98c1cd34a984810a0a0ee2d9fe4085b3933a` == PR #1397
    `Identity: invite managed users via native provider admin adapter`. This is a
    LINEAR develop commit on the exact same lineage
    (`5e4fde71 -> 78e78e66 -> 46ea6c73 -> ... -> 8bda98c1`) as the compose branch's
    own parent; NOT a foreign r27 candidate. It was develop's HEAD at the time the
    merge was made.
- develop ancestry: `8bda98c1` IS a strict ancestor of current develop
  `f3db942859c` (develop moved forward via #1404 after this merge; `8bda98c1` is
  #1397, `f3db942859c` is #1397+#1404). So develop IS an ancestor of the merge
  through `8bda98c1`. The merge is develop-faithful.
- All 7 compose-chain commits are ancestors of the merge head via the first-parent
  chain; PR-inhalt (4 files) fully present:
  `cmd/baha/app_repository_integration_test.go`,
  `cmd/baha/app_workload_integration_test.go`,
  `cmd/baha/repository_runtime_fixture_test.go`,
  `cmd/baha/runtime_integration_test_support_test.go` (all PRESENT at `f236b66`).
- The 6 named intermediate chain commits (`3b2ec108, 3fcba08f, 789ae146, 9bf29c89,
  bb25f152, 5246bd9d`) are NOT in develop and are NOT to be "rescued" — they are
  the PR #1383 branch's pre-`b11eec12` commits whose net effect is already captured
  in `b11eec12`. They correctly fall behind. NO delete/ls-remote call was issued to
  any foreign branch (coord-delete-list ignored: no foreign write-access by a lane).
- Worktree: isolated clone of branch `work/r27-compose-isolation` at
  `/home/marcel/dev/baseharbor-ecosystem/workers/.w282-compose-wt` (worker zone).
  Source tarball `/tmp/w282-src.tgz` transferred to the named SSH hosts only; no
  scratch under the ecosystem product root.
- No `.github/` edit, no workflow_dispatch, no rerun (Owner field). No subagents.
- No main-branch write in a PRODUCT repo; evidence lives only in baseharbor-plan.

## Quarantaene-tauschung (why the pre-merge provenance note looked wrong)

The pre-merge git worktree `workers/baseharbor` is a QUARANTINED checkout (ref
blind, push URL poisoned after a kraft-push incident; Owner + mission: tabu zone).
Its local ref/ancestry reading (`merge-base` showed the compose chain as
NOT-ancestor, first-parent chain ambiguous) was a QUARANTINE ARTIFACT, not reality.
Canonical measurement is fresh `git ls-remote` + log from the isolated
`.w282-compose-wt` worktree. The merge's canonical 2-parent chain
`[b11eec12, 8bda98c1]` proves the PR is a single commit on a develop line; nothing
was hidden, nothing to rescue. `workers/baseharbor` is NOT used for reality checks
in ANY lane from now on (Owner directive).

## Local verification (this lane, head f236b66)

- Build (docker host): `go build ./cmd/baha` -> exit 0.
- Vet: `go vet ./cmd/baha` -> exit 0.
- gofmt: `gofmt -l cmd/baha/{app_repository_integration_test.go,app_workload_integration_test.go,repository_runtime_fixture_test.go,runtime_integration_test_support_test.go}` -> empty (clean).
- Same clean build/vet/gofmt on the podman host.
- The four InCI tests are gated on `BASEHARBOR_CI_RUNTIME_INTEGRATION=1` and
  `BASEHARBOR_TEST_RUNTIME` (docker|podman); run on the named hosts only.

## GATE (1) docker matrix (baha-docker-2, 29.9.3, claimed w282)

Host prior to claim: 12 foreign leftover containers (w266dn/w-local remnants),
documented, NOT deleted (foreign ownership). No w282 containers were running.

Run (`BASEHARBOR_TEST_RUNTIME=docker`), verbatim tail:

```
=== RUN   TestRepositoryWorkflowRealLifecycle
    app_repository_integration_test.go:34: isolated runtime resolved ... target="w169-repository-workflow-...-iso" runtime="docker"
    app_repository_integration_test.go:60: repository apply failed: missing deployment state for w169-repository-workflow-...-iso/repo-e2e/dev [...]: deployment has no authoritative installation owner for its Target; the installation must be restored before its state is loadable
--- FAIL: TestRepositoryWorkflowRealLifecycle (113.68s)
=== RUN   TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI
    app_workload_integration_test.go:105: ... deployment has no authoritative installation owner ...
--- FAIL: TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI (0.18s)
=== RUN   TestRepositoryComposeWorkloadOnlyLifecycleInCI
    app_workload_integration_test.go:245: ... deployment has no authoritative installation owner ...
--- FAIL: TestRepositoryComposeWorkloadOnlyLifecycleInCI (0.62s)
=== RUN   TestRepositoryBuildWorkloadRebuildsSourceChangesInCI
    app_workload_integration_test.go:405: ... deployment has no authoritative installation owner ...
--- FAIL: TestRepositoryBuildWorkloadRebuildsSourceChangesInCI (0.60s)
FAIL
	github.com/mcpdev80/baseharbor/cmd/baha	97.059s
```

Observation: the bootstrap Core-identity readiness gate (rounds 1-6: "Core
identity failed: context deadline exceeded") is GONE — the tests now reach the
deployment-owner seam. All four fail with the SAME new `ErrDeploymentOwnerMissing`
at `internal/deployment/deployment_store.go:107`.

## GATE (2) podman matrix (baha-podman, 5.7.0, claimed w282)

Run (`BASEHARBOR_TEST_RUNTIME=podman`), verbatim tail:

```
=== RUN   TestRepositoryWorkflowRealLifecycle
    app_repository_integration_test.go:34: ... target="w169-repository-workflow-...-iso" runtime="podman"
    app_repository_integration_test.go:60: ... deployment has no authoritative installation owner ...
--- FAIL: TestRepositoryWorkflowRealLifecycle (139.97s)
=== RUN   TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI
    ... deployment has no authoritative installation owner ...
--- FAIL: TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI (0.03s)
=== RUN   TestRepositoryComposeWorkloadOnlyLifecycleInCI
    ... deployment has no authoritative installation owner ...
--- FAIL: TestRepositoryComposeWorkloadOnlyLifecycleInCI (0.19s)
=== RUN   TestRepositoryBuildWorkloadRebuildsSourceChangesInCI
    ... deployment has no authoritative installation owner ...
--- FAIL: TestRepositoryBuildWorkloadRebuildsSourceChangesInCI (0.23s)
FAIL
	github.com/mcpdev80/baseharbor/cmd/baha	140.464s
```

Cross-family identical failure (the "w273 podman fall" confirmed): docker AND
podman both pass the isolation/bootstrap gate and fail at the identical
`ErrDeploymentOwnerMissing` seam. This is a family-wide defect.

## GATE (3) mutant probe at the genuine isolation line (`deployment_store.go:107`)

Line: `internal/deployment/deployment_store.go:107` `return ErrDeploymentOwnerMissing`
(inside `VerifyDeploymentOwner`, the one-installation-owner storage seam).

- SHA256 ORIGINAL:
  `8e15cdfbe3a3d37fa863811b4c9c91a5875ed4e02178fd28539aa787b7998807`
- MUTANT applied (line 107 `return ErrDeploymentOwnerMissing` -> `return nil`, i.e.
  the seam admits a missing owner):
  SHA256 MUTANT:
  `d02c92a135a73d277bffd11095a23abf4a36af15fe0395074f5f68699d957463`
  Run on podman (small test, `BASEHARBOR_TEST_RUNTIME=podman`):
  `--- PASS: TestRepositoryComposeWorkloadOnlyLifecycleInCI (130.64s)` — the mutant
  lets the test get PAST the seam. The guard MATTERS.
- RESTORED: `cp /tmp/deployment_store.go.orig internal/deployment/deployment_store.go`
  SHA256 RESTORED:
  `8e15cdfbe3a3d37fa863811b4c9c91a5875ed4e02178fd28539aa787b7998807` (== original).
  Build clean after restore.

Three SHA256 stages captured (original -> mutant -> restored==original). The existing
unit seam test suite (`internal/deployment/deployment_owner_binding_test.go`, 15
tests) stays GREEN (in-memory/fake owner resolver); the INTEGRATION apply path is
the genuine RED where no Core owns the per-test target.

## Repo-rule compliance

- No `.github/` files edited, no workflow_dispatch, no CI rerun (Owner field).
- No subagents spawned. No code change of any kind (Owner: report this card's defect
  RED, do NOT fix in PR #1383; defect routed to a different chain).
- No runner/infra build, register or repair. Test VMs = GitHub-hosted shared runners
  only; the four InCI tests ran on dedicated SSH test hosts (baha-docker-2,
  baha-podman), not the coordinator/codeman server.
- VM compose projects: none started on the coordinator host. No orphaned w282
  containers on either host; both vm-claims released (`FREE host=baha-docker-2`,
  `FREE host=baha-podman`); ps-witness: no `w282` containers on either host.
- `bh-*shared*` volumes/containers untouched (foreign ownership).

## Hosts / run-ids

- docker host: baha-docker-2, docker 29.9.3, run-ids per test target suffixes
  (`w169-repository-workflow-...-iso` etc.), matrix rc=FAIL (4/4, same seam).
- podman host: baha-podman, podman 5.7.0, run-ids `w169-*-iso`, matrix rc=FAIL
  (4/4, same seam).
- Hosted checks at new head f236b66: observed PENDING (`total_count: 0`) via GitHub
  status API (terminal observation; not yet a live pass/fail).

## END

END_DONE w282-compose-cells-now: head=f236b6610b88 branch=work/r27-compose-isolation (PR #1383, develop-merged, parents [b11eec12, 8bda98c1]) docker_zellen=0/4 GRUEN (isolation bootstrap GONE now; 4/4 ROT at ErrDeploymentOwnerMissing deployment_store.go:107, w169-*-iso no owner Core) podman_zellen=0/4 GRUEN (identical cross-family defect, w273 fall confirmed) mutante=8e15cdfbe3a3d37fa863811b4c9c91a5875ed4e02178fd28539aa787b7998807 (orig) / d02c92a135a73d277bffd11095a23abf4a36af15fe0395074f5f68699d957463 (mutant return nil -> test PASS 130.64s) / 8e15cdfbe3a3d37fa863811b4c9c91a5875ed4e02178fd28539aa787b7998807 (restored) journal=claim w282 baha-docker-2 + release + claim w282 baha-podman + release (4 zeilen) lauf=113.68s/97.059s docker + 139.97s/140.464s podman (PENDING hosted) lsremote=b11eec12f90 (alt) / f236b6610b88 (neu) akte=/home/marcel/dev/baseharbor-ecosystem/workers/compose-host-isolation/round-7.md | isulations-gate formerly blocking is now GONE; four InCI tests fail LATER at genuine ErrDeploymentOwnerMissing seam (deployment_store.go:107), family-wide docker+podman defect now on card t-20261106-r27-remote-up-podman-owner-missing (lane w286); this card reports honest ROT as END_DONE, no code change in PR #1383, no orphaned containers, both vm-claims released.
