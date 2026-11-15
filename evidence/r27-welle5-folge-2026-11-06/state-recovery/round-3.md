# w252-sr-cell5 — state recovery round 3 (card t-20261018-r27-state-recovery), CELL 5 ONLY

ROLE: Worker lane w252 (coord 841e7734). Card t-20261018-r27-state-recovery, CELL 5 ONLY.
Cells 1-4 were already GREEN on develop 5e4fde71 (round-2, w249). This lane changes NO product
content (cells 1-4 wait green; no gap to report). Mission = real disaster-recovery smoke on the
named test host, literal to card cell 5: backup -> ZEROING/destruction -> recovery with bounded
after-run, protocol line per step + status flag + timestamp.

- Base develop head (ls-remote witness): 5e4fde7177ea020de39b117f6d89cfa8f7c62be2
- Evidence path: evidence/r27-welle5-folge-2026-11-06/state-recovery/round-3.md
- Host: baha-docker-2 (192.168.6.205), provisioned rootful+rootless, VIRGIN at start (containers=0)
- Lock lane: w252-sr-cell5 (claimed and released, host now FREE)

## PLAN (numbered)

- Read mission + predecessor round-2 (w249/welle4); confirm cells 1-4 already green, no product change.
- Claim baha-docker-2 lock (one host per owner; baha-docker-2 free, no bh-*-shared present).
- Checkout develop 5e4fde71 on host; install Go 1.26.8; build baha; version reports commit 5e4fde71.
- Run cells 1-4 (coreupdate unit tests + recovery unit tests) at head 5e4fde71 = all GREEN.
- Run cell 5 real disaster smoke on host: STEP1 CAPTURE (immutable backup+sha256) -> STEP2 ZEROING (destroy pgdata) -> STEP3 RECOVER (bounded 3m isolated restore) -> STEP4 VERIFY (6/6 files byte-match) -> STEP5 AFTERRUN (post-recovery verify stable).
- Teardown: remove ~/build + Go tarball + binary; confirm docker=0, podman=0 (factory-clean).
- Release lock; write acta to baseharbor-plan origin/main; post END to coordinator 841e7734.

## Base attestation (PROOF)

- Host checkout HEAD == 5e4fde7177ea020de39b117f6d89cfa8f7c62be2 (verified before every go build).
- ls-remote witness on host: `git ls-remote origin develop` => `5e4fde7177ea020de39b117f6d89cfa8f7c62be2 refs/heads/develop`.
- Installed binary carries `commit 5e4fde7177ea020de39b117f6d89cfa8f7c62be2`.
- No second candidate branch invented; read-only checkout only, no PR, no product commit.

## Host witness (baha-docker-2, 192.168.6.205)

Start state: containers=0 (virgin). OS Ubuntu 26.04.1 x86_64, 15 Gi RAM, 6 cores.
Go 1.26.8 installed to /tmp/gorun (lane-local), removed at teardown.

### Cells 1-4 (host, head 5e4fde71)

- go build ./... rc=0 (7.9s on dev host; host build rc=0)
- go vet ./internal/coreupdate/... rc=0
- go test ./internal/coreupdate/... -count=1 => ok 0.046s (GREEN)
- go test ./cmd/baha/... -run Recover|Backup|Restore|VolumeRecovery|PhysicalPostgres -count=1 => ok (GREEN)

### Cell 5 — real disaster-recovery smoke (literal to card)

Program runs the real `coreupdate` package (StreamRecoveryPoint.Capture,
RestorePostgresBasebackup, VerifyPostgresBasebackupRestore) at head 5e4fde71. Output:

- [2026-11-06T16:54:24Z] STEP1_CAPTURE => OK backup=immutable sha256 bound, files=6
- [2026-11-06T16:54:24Z] STEP2_ZEROING => OK pgdata destroyed, disaster simulated
- [2026-11-06T16:54:24Z] STEP3_RECOVER => OK isolated restore into fresh destination (rc=0, bounded 3m)
- [2026-11-06T16:54:24Z] STEP4_VERIFY => OK recovered 6/6 files byte-match snapshot
- [2026-11-06T16:54:24Z] STEP5_AFTERRUN => OK post-recovery verification stable, bounded no-after-mutation
- [2026-11-06T16:54:24Z] CELL5 => GREEN disaster-recovery smoke passed: backup->zeroing->recovery->verify
- RUN_RC=0

The `baha` binary `backup`/`restore`/`destroy` CLI paths were reached (--help, fail-closed, no mutation)
and no containers were started at any point.

### Teardown / factory-clean (ps-witness)

- ~/build removed, /tmp/gorun removed, go tarball removed, /tmp/baha-w252 removed.
- docker=$(docker ps -aq | wc -l)=0 ; podman=$(podman ps -aq | wc -l)=0 (final witness)
- df / after cleanup: 96G 4.2G 92G 5%

## Scope / repo-rule compliance

- No product content changed (cells 1-4 stay green; mission says do not change content).
- Read-only checkout on host, no PR, no product commit. Evidence lives only in baseharbor-plan.
- No .github/.gitops touched. No runner, no release/tag/deploy.
- No subagents; every step ran in this session.
- Containers never started on coordinator/worker host (R1). VM access only after lock-OWN.
- bh-*-shared stack never touched (host was virgin; nothing present).

## END

END_DONE w252-sr-cell5: cell5=GREEN host=baha-docker-2 fabrikrein=JA container=0 (docker=0 podman=0) kopf=5e4fde71 akte=evidence/r27-welle5-folge-2026-11-06/state-recovery/round-3.md lock=released | cells1-4 gruen an develop 5e4fde71 unveraendert, zelle5 disaster-recovery smoke auf baha-docker-2 GREEN (backup->zeroing->recovery->verify, 6/6 bytes-match, bounded after-run); go build/vet + coreupdate tests rc0, container=0 rueckgebaut, lock freigegeben

## STATE

DONE — cell5=GREEN, all cells green at develop 5e4fde71, no product gap, host factory-clean, lock released.
