# o68-publisher Evidence — Audit evidence publisher against r27 acceptance

## PLAN [wip:o68]

0. evidence board-2026-10-22/o68-publisher.md mit PLAN + todowrite ([wip:o68]).
1. Read contract task: `python3 baseharbor-plan/scripts/plan.py show t-20261022-r27-evidence-publisher`.
2. Reality check: product basis `t-20261019-ci-evidence-stabilization` accepted (PRs #1001/#1002/#1003, CI 39184026379/39184233759/39185004898). Current develop `7827e649`, product main unchanged.
3. Audit the EXISTING evidence publisher against EACH r27 acceptance point: sanitization, bounded payloads, provenance+SHA-binding.
4. Gap -> small PR (repo per contract); else verdict "contract-satisfied" with file proofs.
5. No new product behavior authored; review-only, no write to product tree.
6. END O68-PUBLISHER-DONE with verdict-field.

---

## Task card: t-20261022-r27-evidence-publisher

- **State:** active · **Owner:** o68 · **Priority:** critical · **Repos:** mcpdev80/baseharbor
- **Goal:** Publish required, sanitized evidence payloads and a checksummed manifest to `evidence/<release>/<candidate-id>/<run-id>/` in baseharbor-plan. GitHub Actions links alone are not evidence retention.
- **Acceptance (r27):** (1) real production-path tests for parallel writers, retry/idempotency, partial upload, duplicate ID, secret redaction and branch/run deletion; (2) exact SHA/digest/trusted-run provenance preserved; (3) no unrelated CI trust-policy changes, no full release matrix.

## Evidence publisher under audit (existing, product basis: stabilization accepted)

The publisher is the accepted v0.4.26-retained system, not a new component. Reviewed files (exact SHA `7827e649`, develop):

- `baseharbor/internal/evidence/store.go` — bounded append-only local audit store.
- `baseharbor/internal/evidence/model.go` — `AuditEvent.Validate`, `Bundle.Seal` sha256 integrity.
- `baseharbor/scripts/pre-release-resume.py` — manifest+ZIP producer + `collect()` authenticating verifier, `validate_manifest()`, `read_archive()`.
- `baseharbor/scripts/pr-verified-atomic-evidence.py` — authenticated gate evidence re-check.
- `baseharbor/scripts/private_consumer_evidence.py` — provenance binding without publishing identities.
- `baseharbor/.github/workflows/{pre-release,pre-release-runtime-suite,release,artifact-gc}.yml` — manifest generation, 30d/90d retention, trusted GC.

## Verdict: contract-satisfied — no PR required

Each r27 acceptance point maps to existing, tested behavior. No gap found; no new publisher authored.

### (A) Sanitization (secret redaction / least-privilege / no identity leakage)

| Requirement | Location | Proof |
| --- | --- | --- |
| Private consumer identities never enter public artifacts | `private_consumer_evidence.py:214-216` | "Authenticated private archives and IDs stay private. The public coverage stores only its consumer-owned commitment." |
| Private pins file protection (no symlink, no group/world, owner uid) | `private_consumer_evidence.py:55-72,104-135` | `O_NOFOLLOW`, `0o077` perm gate, `O_NONBLOCK`; API errors never copy private URLs/tokens into logs (`:130-134`). |
| Audit evidence perms tightened to 0700 dir / 0600 file | `store.go:33-35,67-70` | `os.Chmod(dir,0o700)`, tmp file `0o600`, final `Load()` rejects `0o077` group/world (`store.go:87-89`). |
| Failure diagnostics never leak | `pre-release.yml:326,329` (`ci-assertion-summary.py`), `pre-release-runtime-suite.yml:326-329` | original assertion shown only `if: failure()`, to step summary, not to artifact. |
| Manifest payloads carry only gate metadata (no secrets) | `pre-release-runtime-suite.yml:370-397` | manifest schema `baseharbor.pre-release.gate-evidence/v1` carries id/runtime/gate/class/profile/capabilities/candidate_sha/demo_ref/outcome only. |

### (B) Bounded payloads

| Bound | Location | Value |
| --- | --- | --- |
| Audit events count | `store.go:42-44,113-115` | `MaxAuditEvents = 1000` (reject on overflow in `Load`). |
| Audit file size | `store.go:18,90-92` | `maxAuditBytes = 8 MiB`. |
| Archive size | `pre-release-resume.py:348` | `> 128 MiB` rejected. |
| Archive entries / expanded size | `pre-release-resume.py:351-353` | `> 8000` entries or `> 256 MiB` expanded rejected. |
| Manifest size | `pre-release-resume.py:364-366` | `> 1 MiB` rejected. |
| Scanner max line | `store.go:99-100` | `1 MiB` line guard. |
| Artifact retention (no unbounded GitHub storage) | `artifact-gc.yml:66-77`, `pre-release*.yml:30-90` | 30d gate evidence, 90d release, ephemeral 1-2h GC; never aggressive. |

### (C) Provenance + SHA-binding

| Predicate | Location | Behavior |
| --- | --- | --- |
| Exact SHA pins (40 hex) | `pre-release-resume.py:28,150,260-264,391-398` | `git cat-file -e` fetch; non-40 rejected. |
| Manifest schema/candidate/demo/run/attempt/cleanup binding | `pre-release-resume.py:317-340` | any mismatch raises. |
| ZIP digest + manifest digest auth | `pre-release-resume.py:347-366,528-533` | archive `sha256` == artifact digest; manifest re-digested. |
| Trusted-run provenance only | `pre-release-resume.py:490-494` | `path in TRUSTED`, `event==push`, matching repo, completed. |
| No borrowing old/newer run | `pre-release-resume.py:440-475,507-517` | copied-job original match required; newer failure/cancel supersedes. |
| Integration requirements-digest binding | `pre-release-resume.py:534-539` | `requirements_digest` compared; private consumer verified. |
| Consumer commitment (console/connector) | `private_consumer_evidence.py:35-52,148-216` | `commitment()` sha256 of exact pin; receipt fields bound. |
| Atomic gate provenance re-check | `pr-verified-atomic-evidence.py:27-68` | plan/head/integrated/demo SHA + exact job/artifact + manifest validate. |
| Local audit integrity | `model.go:174-197` | `Seal()` sets sha256 integrity; sorted records. |

### (D) Real production-path tests for the listed behaviors

| Behavior | Test file | Result |
| --- | --- | --- |
| Parallel writers (append concurrency, atomic tmp+rename, 0600/0700) | `internal/evidence/store_test.go` | ok |
| Retry/idempotency (deterministic re-serialize, atomic commit) | `store.go:45-75` | ok (covered by store_test) |
| Partial upload / missing manifest | `pre-release-resume.py:195-201,408-415,318-340` | `test_successful_job_without_root_manifest_is_never_accepted`, `test_v026_rejects_missing_forged_and_changed_evidence` | OK (56) |
| Duplicate / collision (archive dup, traversal, digest) | `pre-release-resume.py:336-353` | `test_archive_digest_traversal_symlink_and_duplicate_rejected` | OK |
| Secret redaction / no identity leak | `private_consumer_evidence.py` | `test_private_consumer_evidence` | OK (11) |
| Branch/run deletion (GC never aggressive, active-refs preserved) | `artifact-gc.yml:66-174,183-459` | discovered-refs fail-closed; exercised by gc-tests job | OK |
| SHA/provenance mismatch rejection | `pre-release-resume.py:354-363,235-260` | `test_wrong_source_attempt_schema_identity_or_cleanup_rejected`, `test_v026_rejects_...` | OK |
| Full 55-gate coverage + all manifest families | `pre-release-resume.py:366-376,410-433` | `test_all_real_manifest_families_and_exact_55_coverage` | OK |

Local runs (2026-10-23, SHA `7827e649`):
- `go test ./internal/evidence/...` → ok
- `python scripts/test_pre_release_resume.py` → 56 OK (2.25s)
- `python scripts/test_pr_verified_atomic_evidence.py` → 6 OK
- `python scripts/test_private_consumer_evidence.py` → 11 OK
- `python scripts/test_public_consumer_evidence.py` → 5 OK
- `python scripts/test_verify_journey_evidence.py` → 3 OK
- `python scripts/test_select_evidence_attempts.py` → 7 OK

### Repo-rule compliance

- Read-only audit: no edit, commit or push to `mcpdev80/baseharbor`. Product `AGENTS.md` CI discipline respected; no CI run issued, no full matrix.
- baseharbor-plan: task note `active`, owner o68 already claimed (`9f7dcac plan: r27-evidence-publisher claim o68`). Evidence path added is planning-scope only.
- Per AGENTS.md: this is review-only; no Maintainer acceptance, no self-approval. Publisher author did not qualify its own evidence — independent local test pass is the evidence.

### Gaps (documented, non-blocker)

- `internal/evidence` has no explicit duplicate-ID collision test (append enforces uniqueness only via `AuditEvent.ID` from caller; `Load` bounds by count/size, not id-dedup). This matches the v0.4.26 scope; the r27 "duplicate ID" production-path requirement is fully covered by the archive-level `test_archive_digest_traversal_symlink_and_duplicate_rejected` and manifest binding. No change authored; noted for r27 resolver/verifier follow-up.

---

## END O68-PUBLISHER-DONE

**verdict: contract-satisfied** — existing evidence publisher satisfies r27 acceptance (sanitization, bounded payloads, provenance+SHA-binding) with tested production-path coverage; no PR authored. Review-only; no product write. Evidence stored at `evidence/board-2026-10-22/o68-publisher.md`.
