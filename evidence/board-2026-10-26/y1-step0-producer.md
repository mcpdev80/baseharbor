# y1 Step 0 — Shadow Producer Run (Evidence)

Mission: `ops/missions/y1.txt` — CHAIN-RUNNER Schattenlauf, step 0-2 (x3 command plan).
Role: CHAIN-RUNNER, shadow-run step 0 producer base only. Steps 1+2 NOT executed
locally (per mission item 3): only step 0 + artifact evidence.

Author: Codeman, AgentGateway mission lane for `ops/missions/y1.txt`.
UTC host time confirmed via turn-start (`basics-check.py` at `evidence/board-2026-10-19/`);
host `date -u` monotonic, no clock-jump.
Write scope: this evidence file in `evidence/board-2026-10-26/` only.

## 0. Sources and ground truth

- `ops/missions/y1.txt` — task contract (step 0 producer + artifact evidence).
- `evidence/board-2026-10-26/x3-shadow-commandplan.md` — ABSCHNITT 1 Prechecks P0-P8.
- `baseharbor-plan/decisions/2026-10-26-minimal-receipts.md`, `t-20261026-r27-receipt-pilot` (waiting).
- Frozen receipt interface pinned from **ec82d476** in `baseharbor`.
- PR #1093 `probe(r27): shadow producer run`, base `develop`, branch `y1/shadow-probe`.
- CI run `40426322604` (`CI · Source & Quality`), event `pull_request`, head
  `af57a26d`, conclusion success — this is the witness run that emitted
  `trusted-pr-plan`.

## 1. Precheck list (P0-P8)

Run at 2026-10-27T00:15Z (host time via `date -u +%Y-%m-%dT%H:%M:%SZ`).

| # | Check | Command | Result |
|---|-------|---------|--------|
| P0 | Host UTC time sane | `date -u` + `basics-check.py` | OK — monotonic, no clock-jump flagged (basics-check.py at `evidence/board-2026-10-19/scripts/basics-check.py`; the `board-2026-10-26/scripts/` copy does not exist) |
| P1 | gh authenticated as mcpdev80 | `gh auth status` | OK — account `mcpdev80`, SSH git, token scopes `gist/read:org/repo` |
| P2 | PR1 (1090) accepted | `gh pr view 1090` | CHANGED: PR1090 now **CLOSED** (was OPEN at x3 plan time), `mergedAt=null` — **not merged**. Mission is the shadow producer run with its own y1/shadow-probe base; not gated on PR1 acceptance |
| P3 | Receipt shadow run exists? | `gh pr list ... "receipt in:title"` | none (`[]`) — no genuine PR2 run yet; correct for a shadow run |
| P4 | Protected Planning revision pinned? | `gh api .../branches/main --jq .protection.enabled` | `false` — Planning `main` protection disabled (known documented edge, per y1) |
| P5 | Frozen scripts present & runnable | `git show ec82d476:scripts/receipt_writer.py` | OK — `receipt_writer.py` present at ec82d476 |
| P6 | Clean Planning checkout | `git -C baseharbor-plan status --porcelain` | clean (empty) |
| P7 | Git author configured (planning) | `git -C baseharbor-plan config user.email` | `dev@pfingstgraef.eu` (non-empty) |
| P8 | gh can reach run provenance | `gh run view 40426322604` | OK — 200, `conclusion=success`, event `pull_request`, head `af57a26d` |

Abort-before-run gates (per x3 §1): P2 not accepted, P3 no genuine run, P4 no
protected Planning revision — all documented edges for the shadow producer run.
None block step 0 of the y1 mission, which builds its own producer base on a
dedicated branch.

## 2. Step 0 — producer base (genuine)

- `git -C baseharbor fetch origin` — OK.
- Branch `y1/shadow-probe` from `origin/develop` (`9a630665`) via worktree
  `worktrees/y1-shadow-probe`.
- Commit 1 (`f8b90703`): `docs/de/operations/schattenlauf-note.md` (harmless
  marker, 3 lines date+purpose).
- PR #1093 created — `probe(r27): shadow producer run`, base `develop`, body
  references v0.4.27 shadow producer baseline.
- First CI run (`documentation-audit`) **FAILED**: `operations/schattenlauf-note.md`
  is a DE page missing an EN counterpart.
- Fix (commit `4e2a5032` / `af57a26d`): added EN counterpart at
  `docs/operations/schattenlauf-note.md` (parity checker maps EN by path relative
  to `docs/`, DE by path relative to `docs/de/` — EN counterpart must live at
  `docs/operations/...`, not `docs/en/operations/...`).
- `scripts/documentation-audit.sh` passes locally; run #40426322604
  `PR · Source & Quality` = **SUCCESS** on final head.

## 3. Step 1+2 — NOT run locally (mission item 3)

Only artifact evidence reported:

- **run_id** (final CI run producing `trusted-pr-plan`): **40426322604**
  (`CI · Source & Quality`, event `pull_request`, head `af57a26d`, conclusion success).
- Artifact `trusted-pr-plan` downloaded via
  `gh run download 40426322604 --repo mcpdev80/baseharbor --name trusted-pr-plan
  -D /home/marcel/tmp-ops/y1-art` → `/home/marcel/tmp-ops/y1-art/pr-plan.json`.
- `plan_commit` (HEAD of the witness run): **`af57a26d610c79ff3bb55a5727b55db497fa7c1a`**
  — `probe(r27): relocate EN counterpart to docs/operations for locale parity`
  (2026-10-27 01:25:27 +0100).
- `integration_sha` (tested integration binding from plan fixture):
  **`381247041315127b48425b73de58f991aa143b26`**.

### pr-plan.json (content)

```json
{
  "broad_required": false,
  "coverage_status": "resolved",
  "head_sha": "af57a26d610c79ff3bb55a5727b55db497fa7c1a",
  "integration_sha": "381247041315127b48425b73de58f991aa143b26",
  "mapping_schema": "baseharbor.pr-gate-map/v1",
  "merge_base_sha": "eb617cf2fefea0b29c52a7654070c959b476ee96",
  "reasons": [
    "docs/de/operations/schattenlauf-note.md: documentation",
    "docs/operations/schattenlauf-note.md: documentation"
  ],
  "release": "v0.4.26",
  "required_validation_checks": [
    {"command": "bash scripts/documentation-audit.sh", "id": "documentation-audit", "owner": "scripts/documentation-audit.sh"},
    {"command": "mkdocs build --strict --config-file mkdocs.de.yml", "id": "mkdocs-de-strict", "owner": ".github/workflows/pages.yml:build"},
    {"command": "mkdocs build --strict --config-file mkdocs.yml", "id": "mkdocs-en-strict", "owner": ".github/workflows/pages.yml:build"},
    {"command": "build candidate documentation without deploying Pages", "id": "pages-candidate-build", "owner": ".github/workflows/pages.yml:build"}
  ],
  "schema": "baseharbor.pr-gate-plan/v1",
  "selected_gates": [],
  "target_base_sha": "9a630665bdbaeabda952475d804fc0d23f17609f",
  "unresolved_coverage": [],
  "validation_contract": "Enforce all required_validation_checks independently of selected_gates; zero selected_gates is valid only when coverage_status is resolved. If unresolved, final check MUST fail closed; never treat as no checks or automatically run the complete release catalog.",
  "required_consumer_dependencies": []
}
```

### pr-plan.json fields (extracted)

- `coverage_status`: **resolved**
- `head_sha`: **`af57a26d610c79ff3bb55a5727b55db497fa7c1a`** (matches witness run head)
- `base_sha`: field absent in fixture — binding base is `merge_base_sha`:
  **`eb617cf2fefea0b29c52a7654070c959b476ee96`** (the develop HEAD at producer-build
  time; `target_base_sha`/origin/develop is `9a630665`, which advanced after build).
- `integration_sha`: **`381247041315127b48425b73de58f991aa143b26`** (tested integration
  SHA read from the plan fixture).
- `plan_commit`: **`af57a26d610c79ff3bb55a5727b55db497fa7c1a`** (commit that carries the
  plan fixture for this run; the plan-fixture commit consumed by `write()`/`load_plan`
  is this HEAD).
- `schema`: `baseharbor.pr-gate-plan/v1` (correct schema).
- `selected_gates`: `[]` — valid because `coverage_status=resolved` (per validation contract).
- `required_validation_checks`: 4 checks (documentation-audit, mkdocs-de-strict,
  mkdocs-en-strict, pages-candidate-build); all 4 SUCCESS on run #40426322604.

## 4. PR / run provenance summary

- PR number: **1093** (`probe(r27): shadow producer run`, base `develop`, head branch `y1/shadow-probe`).
- PR URL: https://github.com/mcpdev80/baseharbor/pull/1093
- Final head SHA: **`af57a26d610c79ff3bb55a5727b55db497fa7c1a`**
- Final CI run: **40426322604** (`CI · Source & Quality`, event `pull_request`, conclusion success)
- Artifact: `trusted-pr-plan` → `/home/marcel/tmp-ops/y1-art/pr-plan.json`
- `PR · Source & Quality` reached SUCCESS on the final head (Watcher-tauglich).

## repo-rule compliance

- Producer base only on dedicated branch `y1/shadow-probe`; step 0 genuine.
- Steps 1+2 not run locally (mission item 3); artifact evidence only.
- Evidence file written under `evidence/board-2026-10-26/` only; no Planning write,
  no product edit outside the shadow worktree, no receipt written, no workflow
  activated, no real final-check run.
- `gh` used read/download for artifact provenance; PR push via SSH.
- PR1090 state changed from OPEN (x3 plan time) to CLOSED — documented, not a block
  for the y1 shadow producer run.

END Y1-DONE
