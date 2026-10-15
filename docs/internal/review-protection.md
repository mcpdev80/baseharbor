# Review protection proposal for develop (#895)

Proposal only. Nothing in this document is activated. #890 is integrated; #893
(Session 1, PR #930) still needs qualification. Central process rules live in
`docs/DEVELOPMENT_GUIDELINES.md` ("Development process") and are not duplicated
here.

## Verified live state (2026-10-15, develop `f16ca948`)

- API: `X-GitHub-Api-Version-Selected: 2022-11-28`; REST ruleset documentation
  fetched with header `X-GitHub-Api-Version: 2026-03-10`.
- Repository `mcpdev80/baseharbor`: id `1360667669`, **public** (CODEOWNERS and
  code-owner enforcement are available); `default_branch: main`; PR-only
  integration on develop relies on the ruleset `pull_request` rule.
- `develop` is protected by repository ruleset **23754541**
  ("develop - integration branch", `enforcement: active`,
  conditions `refs/heads/develop`), currently containing rules `deletion`,
  `non_fast_forward`, `required_linear_history` and
  `pull_request(required_approving_review_count: 0, dismiss_stale_reviews_on_push:
  false, require_code_owner_review: false, require_last_push_approval: false,
  required_review_thread_resolution: true, require_extra_approval_for_unattributed_changes:
  true, allowed_merge_methods: ["squash"])`. `bypass_actors: []`,
  `current_user_can_bypass: never`.
- Classic branch protection on `develop`: absent (`404 Branch not protected`);
  rulesets are the only mechanism. Org rulesets: not applicable (404).
- CODEOWNERS: absent repo-wide (root, `.github/`, tree scan).
- `main` ruleset 23754542: active, review count 0, no required status checks;
  outside #895 scope.
- The #893 final completion check is named **`PR Selected Checks`**; it exists
  on PR #930 head `ca2653cb` as a check run produced by app
  `github-actions` (integration id `15368`) with conclusion `failure`
  because runtime qualification is incomplete. This is an **activation
  prerequisite**, not a missing check name.

## Proposed minimal configuration (not applied)

Two separate rulesets keep review policy and CI policy independently
protected. Payload files live next to this document:

1. `develop-integration.ruleset.json` — update of ruleset **23754541**
   (`PUT /repos/mcpdev80/baseharbor/rulesets/23754541`):
   - `pull_request`: `required_approving_review_count: 1`,
     `require_code_owner_review: true` (with the new `.github/CODEOWNERS`
     entry `* @mcpdev80`, the approving review must come from the Maintainer
     themselves, not from an arbitrary reviewer),
     `dismiss_stale_reviews_on_push: true` and `require_last_push_approval: true`
     (new commits invalidate prior approvals); existing integrity rules
     (`deletion`, `non_fast_forward`, `required_linear_history`),
     thread-resolution and squash-only merge methods are preserved.
     `require_extra_approval_for_unattributed_changes: true` is carried over
     verbatim from the live ruleset (`GET` response); it is not documented in
     the PUT parameter list of the current REST reference, and omitting it
     from a PUT body would silently downgrade the existing protection.
   - `bypass_actors`: one entry
     `{actor_id: 22537373, actor_type: "User", bypass_mode: "pull_request"}`.
     Per current REST docs `User` is a supported actor type and
     `pull_request` is a supported bypass mode for branch rulesets, so the
     Maintainer can merge pull requests without waiting for a review of their
     own PRs.
2. `develop-required-checks.ruleset.json` — new ruleset
   (`POST /repos/mcpdev80/baseharbor/rulesets`) with
   `required_status_checks` requiring context `PR Selected Checks` bound to
   `integration_id: 15368`, `strict_required_status_checks_policy: true`, and
   `bypass_actors: []`. Because bypass actors are declared per ruleset and the
   CI ruleset has none, a red required check stays binding for everyone,
   including the Maintainer — the green-check requirement is configuration,
   not a process-only policy.

Rationale for using CODEOWNERS instead of the ruleset `required_reviewers`
field: the API documents `required_reviewers` as beta and Team-only; a
user-owned repository has no teams, so code-owner review is the supported
targeted mechanism for binding approval to @mcpdev80.

Trusted source binding: `integration_id: 15368` ensures the check originates
from the GitHub Actions integration. The integration id alone cannot identify
a specific workflow file, so protection against workflow self-modification
comes from CODEOWNERS over `/.github/workflows/` (any PR that alters the
check's definition needs the Maintainer's code-owner approval) and from
keeping the check logic on protected branches; a matching display name alone
is explicitly insufficient.

## Honest limits

- A Maintainer `pull_request` bypass is **not technically restricted to the
  Maintainer's own PRs**: it lets that actor bypass the review ruleset on any
  PR. This remains the documented solo-maintainer compromise from #888;
  the CI ruleset is deliberately bypass-free so green checks still apply.
- Bypass audit entries show who bypassed what; self-discipline plus the
  pilot evidence in #901 carry the remaining risk.
- Direct push to `develop` stays blocked (`non_fast_forward`, `deletion` and
  the PR-merge semantics of `pull_request`); no contributor-wide bypass.

## Activation prerequisites (ordered)

1. #893 (PR #930) merged and `PR Selected Checks` reliably green on the
   integration base; the current `failure` is the runtime-qualification
   gate, so do not activate before it is qualified.
2. Code-owner review available for the public repository (verified: repo is
   public; CODEOWNERS supported).
3. Marcel reviews and merges this proposal PR, then applies the two payload
   files via the documented `gh api` calls and runs the verification plan
   below. Activation itself is a separate, explicitly approved step.

## Verification plan (no production merges)

Run on a disposable branch/PR flow after activation; verify then close:

- Foreign PR into `develop` **without** approval → merge blocked by
  review/code-owner rule (`mergeable` blocked; API
  `GET /repos/.../pulls/{n}/mergeability`/status shows the reason).
- Same PR **with** @mcpdev80 code-owner approval and green `PR Selected
  Checks` → mergeable (verify, do not merge, close).
- New commit after approval → approval dismissed
  (`dismiss_stale_reviews_on_push`), merge blocked again.
- Maintainer's own PR, checks green, no review → mergeable via the
  `pull_request` bypass; the bypass audit entry must be visible.
- Any PR with a **red** `PR Selected Checks` → blocked for everyone,
  including the Maintainer (proves CI protection is bypass-free).

## Local validation status

Payload JSON files parse and match the documented rule/parameter names of the
REST schema (`X-GitHub-Api-Version: 2026-03-10` docs page; checked field
enums locally); CODEOWNERS syntax validated against GitHub's format.
Enforcement of the payloads was **not** exercised against live rulesets in
this proposal; that qualification belongs to the activation step after #893.
