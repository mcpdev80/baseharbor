# F6 — Demo thread cross-audit (READ-ONLY)

LANE: F6 — cross-audit of q14/q15 cleanup thread + HANDOFF 'Demo handoff' claims vs branch content
DATE: 2026-10-22
MODE: READ-ONLY (fetch + git grep/show/log only; zero writes to any repo, zero pushes, zero merges)
REPO: /home/marcel/dev/baseharbor-ecosystem/baseharbor-demo
REFS AUDITED: origin/main @ `8b26585fa9c6b6c541216f7f4a63270fa4410462` (q15 tip) vs origin/integrated/v0426-new-edit-native @ `7bfb157ea80380220929d12afc90b328c64f3989`
INPUTS: q14-podman-cleanup-prov.md (root cause), q15-demo-cleanup-race.md (fix+RESOLUTION), HANDOFF.md 'Demo handoff', f5-demo-gates.md (gate receipts)

## PLAN

- [x] 0. Write PLAN to todowrite + this evidence file (WRITE-FIRST)
- [x] 1. Fetch origin; verify refs exist; record SHAs — main tip = 8b26585 (2026-10-22 18:50+02), branch tip = 7bfb157 (18:57+02)
- [x] 2. (a) Diff cleanup accounting path 8b26585 vs branch: coherence of q15 conflict resolution vs q14 root cause
- [x] 3. (b1) e28 XDG retention claim: git grep XDG handling in both refs
- [x] 4. (b2) bb9eb892/e6ba5b45 lineage + invariant grep in both refs
- [x] 5. (b3) 56661ac9 fixtures wiring grep in both refs
- [x] 6. (c) Verdict per claim + post-Source-B native re-dispatch proof obligations (gate names)
- [x] 7. Compliance section + END F6-DEMOAUDIT-DONE

## Lineage ground facts

- `8b26585` IS the tip of origin/main; it is **NOT an ancestor** of origin/integrated/v0426-new-edit-native.
- Branch tip `7bfb157` "merge main: unify cleanup accounting" is a **single-parent linear commit** (parent `a9c6f0d`), despite its message "Merge origin/main into integrated/...". Genealogically, main was never merged into the branch; the "unification" is a hand-reapply. A future real merge of 8b26585 will conflict again on scripts/failure-runtime-diagnostics.sh.
- HANDOFF checkpoint SHAs `56661ac9`, `e28acf20`, `bb9eb892`, `e6ba5b45`, `2adc744d` do **not exist as objects** in this clone (local-only worktree commits, as HANDOFF itself states). Claim verification therefore proceeds at content-equivalence level only.

## (a) q15 conflict-resolution coherence: 8b26585 (main) vs 7bfb157 (branch)

Main side (8b26585): q15 patched the **legacy 69-line bash** failure-runtime-diagnostics.sh — a 3-retry `for attempt in 1 2 3` + 0.25s backoff around inspect; on persistent failure it prints "accounted with reason" to stderr, writes empty state/log files and **continues without failing** (the script has no errors channel). Main's acceptance.sh has **no** `cleanup-preparation --unpause-owned` call at all (old unpause-all loop). So q15's "fix" landed on a code path where q14's preparation=1 signature cannot even occur; it did NOT touch the path q14 identified (branch `owned_resources()`/`--unpause-owned`).

Branch side (7bfb157): keeps the branch's Python ownership framework and inserts one retry loop at failure-runtime-diagnostics.sh:150 (`for attempt in range(1,4)` + `time.sleep(0.25)`, `import time` present). Findings:

1. **TOLERANCE NEUTRALIZED (q14 defect survives in the unified branch code).** The auto-collection branch does `errors.append('owned container auto-collected by podman daemon — accounted with reason: '+identity)`. The script tail computes `'outcome': 'failure' if errors else 'success'` and `if errors: raise SystemExit(1)`. Any "accounted with reason" entry therefore still yields exit 1 → `cleanup_preparation=1` → `[acceptance] cleanup: incomplete (destroy=0 preparation=1 ...)` — exactly the q14 receipt. The comment claims "This is a legitimate state" but the accounting treats it as failure. q14 remedy option (c) explicitly required graceful handling **not as a failure**; the unified implementation does not do that.
2. **Retry placed too late / two earlier failure channels bypass it.** If the Quadlet daemon has fully removed the container before listing, `owned_containers()` raises `canonical runtime resources lack verified native ownership` (or the label-list raises `selected runtime label inventory failed`) → caught as `owned runtime evidence unavailable` → exit 1. The retry only helps for the narrow window "container listed but inspect fails yet still present as exited/dead". q14's race can end in the removed-before-list channel, which remains intolerant.
3. **NO duplicated retry logic within either ref.** Branch has exactly one retry loop (line 150); main has exactly one (bash line 45). But two *divergent* implementations of the same fix coexist **across** refs (bash vs Python), which is a re-conflict hazard at the real merge, not yet a duplicated code path.
4. **DROPPED evidence: q15's fixture test is absent from the branch.** `tests/debug-podman-cleanup-accounting/run.sh` exists only in origin/main; the "unify" commit carries it in as a 97-line deletion. Branch `scripts/test_owned_runtime_diagnostics.py` contains **no** `auto-collected` case (grep empty). So no branch-side test asserts the preparation==destroy accounting under simulated early-collection.
5. **Unpause step NOT dropped:** `--unpause-owned` mode still checks `state['Paused']` and invokes native unpause (lines 175–179); auto-collected (exited/dead/removed) containers cannot be paused, so skipping them is operationally fine — the fault is the failure-accounting (finding 1), not a lost unpause.

**(a) VERDICT: NOT YET ONE COHERENT IMPLEMENTATION.** The branch's "unify" preserves q15's intent textually (retry + "accounted with reason" message) but routes it into the errors channel, so q14's `preparation=1` Podman failure signature can still occur; additionally q15's accounting fixture test was dropped from the branch and main was never actually merged (false-merge commit).

## (b) HANDOFF 'Demo handoff' claims vs branch content

### b1. e28 XDG retention — PRESENT in branch; PARTIAL in main

- Branch: `acceptance.sh:43-44` exports fixture-isolated XDG for Core/baha; `acceptance.sh:62` and `install-baseharbor.sh:27` strip **only** `XDG_CONFIG_HOME`/`XDG_DATA_HOME` for native podman (host storage retained); `failure-runtime-diagnostics.sh:39-40` pops exactly those two keys for `native=True` runs while `run()` without `native` keeps `os.environ` (isolated fixture XDG) — plus a guard at :31-33 rejecting a runtime owner XDG outside the isolated fixture. Matches the e28 claim precisely.
- Main: same `env -u XDG_CONFIG_HOME -u XDG_DATA_HOME` pattern in acceptance.sh/install script, but no Python-side ownership/XDG guard exists (legacy script). Content-equivalence of e28 confirmed for the branch; SHAs absent (local-only).

### b2. bb9eb892/e6ba5b45 cleanup checkpoint lineage — CONTENT PRESENT in branch; ABSENT from main; NO commit lineage

- Branch acceptance.sh: `trap cleanup EXIT` + `trap 'exit 130' INT` + `trap 'exit 143' TERM` (123–125) and `trap - EXIT; trap '' INT TERM` inside cleanup (72–73) → original EXIT/INT/TERM preservation + re-signal quiesce; bounded `--unpause-owned` preparation (80); truthful four-counter status `destroy/preparation/foreign-check/foreign-remove` (103–106); actual teardown → foreign-check → separate exact-owner foreign-remove (89, 97–98); failure-time diagnostics only when status≠0 (81–83).
- Branch failure-runtime-diagnostics.sh: exact read-only Core preview `baha destroy --all --json` with `preview/all/destroyed` validation (`owned_resources()`), verified native label+name ownership **before** any inspect/log/unpause (`owned_containers()`), bounded process groups (`start_new_session=True` + killpg), bounded output/redaction via `redactor`/`retain()`, `scope-result.json` outcome receipt. Unit corpus `scripts/test_owned_runtime_diagnostics.py` (323 lines) present.
- Main: none of this — only `trap cleanup EXIT` (120); no --unpause-owned, no scope-result, no foreign accounting. The checkpoint SHAs are not objects in either ref → lineage claim is **squashed-equivalence only**, consistent with HANDOFF's "local-only".
- Caveat: the "truthful failure status" invariant is truthful, but per (a)-finding-1 it now truthfully reports failure for a state q15 declared legitimate — the contradiction must be resolved before native re-qualification.

### b3. 56661ac9 fixtures wired — PRESENT in branch; ABSENT from main

- Guided: `tests/guided/run.sh:155` invokes `tests/guided/new_edit.py` (284 lines; real No/Cancel/EOF terminal path, "No/Cancel changed source, Target state or native resource inventory" guard at :106, actual Cancel via Esc at :171-173, never controller injection).
- Data-capabilities: `tests/data-capabilities/run.sh:43` invokes `tests/data-capabilities/new_edit.py` (357 lines; SQL flow, foreign paused sentinel with owner/permission/immutable-identity guards, "Core teardown changed a foreign paused container or its volume" at :76, private receipt symlink/size checks).
- MCP: `tests/mcp/run.sh:193-194` invokes `tests/mcp/new-edit-static.py` + `assert_no_secret_leak` (the "do not lose it" run wiring from HANDOFF's uncommitted note — it is present in the branch).
- Main: `git grep new_edit` in the three run.sh files → zero hits (rc=1). Fixtures exist only in the PR #116 lineage. SHA `56661ac9`/`2adc744` not objects; `a7a4a3a` "(handoff 2adc744 equivalent)" is the content-carrying commit.

## (c) Verdicts + post-Source-B native re-dispatch proof obligations

| Claim | Verdict |
| --- | --- |
| (a) one coherent cleanup-accounting implementation after q15 conflict resolution | **FAIL** — tolerance neutralized via errors channel ((a)-1); earlier removal channel un-tolerated ((a)-2); false-merge lineage + dropped branch-side accounting test ((a)-4) |
| b1 e28 XDG retention | **PASS (branch)** content-equivalent; main partial by design (legacy path) |
| b2 bb9eb892/e6ba5b45 invariants | **PASS (branch)** content-equivalent, EXIT/INT/TERM + redaction + ownership-before-inspect + truthful accounting present; **no commit lineage** (local-only); truthful-status invariant currently exposes the (a)-1 contradiction |
| b3 56661ac9 fixtures wired | **PASS (branch)** guided/data/MCP wiring + sentinel + No/Cancel/EOF present; **absent in main** as expected (PR #116 not merged) |

Before native re-dispatch, fix in-branch: (i) make "accounted with reason" a non-error (separate `accounted` list; outcome failure only for real errors), (ii) make `owned_containers()` tolerate verified auto-collection between preview and listing, (iii) restore accounting-race coverage in `scripts/test_owned_runtime_diagnostics.py`, (iv) perform a genuine `git merge origin/main` (8b26585 ancestor) so main's legacy bash retry is superseded once, in one resolved file.

Post-Source-B (core `99901ff` terminal-Name-delegation fix integrated into final frozen Core) native re-dispatch must prove, per F5 §PENDING-FREEZE binding-proof set (`validation/pre-release/selected/gates/<exact gate IDs>` push format + core `targeted-demo-acceptance.yml` dispatches, demo_ref = merged post-#116 main):

1. **docker · guided** and **podman · guided** — PASS: no `new-edit-new-no` 90s no-output hang (Source-B's own defect class).
2. **docker · data-capabilities** and **podman · data-capabilities** — PASS: no `new-edit-new-sql` hang; SQL/sentinel/foreign rows green.
3. **static · mcp** — remains PASS with new-edit-static wiring.
4. **backup-restore Docker+Podman (demo push trigger, "Targeted Demo compatibility")** — the decisive cleanup proof: `[acceptance] cleanup: complete` with `destroy=0 preparation=0 foreign-check=0 foreign-remove=0` **on podman**, i.e. q14's signature absent even when Quadlet auto-collection races, plus retained artifacts `demo-v024-podman-*` + `scope-result.json` showing `outcome: success`.

## repo-rule compliance

- READ-ONLY lane: fetch + `git show/log/grep/ls-tree/merge-base` only; zero edits to baseharbor-demo/baseharbor worktrees, zero pushes, zero merges, no claims taken, PR #116 untouched; repo dirty state unchanged (pre-existing untracked `tests/__pycache__/` only).
- Read: AGENTS.md (ecosystem + demo lane context), q14/q15 evidence, HANDOFF 'Demo handoff' §, F5 gate receipts. No credentials or secrets logged.
- Findings are against verified object content under both refs; sandbox-side claims were re-verified against the neutral repo state per Evidence rules.

END F6-DEMOAUDIT-DONE
