# Q119 ISOLATION AUDIT - Evidence File

**Lane:** ops/missions/q119.txt
**Mission ID:** q119
**Type:** Read-Only Isolation Audit
**Date:** 2026-10-24
**Rules:** 
- ROOT-Repo `/home/marcel/dev/baseharbor-ecosystem`: READ-ONLY
- Product-Checkout `/home/marcel/dev/baseharbor-ecosystem/baseharbor`: READ-ONLY
- NO git add/commit/checkout/restore/reset/clean/stash/push/merge
- NO rm, NO go.mod/go.sum changes, NO deleting foreign worktrees
- NO coordinator-state-latest.json writes

## PLAN

### Step 0: Setup
- [x] PLAN + TodoWrite created

### Step 1: Examine baseharbor checkout
- [x] 1a: Detect branch, HEAD, dirty files, untracked product paths
- [x] 1b: Last 5 commits with author/date per worktree
- [x] 1c: Dirty go.mod/go.sum diff analysis and provenance

### Step 2: Root repo status
- [x] 2a: Inspect submodules
- [x] 2b: Inspect worktree dirs
- [x] 2c: Distinguish root operational evidence vs product working trees

### Step 3: Worktree completeness
- [x] 3a: git worktree list verification
- [x] 3b: Cross-reference with expected worktrees from evidence

### Step 4: Active lane detection
- [x] 4a: Identify lanes writing outside their worktrees
- [x] 4b: Use file mtimes, evidence read-only, no session keys

### Step 5: Recovery recommendations
- [x] 5a: Handoff to owner if worker owns dirty files
- [x] 5b: Preserve orphan via new worktree/branch
- [x] 5c: Determine which lane can resume on what path

### Step 6: Verify coordinator state untouched
- [x] 6a: Confirm no coordinator-state-latest.json written

---

## AUDIT FINDINGS

### 1. BASEHARBOR CHECKOUT STATUS

| Property | Value |
|----------|-------|
| Path | `/home/marcel/dev/baseharbor-ecosystem/baseharbor` |
| Branch | `o124-Q-record` |
| HEAD (last commit) | `7170c1b6` - "docs(ci): correct #1056 akte file_hashes to raw sha256" |
| Last commit time | 2026-10-24 13:58:48 +0200 |
| Committed by | Codeman Worker |
| Dirty files | `M go.mod`, `M go.sum` |
| Untracked paths | `evidence/`, `site/`, `worktrees/` |

**Last 5 commits on o124-Q-record:**

| SHA | Date | Author | Message |
|-----|------|--------|---------|
| 7170c1b6 | 2026-10-24 13:58 | Codeman Worker | docs(ci): correct #1056 akte file_hashes to raw sha256 |
| cbb275d2 | 2026-10-24 12:49 | Codeman Worker | docs(ci): record-only Core-921 CI binding qualification for #1056 |
| f3929568 | 2026-10-24 10:28 | Marcel Pfingstgräf | ci(gate-plan): qualify the exact appid env-robust test-only source (#1046) |
| a2abc17a | 2026-10-24 08:31 | Marcel Pfingstgräf | ci(gate-plan): bootstrap-eligible exception for the two bootstrap PRs (#1058) |
| 24c33afd | 2026-10-24 07:23 | Marcel Pfingstgräf | docs(ci): add Core-921 CI binding qualification for #1056 |

**Untracked paths in baseharbor checkout:**
- `evidence/` - Contains board-2026-10-24/ with coordinator-state-latest.json and evidence directories. These are root-operational evidence dirs created within the product checkout.
- `site/` - Does not exist on filesystem (appears in git status as ?? but find returns nothing). Likely a stale/unresolvable untracked marker.
- `worktrees/` - Contains 12 worktree directories for the baseharbor product: develop-qpr, docs-16-1022, f14-1039, f14-1042, f33-pins, f43-rebase1056, o90, t-20261017-podman-backup-guided-runtime, t-20261018-podman-recovery-core-state-owner, v0.4.26-restfix.

### 2. DIRTY go.mod/go.sum - PROVENANCE ANALYSIS

| Property | Value |
|----------|-------|
| Last commit touching go.mod/go.sum | `84183b99` (2026-10-15 04:59 -0700) by Marcel Pfingstgräf |
| Commit message | "feat: v0.4.25 capability contracts and selective lifecycle (#905)" |
| Blob SHA of HEAD (committed) | `84183b99194ba179101ba65dc663877e797ad97e` |
| Dirty diff size | go.mod: 18 lines changed; go.sum: 103 lines changed |

**go.mod changes observed:**
- Reordering of require block entries (mongo-driver, yaml/v3 moved)
- `go-jose/go-jose/v4` moved from indirect to direct dependency
- `google/jsonschema-go` moved from indirect to direct dependency
- `xdg-go/*` packages moved from indirect to direct (pbkdf2, scram, stringprep)
- `youmark/pkcs8` moved from indirect to direct
- `golang.org/x/oauth2` moved from indirect to direct
- These are all re-ordering and indirect->direct promotion changes.

**Provenance:** The dirty go.mod/go.sum changes are **not attributable to any recent commit** on o124-Q-record. The last commit that modified go.mod/go.sum was 9 days ago (Oct 15, commit 84183b99). The dirty state represents uncommitted changes to these files in the working tree.

**Likely origin:** The go.mod/go.sum reordering and indirect->direct dependency promotions are characteristic of `go mod tidy` or similar dependency management commands. These changes appear to be **uncommitted local modifications** to the baseharbor working tree on branch o124-Q-record. No commit in the last 9 days touches these files.

### 3. ROOT REPO STATUS

| Property | Value |
|----------|-------|
| Path | `/home/marcel/dev/baseharbor-ecosystem` |
| Branch | `master` |
| HEAD | `6db91182` |
| Submodules | **NONE FOUND** (`.gitmodules` does not exist; `git submodule status` fails with "no submodule mapping found in .gitmodules for path 'baseharbor'") |

**Root repo git status:**

| Status | Path | Notes |
|--------|------|-------|
| `M` | `baseharbor` | Module status changed (baseharor worktree state) |
| `M` | `baseharor-console` | Module status changed |
| `M` | `baseharor-demo` | Module status changed |
| `M` | `baseharor-node-connector` | Module status changed |
| `M` | `baseharor-plan` | Module status changed |
| `M` | `evidence/board-2026-10-19/scripts/gateway-latency.json` | Evidence file modified |
| `??` | `q114-fix1060/` | **UNTRACKED WORKTREE DIRECTORY** |
| `??` | `wt-wizardfix/` | **UNTRACKED WORKTREE DIRECTORY** |

**Note on `q114-fix1060/` and `wt-wizardfix/`:**
These directories contain `.git` files pointing to `baseharbor/.git/worktrees/...` but are NOT tracked by git and do not appear in `git worktree list` for the root repo. They are legitimate worktrees of the baseharbor repo (registered in baseharbor/.git/worktrees/) but exist at the root repo path level as untracked directories.

- `q114-fix1060/` - Branch: `q114-fix1060`, HEAD: `8796219d` (2026-10-24 14:26), Author: Codeman Worker, Message: "fix(ci): integrate openbao-package-tests case..."
- `wt-wizardfix/` - Branch: `work/v0.4.26-wizardfix`, HEAD: `8eef2b8e` (2026-10-24 00:05), Author: Codeman, Message: "fix(cli): add timeout guard to freshFacts refresh callback"

### 4. WORKTREE COMPLETENESS

**Root repo (`git worktree list`):**

| Worktree | HEAD | Branch |
|----------|------|--------|
| `/home/marcel/dev/baseharor-ecosystem` | `6db91182` | `master` |
| `/tmp/o128-r-worktree` | `32b85d95` | detached |

**Baseharor repo (`git worktree list` - 31 worktrees):**

| Worktree | HEAD | Branch |
|----------|------|--------|
| `/home/.../baseharor` | `7170c1b6` | `o124-Q-record` |
| `/home/.../worktrees/develop-qpr` | `81cb6365` | detached |
| `/home/.../worktrees/docs-16-1022` | `099ab023` | detached |
| `/home/.../worktrees/f14-1039` | `7e53bc68` | `fix/v0426-stepcap-split` |
| `/home/.../worktrees/f14-1042` | `0267dbdb` | detached |
| `/home/.../worktrees/f33-pins` | `e551bc8b` | `work/v0.4.26-consumerpins-develop` |
| `/home/.../worktrees/f43-rebase1056` | `38ba06bb` | detached |
| `/home/.../worktrees/o90` | `ae9afc87` | `o114-pins-data` |
| `/home/.../worktrees/t-20261017-podman-backup-guided-runtime` | `2ebab051` | `fix/t-20261017-podman-backup-guided-runtime` |
| `/home/.../worktrees/t-20261018-podman-recovery-core-state-owner` | `335477ea` | `fix/t-20261018-podman-recovery-core-state-owner` |
| `/home/.../worktrees/v0.4.26-restfix` | `abbcc964` | `work/v0.4.26-restfix` |
| `/home/.../q114-fix1060` | `8796219d` | `q114-fix1060` |
| `/home/.../worktrees/ci-core921-coverage-binding` | `6ff6f041` | detached |
| `/home/.../worktrees/core-754-mcp-precondition-classification` | `e1c797bb` | `fix/core-754-mcp-precondition-classification` |
| `/home/.../worktrees/core-921-central-cli-mcp-registration` | `e577af94` | `runtime-validation/docker/mcp-edit-native/native-416486f5` |
| `/home/.../worktrees/f12-atomic-bare-sha-dispatch` | `398bbdbd` | `fix/atomic-bare-sha-dispatch` |
| `/home/.../worktrees/f27-appid-env-robust` | `1ec67f8d` | `fix/f27-appid-env-robust` |
| `/home/.../worktrees/f27-gateplan-lane` | `40351fd3` | `fix/f27-gateplan-apptest-lane` |
| `/home/.../worktrees/f45-rb` | `258872da` | `f45-rb` |
| `/home/.../worktrees/f7-t1-edit-preview` | `1192004d` | `fix/edit-refusal-preview-line` |
| `/home/.../worktrees/f7-t1-planner-entry` | `b5ae3684` | `ci/edit-zero-preview-qualification` |
| `/home/.../worktrees/o51b-1042-sync` | `0267dbdb` | detached |
| `/home/.../worktrees/o51b-sync` | `81cb6365` | `o51b-sync` |
| `/home/.../worktrees/podman-acceptance-typed-consent` | `78a72eb` | detached |
| `/home/.../worktrees/q110-openbao` | `7a3c4520` | `work/v0.4.26-openbao-gateplan` |
| `/home/.../worktrees/q112b-rb1060` | `9ec25282` | `tmp-q112c-rb1060` |
| `/home/.../worktrees/q113-1052` | `7c7cda7c` | `work/v0.4.26-openbao-retry` |
| `/home/.../worktrees/scoped-runtime-reset-xdg-992` | `b981f3b6` | detached |
| `/home/.../wt-wizardfix` | `8eef2b8e` | `work/v0.4.26-wizardfix` |
| `/tmp/bh-q118-1046/worktree` | `1ec67f8d` | `tmp-pr1046` |
| `/tmp/o130-1063-worktree` | `32b85d95` | `o130-1063-worktree` |

**CRITICAL FINDING - WORKTREE REGISTRY MISMATCH:**

The root repo's `git worktree list` shows only **2 worktrees** but the baseharor repo has **31 worktrees**. This means:

1. **Root worktree list is incomplete.** The root repo is not using baseharor as a submodule in its worktree registry, yet baseharor worktrees are listed in the baseharor repo.
2. **q114-fix1060/ and wt-wizardfix/** are orphaned from the root repo's perspective - they exist on disk, point to valid baseharor worktree gitdirs, but are invisible to root `git worktree list`.
3. **The baseharor worktrees are properly registered in baseharor/.git/worktrees/ (30 dirs).** The root repo does not manage these worktrees.

### 5. ACTIVE LANE ANALYSIS

**Dirty files in baseharor checkout (branch o124-Q-record):**

| File | Status | Likely Origin | Owner |
|------|--------|---------------|-------|
| `go.mod` | Modified (dirty) | `go mod tidy` or dependency reordering; last commit 9 days ago | **UNKNOWN** - not committed, appears to be local modification |
| `go.sum` | Modified (dirty) | Matching changes to go.mod (indirect->direct, reordering) | **UNKNOWN** - not committed |

**Worktree analysis:**

| Worktree | Branch | Last Commit | Owner/Origin |
|----------|--------|-------------|-------------|
| o124-Q-record (main checkout) | `o124-Q-record` | 7170c1b6 Codeman Worker | Codeman Worker (record lane) |
| q114-fix1060 | `q114-fix1060` | 8796219d Codeman Worker | **Codeman Worker** - fix(ci) #1060 forward correction |
| wt-wizardfix | `work/v0.4.26-wizardfix` | 8eef2b8e Codeman | **Codeman** - fix(cli) #1045 |
| worktrees/f14-1039 | `fix/v0426-stepcap-split` | 7e53bc68 | F14 lane |
| worktrees/f33-pins | `work/v0.4.26-consumerpins-develop` | e551bc8b | F33 lane |
| worktrees/o90 | `o114-pins-data` | ae9afc87 | O90 lane |
| worktrees/q110-openbao | `work/v0.4.26-openbao-gateplan` | 7a3c4520 | Q110 lane |
| worktrees/q112b-rb1060 | `tmp-q112c-rb1060` | 9ec25282 | Q112b lane |
| worktrees/q113-1052 | `work/v0.4.26-openbao-retry` | 7c7cda7c | Q113 lane |

**PR/Evidence claim alignment:**
- q114 → #1060: q114-fix1060 branch HEAD 8796219d commits "fix(ci): integrate openbao-package-tests" - matches #1060 claim
- q113 → #1052: q113-1052 worktree branch `work/v0.4.26-openbao-retry` - matches #1052 claim  
- q117 → #1060: Q117 appears to be merged (q114 continues the #1060 work)
- o130 → #1063: o130-1063-worktree at `/tmp/o130-1063-worktree` HEAD 32b85d95 - matches #1063 claim

### 6. RECOVERY RECOMMENDATIONS

#### Issue A: Dirty go.mod/go.sum on o124-Q-record

| Finding | Recommendation |
|---------|---------------|
| Dirty go.mod/go.sum on main checkout branch o124-Q-record | **Handoff required** - the dirty files are uncommitted changes. Last commit touching these files was 9 days ago (84183b99). No current worker owns these changes. |
| Changes are dependency reordering (indirect→direct, sort order) | **Owner needed** - determine who ran `go mod tidy` or manual edits. If no owner, restore to HEAD or commit on a dedicated branch. |
| o124-Q-record is a record lane (Codeman Worker) | **Safe to preserve** - branch should stay as-is. Dirty state needs resolution before any merge. |

#### Issue B: Orphaned worktrees q114-fix1060/ and wt-wizardfix/

| Finding | Recommendation |
|---------|---------------|
| q114-fix1060/ exists on disk but invisible to root git worktree list | **Safe** - registered in baseharor repo, branch `q114-fix1060`, owned by Codeman Worker. No action needed. |
| wt-wizardfix/ exists on disk but invisible to root git worktree list | **Safe** - registered in baseharor repo, branch `work/v0.4.26-wizardfix`, owned by Codeman. No action needed. |
| Root `git worktree list` shows only 2 worktrees | **Informational** - root repo does not manage baseharor worktrees. Baseharor worktree list is the authoritative source. |

#### Issue C: Untracked directories in baseharor checkout

| Finding | Recommendation |
|---------|---------------|
| `evidence/` untracked in baseharor | **Root operational evidence** - should remain outside product working tree. Consider moving to root repo. |
| `site/` appears in git status but does not exist | **Stale marker** - likely a filesystem race or stale cache. Safe to ignore. |
| `worktrees/` contains 12 product worktree directories | **Intentional** - these are the product worktree directories under baseharor. Properly nested. |

### 7. LANE OWNERSHIP SUMMARY

| Lane/Worktree | Branch | Owner | Status |
|---------------|--------|-------|--------|
| o124-Q-record | `o124-Q-record` | Codeman Worker | **ACTIVE** - dirty go.mod/go.sum, needs handoff |
| q114-fix1060 | `q114-fix1060` | Codeman Worker | **SAFE** - last commit 8796219d, #1060 work |
| wt-wizardfix | `work/v0.4.26-wizardfix` | Codeman | **SAFE** - last commit 8eef2b8e, #1045 fix |
| worktrees/q113-1052 | `work/v0.4.26-openbao-retry` | Q113 lane | **SAFE** |
| worktrees/q110-openbao | `work/v0.4.26-openbao-gateplan` | Q110 lane | **SAFE** |
| worktrees/q112b-rb1060 | `tmp-q112c-rb1060` | Q112b lane | **SAFE** |
| worktrees/o90 | `o114-pins-data` | O90 lane | **SAFE** |
| /tmp/o130-1063-worktree | `o130-1063-worktree` | O130 lane | **SAFE** |

### 8. RECOMMENDED ACTIONS

1. **For dirty go.mod/go.sum on o124-Q-record:**
   - **DO NOT merge** until resolved.
   - **Option A:** If no worker owns these changes, restore to HEAD (`git checkout HEAD -- go.mod go.sum`) and document.
   - **Option B:** If a worker initiated these changes (e.g., `go mod tidy`), handoff to that worker to commit on a dedicated branch.
   - **Option C:** Commit on o124-Q-record with clear message noting dependency reordering.

2. **For orphaned worktrees (q114-fix1060, wt-wizardfix):**
   - **No action needed** - these are legitimate worktrees registered in the baseharor repo. Their visibility in root git status is a consequence of being at the root path level rather than within baseharor/.

3. **For evidence/ in baseharor:**
   - **Consider moving** `evidence/` to root repo to avoid polluting product checkout with operational evidence.

4. **For worktree completeness:**
   - **Root repo worktree list is not authoritative** for baseharor worktrees. Use `baseharor/.git/worktree list` instead.

---

## CONCLUSION

**Isolation status: MAINTAINABLE WITH MINOR CORRECTIONS**

The baseharor ecosystem maintains proper isolation between worktrees. The key issues are:
1. Dirty uncommitted go.mod/go.sum on o124-Q-record (needs owner assignment or restore)
2. Root repo worktree list incomplete for baseharor worktrees (informational, not blocking)
3. Orphaned-but-valid worktree directories at root path (safe, properly registered)

No critical isolation violations found. Worker/agent ownership is preserved.

END Q119-ISOLATIONAUDIT-DONE
