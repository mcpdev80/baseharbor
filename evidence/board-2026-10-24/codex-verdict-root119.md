# Root/worktree hygiene review — q119

ROLLE: REVIEW
UTC started: 2026-10-24T12:44:36.513190+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native tool operation claimed. Durable checklist follows.

- [x] 0. Read binding policies and cmf14; record role, prohibited actions and plan.
- [x] 1. Read q119 and applicable ownership context; independently inspect authorized worktree/root statuses, HEAD/log/worktree refs and file metadata.
- [x] 2. Compare dirty module bytes/blob history and timestamps; distinguish facts, probable attribution and uncertainty.
- [x] 3. Document ranked nondestructive recovery commands, risk/approval boundaries; recommend one option and at most one already-authorized worker action. Execute no recovery.
- [x] 4. Recheck relevant states/mtimes and complete evidence with UTC/end marker.

Prohibited: git restore/checkout/commit/push/merge, rm, product edits, gh config/auth access. No recovery operation will run. Root Git reads are explicitly authorized by cmf14.

## Verdict — review complete; preserve shared state, no cleanup

Independent reviewer: Codex /root. **Recommend leaving the shared base checkout untouched while existing owners continue only in their assigned isolated worktrees.** The dirty module owner is unproven. Cleanup is not authorized by this review or by q119's read-only assignment. No recovery action was performed; commands below are explicitly future recipes.

## Independently observed state

All Git reads used GIT_OPTIONAL_LOCKS=0 to avoid optional index refresh writes. No gh invocation/config/auth inspection was used by this review. Timestamps below are UTC; raw statuses/logs/worktree lists/metadata are in codex-cmf14-data/.

| Checkout | Branch / HEAD | Working state and ownership evidence |
|---|---|---|
| Ecosystem root | master / `d1fc46312e85f4cd5672420d51cf8daa29312cae` | Operational evidence commits; dirty coordinator/evidence files, five changed gitlinks and untracked q114-fix1060/wt-wizardfix plus ongoing evidence. Root is already on required master. |
| baseharbor | o124-Q-record / `7170c1b67459d4c4fccb1a0bd41b11d5cec52f51` | Unstaged go.mod/go.sum; index entries match HEAD; untracked evidence/, site/, worktrees/. Current branch matches record lane, not proof of module ownership. |
| q114-fix1060 | q114-fix1060 / `13a05ea492a98bb8922785fb828db2a8ff6324a3` | Clean at observation; actual .git points into baseharbor/.git/worktrees/q114-fix1060. Existing q114/#1060 scope; no module changes here. |
| wt-wizardfix | work/v0.4.26-wizardfix / `8eef2b8e25fa92dd7e7cf6ece4c7c5ff50b69d3c` | Untracked cmd/baha/app_wizard_preview_bounded_test.go,3176 bytes; cannot declare disposable/fully incorporated from branch history. Registered in baseharbor. |

Wizard untracked file raw SHA-256 `0febaae818f5f7671069cf9e6e393686b5a428d3c8e202bee111af58f98b7994`, mtime2026-10-23T21:25:09.163311Z. Preserve it; an earlier merged wizard patch does not prove this untracked file is merged or owned by a currently idle lane.

q119's root HEAD6db91182 and q1148796219d are historical observations. Current q114 log ends13a05ea4; author timestamp12:26:32Z, committer timestamp12:29:21Z. Root now contains subsequent evidence commits bb6368c0 at12:30:45Z and d1fc4631 at12:38:55Z. Commit subjects saying “#1052 accepted” are historical coordinator prose, not independent source acceptance or current CI proof. Nothing was rolled back to those checkpoints.

### Root remote / evidence history

`refs/remotes/origin/master` is **absent**; log/rev-list for it exit128. A credential-redacted repository-only read of `git remote get-url origin` identifies **github.com/mcpdev80/baseharbor.git**, the Core repository, not an independently established ecosystem-evidence remote. Cached origin/HEAD points at origin/main54acc4b66715681213788fdba395a569ddc95a1b; cached origin/develop is65aa063e90767c85af976081c100be1817370e67. These are local cached refs; no remote freshness or push destination correction was attempted. Relative to cached Core origin/main, rev-list gives1617 left-only /76 root-master-only commits. This is **not** proof of “76 unpushed ecosystem commits” or an ahead count against a real origin/master.

Operational master history/reflog is intact: d1fc4631 → bb6368c0 →6db91182 →081fd265 →754cc266 →dbcaacd0. Root must stay master; never reset/switch it to Core origin/main/develop to make this comparison clean, and never push its operational master to that remote as a hygiene step. A correct ecosystem backup/publishing destination requires an Owner decision outside this mission; no remote/config change is recommended as an immediate release task.

### Worktree / gitlink corrections to q119

- Root index tracks five mode160000 gitlinks for the product repositories, while .gitmodules is absent. “No submodules found” is too strong: these are gitlink entries without the usual mapping. Do not initialize/update/deinitialize them or rewrite the index during hygiene. Their dirty indicators are not ordinary root product-file modifications.
- Root and BaseHarbor have **independent** worktree registries. BaseHarbor has31 registered worktrees, including q114 and wizard. Root currently has3: itself, /tmp/o128-r-worktree and /home/marcel/tmp-ops/o135-design. The latter detached entries have Core R/H commits, but their .git files actually point into the root .git/worktrees namespace. That is an observed repository identity requiring care, not evidence that either named q114/wizard worktree is orphaned.
- Absence of BaseHarbor worktrees from root's list is expected, not a “critical incomplete root registry”. Use each checkout's actual git-common-dir. No worktree prune/move/removal is justified by counts alone.
- baseharbor/site **exists** (directory mtime2026-10-23T17:14:56Z). q119's “stale/unresolvable marker” is not confirmed. evidence/site/worktrees remain untouched; untracked directories are not trash.

## Module content / probable origin

| File | HEAD Git blob | Dirty Git blob (computed without writing an object) | Dirty raw SHA-256 |
|---|---|---|---|
| go.mod | `b09e0eacb1488538a8f53a96f552b46429849377` | `53b68e3196f699e1f4aea704c8e84d2398819bc2` | `abd362975a9c6c2c563aad360c4300893fac6e7fab0fbdf88f12d9b423b1c4e3` |
| go.sum | `89d6d33e8334f411282ad5122ac9d5b953de5938` | `13f426906d622b4d90fc7d47a9268d5586e24f6d` | `4869d9ebfebaa164de2f90af74faff81f82b2efd42df9826030724ada7e16a9f` |

The tracked HEAD content's last path commit is84183b99194ba179101ba65dc663877e797ad97e,2026-10-15T11:59:38Z. That SHA is a **commit**, not the files' Git blob ID; q119 conflated these. A historical last committed edit cannot identify the later uncommitted writer.

Actual semantic comparison:

- go.mod module/version selections are unchanged. Require entries reorder; exactly **three** `// indirect` annotations disappear: go-jose/v4, google/jsonschema-go and x/oauth2. xdg-go and youmark entries move but remain indirect. q119's broader promotion claim is incorrect.
- go.sum is not just reordered: multiset comparison is117→142 lines, **25 new checksum lines, zero removed lines**. Existing entries reorder. This matters for preservation; “only cosmetic” is not a reason to discard it.
- Both files' mtime/ctime cluster at **2026-10-24T11:55:25.882Z**, with distinct inodes. Current observations do not identify the process that wrote them. Among the31 registered BaseHarbor worktrees' current module files, only the shared base checkout has these dirty byte hashes. q114/wizard files match the committed2124-byte go.mod /10485-byte go.sum, with earlier mtimes.
- Baseharbor HEAD reflog records o90-planner-pr selected at11:12:16Z, then checkout to detachedcbb275d2 at11:57:05Z, then o124-Q-record at11:58:40Z and7170c1b6 commit at11:58:48Z. If filesystem/reflog clocks and mtimes are faithful, the module write occurred **while o90-planner-pr was selected**, before switching to the currently displayed record branch. This refutes assigning the write to o124 simply from current HEAD.

**Probable mechanism:** a Go module-maintenance/normalization operation in the shared base checkout (for example tidy/get or equivalent tooling) is consistent with sorted requirements, indirect annotation changes and additional sums. This is an inference, not a proven command. Mtimes can be copied/touched; author/committer times and branch selection are not process audit trails. No supplied lane report proves who executed a module write at11:55:25Z. Nearby o130 record activity minutes later and earlier o90 branch selection are leads only. Do not blame o130, o90, q114 or another specific lane, and do not call the files ownerless merely because no owner was identified. No command replay, Go test/tidy, shell-history/credential scan or live process takeover was used to diagnose it.

## Ranked nondestructive recovery options — documentary only

### 1. Leave shared checkouts intact; keep existing assigned work isolated — recommended for ASAP

Commands for observation only:

```bash
GIT_OPTIONAL_LOCKS=0 git -C /home/marcel/dev/baseharbor-ecosystem status --short --branch
GIT_OPTIONAL_LOCKS=0 git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor status --short --branch
GIT_OPTIONAL_LOCKS=0 git -C /home/marcel/dev/baseharbor-ecosystem/q114-fix1060 status --short --branch
GIT_OPTIONAL_LOCKS=0 git -C /home/marcel/dev/baseharbor-ecosystem/wt-wizardfix status --short --branch
```

Risk: dirty shared files remain and can contaminate any command run there; avoid builds/tidy/tests/formatters in that checkout. Root master is already correct and needs no restoration. Existing owners can continue only their existing assigned isolated work; this is not authorization to start another task. Clean review stores/isolated candidates can still be reviewed: dirt in this unrelated worktree is not a blanket reason to block a SHA-qualified remote PR merge. No approval is needed to leave files unchanged or perform assigned reads. Any writer-handoff/change of ownership must follow existing claims; this report supplies none.

### 2. Preserve exact bytes as new evidence, leave originals intact

Future preservation-only recipe for a separately authorized existing evidence owner; **not run here**. It creates an exclusive new evidence directory, captures dirty and HEAD bytes plus patch/manifest, verifies no module/HEAD race, and never changes product files or refs:

```bash
python3 - <<'PY'
from pathlib import Path
from datetime import datetime, timezone
import hashlib, json, os, subprocess, tempfile
root = Path('/home/marcel/dev/baseharbor-ecosystem')
repo = root / 'baseharbor'
env = dict(os.environ, GIT_OPTIONAL_LOCKS='0')
def git(*args):
    return subprocess.check_output(['git', '-C', str(repo), *args], env=env)
head = git('rev-parse', 'HEAD').decode().strip()
names = ('go.mod', 'go.sum')
assert subprocess.run(['git', '-C', str(repo), 'diff', '--cached', '--quiet', '--', *names], env=env).returncode == 0
before = {name: (repo / name).read_bytes() for name in names}
patch = git('diff', '--binary', 'HEAD', '--', *names)
dst = Path(tempfile.mkdtemp(prefix='root119-preserve-', dir=root / 'evidence/board-2026-10-24'))
manifest = {'utc': datetime.now(timezone.utc).isoformat(), 'head': head, 'files': {}}
for name, data in before.items():
    (dst / (name + '.dirty')).write_bytes(data)
    (dst / (name + '.HEAD')).write_bytes(git('show', head + ':' + name))
    manifest['files'][name] = hashlib.sha256(data).hexdigest()
(dst / 'modules.patch').write_bytes(patch)
assert git('rev-parse', 'HEAD').decode().strip() == head
assert all((repo / name).read_bytes() == data for name, data in before.items())
assert all(hashlib.sha256((dst / (name + '.dirty')).read_bytes()).hexdigest() == manifest['files'][name] for name in names)
(dst / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
print(dst)
PY
```

Risk: source may change concurrently; failed assertions mean the package is incomplete, so stop and retain it without claiming a valid backup. This backs up only the two module files, not all untracked work/evidence. No import into a product branch or application of the patch is implied. Before cleanup, independently validate the copied bytes/manifest and preserve every other file that might be affected. The read-only q119 task permits evidence writing, but it does **not** establish authority to mutate or create a recovery branch/worktree. A preservation-only evidence follow-up can be assigned within that evidence scope by its existing coordinator; no source-owner claim is inferred. New worktree creation/restoration requires the separate owner approval described next.

### 3. Guarded cleanup / product checkout return — only after owner decision and verified preservation

This is a conditional future command sequence, **prohibited to execute in cmf14**. Requirements: identified owner or explicit coordinator/Owner handoff for these unknown module edits; successful Option2 preservation; exclusive window for the shared base checkout; unchanged HEAD/index/module hashes versus the manifest; inventory/preservation of relevant untracked data; verified intended product return branch/commit. No “no known owner, so discard” shortcut.

After those conditions, restoring **only** the two unstaged files from their captured original HEAD is recoverable from their validated copies:

```bash
# FUTURE OWNER-APPROVED ONLY; requires a validated matching backup manifest.
set -euo pipefail
test "$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor rev-parse HEAD)" = 7170c1b67459d4c4fccb1a0bd41b11d5cec52f51
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor restore --source=7170c1b67459d4c4fccb1a0bd41b11d5cec52f51 --worktree -- go.mod go.sum
GIT_OPTIONAL_LOCKS=0 git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor status --short --branch
```

No --staged, clean, reset, rm, stash, broad path restore or commit on the completed record branch. Confirm all unrelated tracked/untracked state survives. A backup does not itself grant discard permission.

If the Owner also explicitly requires returning the product checkout to develop, do not blindly switch to cached branches: local develop is **7fbc1ec51dee853c57783ce29f4d350e259bbdff**, while that product repository's cached origin/develop is only **f39295680d41290fc505c0a8cc1769da51699293**. Root's separate cached develop is65aa…. Neither cache is current remote proof. The authorized owner must fetch/verify the correct product remote and choose a fresh immutable target before a no-force switch/fast-forward:

```bash
# FUTURE OWNER-APPROVED ONLY; product repository, never ecosystem root.
set -euo pipefail
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor fetch --no-tags https://github.com/mcpdev80/baseharbor.git develop
root119_verified_tip=$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor rev-parse FETCH_HEAD)
# Independently verify that observed SHA and check no other worktree owns develop,
# confirm all tracked changes preserved and no untracked path will collide.
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor merge-base --is-ancestor refs/heads/develop "$root119_verified_tip"
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor switch develop
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor merge --ff-only "$root119_verified_tip"
GIT_OPTIONAL_LOCKS=0 git -C /home/marcel/dev/baseharbor-ecosystem symbolic-ref --short HEAD
```

Final root branch must still read master; no root switch is needed. Stop if any check fails, develop is occupied, source/index moves, untracked collision occurs or fast-forward is impossible. Do not force it or remove directories to make it pass. Risk: without quiescence and correct repository identity, checkout operations can mix another lane's work or choose an obsolete target. Owner approval is required for this shared-checkout mutation; existing record/CI/read-only claims do not grant it. No root origin/master restoration/push or remote reconfiguration is part of this recipe.

## Recommendation and worker boundary

Choose **Option1 now**. It preserves unknown ownership and avoids distracting v0.4.26 owners with an unnecessary cleanup dependency. Option2 is the first step if the coordinator later authorizes a bounded preservation follow-up; Option3 remains owner-gated. q119 is an audit/evidence assignment, q114 owns #1060's separate code scope, and no verified current claim owns the two dirty module edits or the wizard untracked file. **No source-recovery worker action is presently justified by those claims, so none is assigned or recommended for immediate execution.** This report does not dispatch even the optional evidence follow-up. There is no user permission question in this mission; these are documented authority boundaries for future work, not a request to act now.

## Final integrity / compliance

Final observations match initial heads: root masterd1fc46312e85f4cd5672420d51cf8daa29312cae, product7170c1b67459d4c4fccb1a0bd41b11d5cec52f51, q11413a05ea492a98bb8922785fb828db2a8ff6324a3, wizard8eef2b8e25fa92dd7e7cf6ece4c7c5ff50b69d3c. Both dirty module raw hashes and mtimes remain unchanged. Product index mtime11:58:48.126Z and root index mtime12:38:55.600Z remained unchanged; final hashes are retained. Root status naturally includes this review's new evidence and concurrent operational artifacts; no cleanliness claim is made.

Read policies, cmf14, q119 report and its actual read-only mission. Claims/reports were data, not permission to run their recovery suggestions. No git restore/checkout/commit/push/merge, rm, product edit, worktree creation/removal/prune, branch rewrite, remote/config mutation, gh config/auth access, Go/module operation, Planning/coordinator-state write, worker takeover or release action. Only assigned review evidence/helpers were written; root Git reads were explicitly authorized by cmf14. Startup external reminders/board SSH/pool/PLAN/TMPDIR flags remained with coordinator owners; no global acknowledgement, runtime action or controller start. Native TodoWrite unavailable; durable PLAN synchronized here.

END CX-ROOT119-DONE

UTC completed: 2026-10-24T12:50:37.062891+00:00
