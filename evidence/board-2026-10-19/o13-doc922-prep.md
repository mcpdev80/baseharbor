# O13 — core-922 docs prep (read-only), 2026-10-19

Prepared for the future implementation worker of `core-922`. Read-only
preparation only: no product edits, no doc edits, no claims. This is an
analysis artifact, not a handoff; core-922 is still `waiting` and unclaimed.

Source state inspected:
- `baseharbor-plan/tasks/core-922-docs-new-edit-provider-selection.md` (full note read).
- `baseharbor-plan/releases/v0.4.26.md` (authoritative product-intent for the
  needs-first wizard, optional provider preference, consequence/consent).
- `baseharbor` at `origin/develop` vs worktree `worktrees/core-921-central-cli-mcp-registration`
  (HEAD `a66e5de6`, PR #998), which is the interface core-922 must document.

Evidence board: `evidence/board-2026-10-19`.

---

## 0. What core-922 is (from the task note)

- **Goal:** a short, accurate New/Edit guide with no prior infrastructure
  product knowledge. EN/DE.
- **In scope:** EN/DE public guides, navigation, examples, changelog and
  v0.4.26 release notes.
- **Out of scope:** generated CLI/MCP references (core-921); process docs (#890).
- **Acceptance (verbatim in note):**
  1. Explains needs-first selection, optional provider, preview, No/Cancel,
     zero capabilities, footprint and effects with real supported examples.
  2. Only essential delivered developer changes; Console/Connector consumption
     described by its actual reach; no anticipation of v0.4.28/platform features.
  3. EN/DE identical; Pages candidate build/navigation checked; no process
     rules added to the README and no public Pages deployment before promotion.
- **Boundary:** No generated CLI/MCP references, product code, shared CI or
  process-rule edits. Inspect accepted handlers/help and planning directly.
- **Gate:** use only accepted dependencies and APIs already on develop;
  confirm the trusted plan/executor runs the exact test families
  (docs audit/link/EN-DE/candidate build) on this task's file set.

---

## 1. Interface reality that must be documented (#998, HEAD a66e5de6)

Authoritative source is the accepted core-921 branch `work/core-921-central-cli-mcp-registration`.

### 1.1 Two new CLI commands, one new MCP tool each (the delivered surface)

| CLI | Summary (from `cli.Command`) | MCP tool | Description (from `internal/machine/contract.go`) |
| --- | --- | --- | --- |
| `baha app edit [NAME] [-e ENV]` | "Edit current application needs with preview and impact consent" | `baseharbor.app.edit` | "Preview/apply reviewed application needs with exact explicit consent." |
| `baha app footprint show [NAME] [-o json] [--details]` | "Display the authoritative Target footprint without mutation" | `baseharbor.app.footprint.show` | "Read the Target-bound authoritative footprint." |

- Registration: `cmd/baha/app_registration_921.go` (new file).
- `commands.go` appends `appEditRegisteredCommand` and `appFootprintRegisteredCommand`
  to the `app` command group (cmd/baha/commands.go:241-242).
- MCP wiring: `registerMCPAppEditTool` in `mcp_tools.go` (cmd/baha/mcp_tools.go:469)
  and `registerMCPFootprintShowTool` (cmd/baha/mcp_tools.go:139).
- Machine registry: `internal/machine/contract.go:228-229` add `app.edit`
  (`SafetyMutating, PolicyRequired, ConfirmationRequired`) and
  `app.footprint.show` (`SafetyReadOnly`). Golden contract updated at
  `contracts/machine/v1/control.golden.json:660-677`.
- Coverage inventory: `machine_coverage.go` — `baha app edit` → `app.edit`
  (JSONSurface: "Interactive CLI / typed two-phase MCP consent"),
  `baha app footprint show` → `app.footprint.show`.

### 1.2 Two-stage consent is the defining behavior

- `app edit` MCP input schema (`appMCPEditInput`, `app_registration_921.go:88`):
  `phase` (exactly `"propose"` or `"apply"`), `desired` (required), optional
  `proposal` + `consent` (apply only).
- `phase=propose`: carries only the Target-bound `desired` intent; returns a
  typed proposal; **no consent, no write**.
- `phase=apply`: requires the exact typed proposal **and** fresh
  `provider_change_consent` (`ProviderChangeConsent{PlanID, First, Second,
  DeleteData}`). The supported change requires **two staged acknowledgements**
  (`impact.ConfirmationsRequired >= 2`).
- Two-stage consent proven by native test `TestAppEditMCPApplyRunsRealProviderEditLifecycle`
  (cmd/baha/app_mcp_provider_edit_native_test.go): valid propose→apply converges
  a supported SQL add (State:running Verified:true); stale proposal → structured
  `conflict`; missing second stage → `approval-required`. Both leave state
  byte-identical (zero-write).

### 1.3 Snake_case public schema (#998's central correction)

- All public JSON fields are snake_case so the MCP SDK infers consistent names
  (`app_machine_adapters.go`): `instance_id`, `requirements`, `choices`,
  `application_id`, `desired`, `plan_revision`, etc.
- Nested `appProviderChoice`, `appProviderCandidate` also snake_case
  (`preferred_instance_id`, `bound_instance_id`, `candidates`, `reason`).
- Test `TestCore921MCPEditSchemaIsSnakeCaseAndPhaseConditional` asserts no
  PascalCase (`Desired`, `Proposal`, `Consent`, `Target`) leaks and that
  `desired.target` is the only required nested field; `proposal`/`consent`
  must NOT be schema-root-required.

### 1.4 Typed errors from `app_machine_adapters.go`

- `classifyAppMachineError` maps internal `appMachineError` codes
  (`conflict`, `cancel`, `denied`/`unsupported`, `partial`, default) onto the
  shared typed `machine.Error` owner with a concrete Code, CauseCode and
  remediation string (`app_machine_adapters.go:22-58`).
- `main.go` routes every `appMachineError` through it
  (`main.go:315-326`); no `internal_error` fallthrough for these origins.
- Handler-level negatives (`TestCore921MCPEditHandlerLevelNegatives`):
  wrong target → `conflict`; invalid phase → `validation_failed`;
  apply without proposal/consent → `validation_failed`; stale snapshot →
  `conflict`. Public errors surfaced via structured payload `{error:{code,cause,next}}`.

### 1.5 What the docs already describe correctly (do NOT rewrite)

- v0.4.25 release note (`docs/releases/v0.4.25.md`) already explains
  needs-first, two-confirmations, `--yes` insufficient, data-deletion consent,
  footprint labels — the v0.4.25 foundation. core-922 must **query** this
  existing record, not copy open-status prose.
- `docs/how-to/machine-operations.md` already documents two-stage
  `provider_change_consent` (`plan_id`/`first`/`second`) for `baseharbor.app.stop`
  and the non-interactive env var; the edit journey must reuse the same
  consent object wording.
- `docs/how-to/external-providers.md:66,72` already describe changing a
  selected provider entering the shared consequence plan with two
  acknowledgements.

---

## 2. Doc surface inventory and what is stale today (file:line)

Scope discipline: core-922 owns **narrative** EN/DE guides, navigation,
examples, changelog and release notes — NOT the generated CLI/MCP references
(core-921 already updated those in #998). Generated tables that #998 already
corrected (and that must stay correct, not be re-edited by core-922):

- `docs/reference/mcp.md` / `docs/de/reference/mcp.md` — generated registry
  table (lines 98-99 now list `baseharbor.app.edit` mutating/policy/confirm and
  `baseharbor.app.footprint.show` read_only). **Already correct in #998; do not edit.**
- `docs/reference/cli-machine-coverage.md` / `docs/de/reference/cli-machine-coverage.md`
  — generated coverage rows for `baha app edit` / `baha app footprint show`.
  **Already correct in #998; do not edit.**
- `docs/cli/command-index.md` / `docs/de/cli/command-index.md` — generated command
  index rows. **Already correct in #998; do not edit.**

### 2.1 Narrative guides to touch — STALE (need core-922 edits)

**EN**

- `docs/tutorials/getting-started.md`
  - :1-3 hero: "without requiring provider knowledge" — correct hook, but no
    mention that needs-first selection now has an **optional in-place provider
    preference** and a **preview**, nor that an existing app is **edited** with
    `baha app edit`.
  - :40-49 happy path ends at `baha up`/`status`/`doctor`; **no edit journey**,
    no preview, no "No/Cancel", no zero-capability note beyond :7-10.
  - :151-160 "Advanced and automation commands" lists `baha plan`,
    `baha app preflight`, `baha app apply` — **`baha app edit` and
    `baha app footprint show` are missing** from this discoverable list.
- `docs/cli/applications.md`
  - :44-52 Lifecycle table lists `baha app update` and `baha app apply` but **no
    `baha app edit` row**; `baha app footprint show` absent from all tables
    (whole file: no edit/footprint anywhere).
  - :149-155 provider-change/consequence paragraph is accurate but written
    against v0.4.25 `--yes` two-acknowledgement prose; should reference the new
    `app edit` entry and `app footprint show` explicitly.
- `docs/how-to/machine-operations.md`
  - :16-44 "Create and inspect an application" covers creation only; **no edit
    section** (propose → preview → apply with two staged confirmations).
  - :46-62 adopt/validate block; no "edit an existing app" how-to.
  - No `baha app footprint show` example.
- `docs/cli/human-workflows.md`
  - :5-20 Daily workflow table has no `baha app edit` / `baha app footprint show`
    row (line :12-16 area).
  - Header is "Human CLI — v0.4.24"; core-922 release is v0.4.26.
- `docs/cli/index.md`
  - :22-31 CLI-sections navigation; the Applications bullet (line :23) should
    point readers at the new edit/footprint workflow; Start-here block (:9-17)
    omits edit.
- `docs/how-to/postgres.md` :1-9 (primary New example); the SQL how-to is the
  canonical "select a provider" example — should show the optional explicit
  PostgreSQL instance selection within the needs-first flow, matching the
  task's "real supported examples" acceptance.

**DE (must mirror EN identically, parity requirement)**

- `docs/de/tutorials/getting-started.md`, `docs/de/cli/applications.md`,
  `docs/de/how-to/machine-operations.md`, `docs/de/cli/human-workflows.md`,
  `docs/de/cli/index.md`, `docs/de/how-to/postgres.md` — same gaps as EN.
  Confirm DE edits mirror EN line-for-line in intent; do not translate fresh.

### 2.2 Changelog and release notes

- `docs/releases/v0.4.26.md` — **does not exist** in the baseharbor repo yet
  (no `docs/releases/v0.4.26.md` on develop or HEAD). core-922's "v0.4.26
  release notes" item is the product release-note prose under
  `baseharbor-plan/releases/v0.4.26.md` section "Product surfaces"
  (`baha app edit`, `baha app footprint show`, EN/DE documentation). Decide
  whether a `docs/releases/v0.4.26.md` is in-scope for core-922 vs a later
  release-operation task — **open question for Marcel (Q1)**.
- `CHANGELOG.md` (baseharbor repo root) — `[Unreleased]` section (line 14) is
  empty; core-922 should add concise New/Edit/footprint/provider-selection
  entries under `### Added`/`### Changed`, mirroring the v0.4.25 pattern.

### 2.3 Surfaces explicitly OUT of scope (do not touch)

- `docs/reference/mcp.md`, `docs/reference/cli-machine-coverage.md`,
  `docs/cli/command-index.md` and DE equivalents — generated by core-921/#998.
- Any CLI/MCP schema, golden help, or product Go code.
- Process docs, README process rules, Pages deployment.

---

## 3. Acceptance-criteria → concrete edits mapping

| core-922 acceptance item | Where it must become a concrete edit | Concrete edit |
| --- | --- | --- |
| explains **needs-first selection** | getting-started :5-10, applications :42-53 | state that a need is requested first (plain language: secrets, TLS, users, relational data, cache), not a product; keep `provider: postgresql` out of portable intent (v0.4.26 binding note). |
| explains **optional provider** selection | postgres.md :1-9, getting-started, applications :149-155 | show one supported explicit choice (e.g. PostgreSQL instance) expanded **in place**, default `Automatic (recommended)`; the preference goes to the binding owner, not the contract; no advanced page / no mandatory questionnaire. |
| explains **preview** | machine-operations (new edit section), getting-started :40-63 | describe the preview that shows selected provider, reused vs new managed deps, incremental RAM measured/estimated/unknown; `baha app footprint show` is the canonical read. |
| explains **No/Cancel** | getting-started, machine-operations edit section, applications :149-155 | explicit: No/Cancel, EOF, non-TTY and `--no-input` never write silently; interactive `baha app edit` requires a terminal (see `appEditRegisteredCommand` usage guard). |
| explains **zero capabilities** | getting-started :7-10, applications | restate that zero selected needs is valid; a supported local workload runs without Core/login; intent alone is not READY. |
| explains **footprint** | **new short how-to or applications section**, getting-started | `baha app footprint show [NAME] [-o json] [--details]` (read-only, `app.footprint.show`); distinguish measured/estimated/unknown (v0.4.25). |
| explains **effects** with real supported examples | machine-operations (edit section), postgres.md | end-to-end: `baha app edit` → preview → two staged confirmations → `baseharbor.apply` with `provider_change_consent{plan_id, first, second}`; typed negatives (conflict/validation_failed/approval-required) as failure modes; data-deletion consent separate. |
| **essential delivered changes only**; **no v0.4.28/platform anticipation** | all | Console/Connector consumption described by actual reach only; do not anticipate v0.4.28 Console management journeys (v0.4.26 release note: "full Console management journeys stay in v0.4.28"). |
| **Console/Connector consumption described by its actual reach** | getting-started "Next steps", applications | point to Console/Connector where they consume the same domain operations; do not describe unimplemented journeys. |
| **EN/DE identical** | every touched EN file has a DE twin | mirror each EN edit in the DE twin; run EN/DE MkDocs parity check. |
| **Pages candidate build/navigation checked**; no process rules in README; no Pages deploy before promotion | docs/cli/index.md nav, candidate build | verify MkDocs build on the edited file set; do not add process rules to README; do not deploy Pages. |
| no generated CLI/MCP reference edits | — | do not touch core-921 generated tables. |

---

## 4. Verification the future worker must run (per task gate)

- `bash scripts/documentation-audit.sh` (baseharbor repo) — must pass.
- EN/DE MkDocs build (candidate) — clean, navigation valid.
- Link/anchor checks across touched files.
- EN/DE parity diff (intent-equivalent, not byte-identical).
- No runtime matrix / no native runtime for text alone.

---

## 5. Open questions for Marcel

- **Q1 (scope boundary):** `docs/releases/v0.4.26.md` does not exist in the
  baseharbor repo. Is a baseharbor-side release-note file part of core-922, or
  is "release notes" fulfilled by the `baseharbor-plan/releases/v0.4.26.md`
  product-surface prose only? The task note lists "v0.4.26 release notes" under
  in scope.
- **Q2 (new guide vs in-place):** is a dedicated short New/Edit/provider how-to
  page acceptable, or must the guidance stay inside `applications.md` /
  `machine-operations.md` per the "one page, one purpose" style rule?
- **Q3 (footprint placement):** `baha app footprint show` is new — new how-to
  page, folded into an existing how-to, or a subsection of `applications.md`?
- **Q4 (Console/Connector reach):** the exact current Console/Connector
  consumption of `app edit`/`app.footprint.show`/two-stage consent — is it
  implemented already (document as-is) or still v0.4.28 (must be omitted)?
- **Q5 (postgres how-to):** should the optional explicit PostgreSQL provider
  preference be shown in `postgres.md` specifically (matching the "real
  supported examples" acceptance), or kept generic to avoiding product-name
  coupling?
- **Q6 (typed-error surface):** do we document the concrete typed codes
  (`conflict`/`validation_failed`/`approval-required`/`runtime_unavailable`/
  `policy_denied`/`verification_failed`/`internal`) in the guide, or only the
  human-facing consequence/consent flow? (errors.md currently has no
  edit-specific content.)

---

## 6. Notes for the coordinator

- core-922 stays `waiting`; it depends on core-921 (currently `review`, #998 at
  `a66e5de6`). Do not claim/assign until #998 is ACCEPTED.
- No claims, no doc/product edits, no workspace mutation were made by this prep.
- The generated-reference tables (mcp.md, cli-machine-coverage.md,
  command-index.md, EN+DE) are already correct from #998; core-922 should only
  touch narrative guides, changelog and release notes.
- The v0.4.26 binding note in `baseharbor-plan/releases/v0.4.26.md` is the
  authoritative product-intent to document against — it already enumerates
  needs-first, optional in-place provider preference, consequence/consent and
  preview; core-922 is the prose translation of that section.

O13-DOC922-DONE
