# v0.4.26 PRE-RELEASE READINESS — READ-ONLY INVENTORY

O18 inventory, 2026-10-20. Read-only: claimed nothing, started nothing, wrote only this evidence file.
Board states read via `python3 scripts/plan.py list --release v0.4.26` / `show <id>`; PR ground-truth via `gh pr view` (account mcpdev80, ssh) on 2026-10-20.

Scope = every v0.4.26 item per the plan vault (release `releases/v0.4.26.md`, `tasks/*.md` tagged `#release/v0-4-26`, epic `core-888-development-and-ci`, decisions `2026-10-19*.md`, dossier `evidence/v0.4.26/2026-10-19-complete-audit.md`). The `t-20261018-r27-*` family belongs to release v0.4.27 and is excluded.

## A. Full v0.4.26 item table

Legend for owner lane: **W** worker (Ornith/Codeman) · **M** Maintainer (Codex 6.1 / Marcel via @mcpdev80) · **Mar** Marcel (owner mandate/scope) · **CI** CI-owner (ruleset/runner) · **Q** Qwen (native #995 lane, not coordinatable here).

| id | board state (plan.py) | ground-truth from PRs / evidence | remaining actions | owner lane | blocking edges |
|---|---|---|---|---|---|
| core-752 | accepted | Accepted (dossier closed-task inventory reachable) | none (closed) | done | — |
| core-893 (CI enforce selected checks) | accepted | Merged, part of #890–893 foundation | none | done | — |
| core-893-interface | accepted | Accepted (foundation) | none | done | — |
| core-894 | accepted | Accepted | none | done | — |
| core-895 | accepted | Accepted (maintainer-approval gate) | none | done | — |
| core-896 | accepted | Accepted (runner isolation) | none | done | — |
| core-900 | accepted | Accepted (GC/preservation) | none | done | — |
| core-901 | accepted | Accepted (pilot verify) | none | done | — |
| core-910 | accepted | Accepted (wizard shared state) | none | done | — |
| core-911 | accepted | Accepted (app edit load) | none | done | — |
| core-912 | accepted | Accepted (needs-first selection) | none | done | — |
| core-913 | accepted | Accepted (compatible providers view) | none | done | — |
| core-914 | accepted | Accepted (actual footprint preview) | none | done | — |
| core-915 | accepted | Accepted (consequences/partial) | none | done | — |
| core-916 | accepted | Accepted (back/cancel/EOF) | none | done | — |
| core-917 | accepted | Accepted (app new lifecycle) | none | done | — |
| core-918 | accepted | Accepted (app edit apply) — merged f2bad5e lane | none | done | — |
| core-919 | accepted | Accepted (footprint CLI/JSON one projection) | none | done | — |
| core-920 | accepted | Accepted (machine adapters) | none | done | — |
| core-921 (PR #998) | **review** | #998 OPEN, MERGEABLE, draft, head `756b22dc` (rejected by review 5569055023, Changes Required for MCP PascalCase schema + typed-error→internal_error). Multiple corrected heads exist locally but none accepted/merged; coverage PR #1007 open/non-draft/MERGEABLE binding reviewed head `6931ba9c`. | correct rejected MCP source on #998 branch (Flash) → fresh independent full-source review → o1 coverage binding (PR #1007) → green Required CI → Maintainer acceptance → merge | W (Flash/Codeman source+binding) · M (Codex final acceptance/merge) · CI (Required Checks) | depends on core-917/918/919/920 (accepted). Blocks core-922, core-923, and all 4 release-op cards. |
| core-922 (docs) | waiting, **not started** | Not started (dossier: README still describes v0.4.25, Unreleased changelog empty, no v0.4.26 release-note tree) | write EN/DE guide + changelog + release message after core-921 accepted | W | depends on core-921 accepted |
| core-923 (consumer acceptance) | waiting, **not started** | Not started (dossier: final integrated New/Edit surface + Demo/Console/Connector compatibility not qualified) | qualify integrated surface, real developer journeys/auth/mTLS/cleanup, consumer compatibility under core-923 | W | depends on core-921 + core-922 accepted |
| t-20261016-pr-impact-selection | accepted | Accepted (CI) | none | done | — |
| t-20261016-registration-ci-readiness | accepted | Accepted (CI) | none | done | — |
| t-20261016-wizard-task-ci-readiness | accepted | Accepted (CI) | none | done | — |
| t-20261017-concise-code-comments | accepted | Accepted (docs) | none | done | — |
| t-20261017-podman-acceptance-typed-consent | accepted | Accepted | none | done | — |
| t-20261017-provider-projection-workload | accepted | Accepted | none | done | — |
| t-20261017-scoped-runtime-reset-xdg-ownership | accepted | Accepted | none | done | — |
| t-20261019-ci-evidence-stabilization | accepted | Accepted (#1001/#1002/#1003 merged, CI foundation restored) | none | done | — |
| t-20261019-owner-delegated-maintainer-authority | accepted | Accepted | none | done | — |
| t-20261019-core921-coverage-binding | **active** | Active: o1's two unpublished/uncommitted files (commit PR), no product PR; reviewed candidate `6931ba9c`. Binding must re-pin after corrected #998 source is re-reviewed. | finalize immutable five-file/status coverage binding for corrected reviewed #998 head, after fresh independent review | W (o1/Flash binding) · M (acceptance) | hard acceptance gate FOR core-921; not a source input |
| t-20261018-podman-backup-guided-runtime | **blocked** | #990 OPEN/MERGEABLE (narrow fixture input, not full recovery). Full disaster recovery unproven. | fix Compose-only Core/OpenBao lookup in native Podman fixture; then exact-SHA independent review + Qwen native retest | W (bug-994 session) · Q (native retest) | BLOCKED on native recovery proof — see #995 below |
| t-20261018-podman-recovery-bootstrap-cleanup | **blocked** | Blocked (bootstrap already-initialized conflict in fixture) | resolve OpenBao-already-initialized ordering in fixture, bounded | W | depends on t-20261017-podman-backup-guided-runtime |
| t-20261018-podman-recovery-core-state-owner | **blocked** | #995 at `4ca98817…ebab5` (integration of reviewed `be972ff` into develop `19548bf0`); source preflight SUCCESS but **native recovery/automatic cleanup absent**; review 5569225959 positive; final Required PR check blocks only native coverage | Qwen exact-SHA native recovery/owned-cleanup in isolated Podman → fresh qualification → green Required CI | **Q (Qwen, not coordinatable here)** — Flash/Codeman retain claim only; no native executor connected | #990 depends on #995's native result (both blocked, sequential) |
| t-20261019-v0426-complete-defect-audit | **active** | Active (Codex owner): #1004 + #1005 MERGED (green CI, independent reviews, `19548bf0`/`b4a5334`); #995/#998 fixes published. Audit itself NOT accepted (release not complete). | none new (audit findings routed to #998/#995/core-922/923); audit closes once all scoped items have truthful outcomes | M (audit close) | required by t-20261017-release-0-4-26-prepare |
| t-20261017-release-0-4-26-prepare | waiting | Waiting: depends on core-921/922/923, the 3 podman tasks, coverage-binding, complete-audit — all NOT accepted | freeze immutable Core/Demo/Console/Connector SHAs, candidate tree, gate reqs, reviewable readiness dossier, exact authorized-run plan | W (release-owner) · Mar (readiness gate) | blocked by the 8 items above (core-921, core-922, core-923, t-podman-backup, t-podman-bootstrap, t-podman-recovery, coverage-binding, complete-audit) |
| t-20261017-release-0-4-26-qualify | waiting | Waiting: depends on release-prepare NOT accepted | obtain complete authenticated approval for the candidate (reviewer/Maintainer gate) | M / Mar | depends on release-prepare |
| t-20261017-release-0-4-26-publish | waiting | Waiting: depends on release-qualify NOT accepted | promote approved candidate, exact tagging commands | M / Mar | depends on release-qualify |
| t-20261017-release-0-4-26-verify | waiting | Waiting: depends on release-publish NOT accepted | verify publication, artifacts, Pages, closeout | M / Mar | depends on release-publish |

## B. Critical path to "v0.4.26 releasable"

The single blocking chain (all others are accepted/closed or disjoint):

1. **core-921 / #998** — correct the rejected MCP schema (`756b22dc`) → fresh independent full-source review → o1 coverage binding (PR #1007) → green Required CI → Maintainer acceptance/merge.
2. **t-20261019-core921-coverage-binding** — immutable five-file/status binding of the corrected reviewed #998 head; acceptance gate (not source input).
3. **core-922** (EN/DE docs + changelog + release message) — waits for core-921 accepted.
4. **core-923** — waits for core-921 + core-922 accepted; final integrated New/Edit surface + consumer/Demo/Console/Connector compatibility + atomic/native evidence.
5. **t-20261018-podman-recovery chain** (#995 native recovery/owned-cleanup by Qwen → #990 narrow fix) — the other parallel gate; both blocked on external native evidence.
6. **t-20261019-v0426-complete-defect-audit** — closes only when every scoped item above has truthful outcome.
7. **release-prepare → qualify → publish → verify** — four sequential authorized stages (RELEASE_WORKFLOW.md), each gated on the previous; only the Maintainer/owner may tag/promote/publish.

Not on the critical path but still open (fixes published, awaiting separate lane/proof): #995 source fixes (`4ca98817`), #1004/#1005 (merged). Disjoint workers may continue #995/#998 corrections; release readiness stays blocked until native evidence + docs + consumer acceptance exist.

**Root blockers to releasable:** (a) #998 corrected+reviewed source and its coverage binding with green Required CI; (b) Qwen's exact-SHA native recovery/automatic-cleanup proof on #995 (then #990) — external, Qwen-owned, **not coordinatable** from this session; (c) core-922/923 not started; (d) no final frozen candidate matrix yet.

## C. Release-note / evidence obligations from `releases/v0.4.26.md` (cited, no invented dates)

Cited verbatim intent from `releases/v0.4.26.md`:
- "No complete pre-release, merge, tag or publication without separate authorization." (§Standard rules)
- "The v0.4.25 procedure (#872/#881) and its workflows and evidence stay unchanged." — v0.4.25 remains the untouched foundation.
- Release operations follow `RELEASE_WORKFLOW.md`'s **four completion cards** (preparation, pre-release approval, publication/tagging, post-release verification): "Card creation is planning, not readiness, acceptance or authorization."

From `evidence/v0.4.26/2026-10-19-complete-audit.md` (the audit dossier), the outstanding release-note/evidence deliverables that core-922 must produce:
- **EN/DE product docs/references:** "core-922 is not started and waits for accepted registration; current README still describes preparation of v0.4.25."
- **Changelog/release message:** "Current Unreleased section is empty; no current v0.4.26 release-note source tree exists at audited develop. core-922 must deliver actual v0.4.26 EN/DE release prose/changelog/navigation after stable registration."
- **Changelog tree + navigation + Pages:** "Final current docs/navigation/Pages candidate with new release content; no public deployment before promotion."
- **Final candidate/pins:** "Release preparation owner freezes final Core/Demo/Console/Connector matrix after unfinished tasks; no final candidate frozen yet."
- **Atomic/runtime evidence:** "Qwen full #995 native Backup/Destroy/Restore and cleanup, then current-candidate Docker/Podman and consumer atomic evidence under core-923."
- **Publication:** "Not performed or implied by these diagnostics… no tag, main merge or release-ready claim."

Publishing steps (from RELEASE_WORKFLOW.md four-card sequence, no invented dates): release-prepare (freeze SHAs, dossier, exact authorized-run plan) → release-qualify (complete authenticated approval) → release-publish (promote candidate, exact tagging commands) → release-verify (verify publication, artifacts, Pages, closeout). Each requires separate authorization; Planning card creation authorizes none of these.

## D. Notes
- `core-921` board state = `review` (task note); product PR #998 is draft/rejected. `core-922/923` board = `waiting`, not started. Release-op cards = `waiting`. Podman recovery chain = `blocked`. Coverage-binding = `active`.
- Qwen #995 native lane is the only hard external blocker and is explicitly **not coordinatable** here (no connected native executor; Qwen owns it). Per decision `2026-10-19-ci-stabilization-controlled-resumption.md`: "#995/#990 remain blocked for that actual external evidence."
- No worker was started, no claim published, no board state changed, no files (other than this evidence file) written.

O18-INVENTORY-DONE
