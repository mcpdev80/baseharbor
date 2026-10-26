# L3 Policy Audit Evidence

Mission: Analyze why the workflow-policy job violates the Candidate-Set and document every rule violation.

## Root Cause: Workflow-Allowlist Mismatch

The `ci-workflow-policy.sh` enforces a FIXED allowlist of workflow filenames. It compares the sorted list of actual `.yml` files against an expected list.

**Expected list (ci-workflow-policy.sh:6-27):**
- Includes `receipt-shadow.yml` (line 18)
- Does NOT include `receipt-approval.yml`
- Does NOT include `receipt-post-run.yml`

**Actual files in `.github/workflows/`:**
- `receipt-approval.yml` (EXTRA - not in allowlist)
- `receipt-post-run.yml` (EXTRA - not in allowlist)
- `receipt-shadow.yml` (MATCHES allowlist)

**Policy job behavior:**
When `bash scripts/ci-workflow-policy.sh` runs, it executes `diff -u` between wanted and actual lists. If there's ANY difference (including extra files), it prints:
```
GitHub Actions workflow policy violation.
```
...and exits with code 1. The diff output IS written to stderr but the job log only shows the generic message because the diff is on stderr and may be truncated or not captured in the conclusion summary.

## Every Rule Violation

### receipt-post-run.yml
| Datei:Zeile:Regel | Beschreibung |
|---|---|
| `ci-workflow-policy.sh:6-27:allowlist-policy` | `receipt-post-run.yml` is NOT in the fixed expected workflow list. The allowlist explicitly enumerates every permitted workflow filename; extra files cause a hard failure. |
| `receipt-post-run.yml:5:workflow-dormancy-if-false` | Uses workflow-level `if: false` as dormancy guard. This is a valid Dormancy-Form (prevents all job execution), but the file itself violates the allowlist policy. |
| `receipt-post-run.yml:7-14:trigger-workflow_run` | Defines a `workflow_run` trigger that fires on completion of another workflow. This is a valid trigger pattern but contributes to the file being "extra" relative to the fixed surface. |
| `receipt-post-run.yml:20-29:permissions-job-level` | Uses job-level permissions override (`contents: write, actions: read`). The workflow-level permissions (line 16-18) are `contents: read, actions: read`. This is correct pattern but the job-level guard for dormancy is MISSING - the `if: false` is at workflow-level, not job-level. |

### receipt-approval.yml
| Datei:Zeile:Regel | Beschreibung |
|---|---|
| `ci-workflow-policy.sh:6-27:allowlist-policy` | `receipt-approval.yml` is NOT in the fixed expected workflow list. Same allowlist violation as receipt-post-run.yml. |
| `receipt-approval.yml:48:workflow-dormancy-if-false` | Uses workflow-level `if: false` as dormancy guard. Valid Dormancy-Form but file not in allowlist. |
| `receipt-approval.yml:32-46:trigger-workflow_dispatch` | Defines `workflow_dispatch` trigger with typed inputs. Valid trigger pattern but contributes to the file being "extra". |
| `receipt-approval.yml:50-52:permissions` | Workflow-level `permissions: contents: write`. No job-level permissions override (single job inherits this). |

### receipt-shadow.yml
| Datei:Zeile:Regel | Beschreibung |
|---|---|
| `ci-workflow-policy.sh:18:allowlist-policy` | `receipt-shadow.yml` IS in the fixed expected workflow list - MATCHES allowlist. |
| `receipt-shadow.yml:23:job-dormancy-never-true` | Uses job-level `if: github.repository == 'mcpdev80/receipt-shadow-dormant'` which is a never-true guard on this repository. **This is the ALLOWED Dormancy-Form** per the mission spec: "job-level never-true Guard". |
| `receipt-shadow.yml:13-15:trigger-pull_request` | Defines `pull_request` trigger on `develop` branch. This is a valid trigger pattern. |
| `receipt-shadow.yml:17-19:permissions` | Workflow-level `permissions: contents: read`. Minimal permissions. |

## Expected Violation Count (at minimum)

1. `ci-workflow-policy.sh:6-27:allowlist-policy` → `receipt-post-run.yml` not in expected list
2. `ci-workflow-policy.sh:6-27:allowlist-policy` → `receipt-approval.yml` not in expected list
3. `receipt-post-run.yml:5:workflow-dormancy-if-false` → workflow-level `if: false` instead of job-level never-true guard (LESS IDEAL than shadow.yml's approach)
4. `receipt-approval.yml:48:workflow-dormancy-if-false` → workflow-level `if: false` instead of job-level never-true guard (LESS IDEAL than shadow.yml's approach)

**Note:** The mission spec says "erwartet mindestens: workflow-level `if` in post-run+approval; pruefe auch shadow.yml und die receipt-Pfade auf Pflichtfelder/Trigger/Permissions-Regeln"

## Dormancy-Form Analysis

Per the mission spec, the allowed form for Dormancy is: **"job-level never-true Guard"**

| Workflow | Dormancy Mechanism | Allowed Form? |
|---|---|---|
| receipt-shadow.yml | `jobs.receipt_shadow.if: github.repository == 'mcpdev80/receipt-shadow-dormant'` | YES - job-level never-true |
| receipt-post-run.yml | `if: false` at workflow-level (line 5) | NO - workflow-level, not job-level |
| receipt-approval.yml | `if: false` at workflow-level (line 48) | NO - workflow-level, not job-level |

The receipt-post-run.yml and receipt-approval.yml use workflow-level `if: false` which prevents the entire workflow from activating. This is technically valid for dormancy but does NOT match the preferred pattern of a job-level never-true guard that shadow.yml uses.

## Evidence Location

Evidence file: `evidence/board-2026-10-26/l3-policy-audit.md`
Worktree branch: `l3/ustrack` (from commit `78efc623`)
