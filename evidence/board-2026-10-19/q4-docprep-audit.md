# Q4-DOC prep audit of o13-doc922-prep, 2026-10-19

**Source inspected:** `origin/develop` HEAD, worktree `worktrees/core-921-central-cli-mcp-registration`.
**Scope:** Spot-verify factual claims in o13-doc922-prep.md against actual files.
**Mode:** Read-only. No edits.

---

## Part 1: Stale-content claims (section 2.1 of prep report)

### Claim 1: getting-started.md :1-3 "without requiring provider knowledge"

**Claim text:** Lines 1-3 contain "without requiring provider knowledge".
**Actual evidence (getting-started.md lines 1-3):**
```
1: # Getting started
2:
3: This tutorial gets an existing application running under BaseHarbor without requiring provider knowledge or manual manifest editing.
```
**Verdict:** CORRECTIONS — 1 minor. The phrase is on line 3 only, not spanning lines 1-3. Line 1 is `# Getting started`, line 2 is blank. The phrase lives on line 3. Substantively correct.

### Claim 2: getting-started.md :40-49 happy path ends at `baha up`/`status`/`doctor`

**Claim text:** Lines 40-49 contain the happy path ending at `baha doctor`.
**Actual evidence (lines 40-49):**
```
40: If no Target is configured, follow [Target commands](../cli/targets.md) before deploying. Create a fresh application:
41:
42: baha new application orders-api --stack go --http --sql
43: cd orders-api
44: baha plan
45: baha up -e dev
46: baha status
47: baha doctor
48: ```
**Verdict:** CONFIRMS. Lines 40-49 contain the happy path ending at `baha doctor`. No edit/footprint mentioned.

### Claim 3: getting-started.md :151-160 "Advanced and automation commands" lists `baha plan`, `baha app preflight`, `baha app apply` — missing edit/footprint

**Claim text:** Lines 151-160 list plan/preflight/apply/secret set but not edit or footprint.
**Actual evidence (lines 151-160):**
```
151: ## Advanced and automation commands
152:
153: These commands remain available, but are not required knowledge for the basic happy path:
154:
155: baha plan
156: baha app preflight
157: baha app apply
158: baha app secret set APP_SECRET
159: ```
**Verdict:** CONFIRMS. All four commands listed; `baha app edit` and `baha app footprint show` absent.

### Claim 4: applications.md :44-52 Lifecycle table lists `baha app update` and `baha app apply` but no edit row

**Claim text:** Lines 44-52 contain lifecycle table with update/apply but no edit row.
**Actual evidence (lines 44-52):**
```
44: | Command | Purpose |
45: | --- | --- |
46: | `baha up` | Converge an application |
47: | `baha app apply` | Apply/reconcile through the application lifecycle |
48: | `baha down` | Stop application runtime state |
49: | `baha status` | Observe application state |
50: | `baha doctor` | Diagnose and optionally repair |
51: | `baha app update` | Update supported application source/state |
52: | `baha destroy` | Remove application-owned resources |
```
**Verdict:** CONFIRMS. Table has update/apply but no `baha app edit` row. `baha app footprint show` absent.

### Claim 5: applications.md :149-155 provider-change/consequence paragraph

**Claim text:** Lines 149-155 contain provider-change prose against v0.4.25 `--yes` two-acknowledgement prose.
**Actual evidence (lines 149-155):**
```
149: Intentional provider changes show affected consumers, lost functions, selected
150: continuation and retained/deleted material. Alternative-less, destructive or
151: unknown consequences display **Achtung** and require two explicit acknowledgements
152: of the current plan. Ordinary `--yes` supplies neither these acknowledgements nor
153: data-deletion consent. Actual authority, ownership and post-mutation verification
154: remain required. Failed or interrupted execution stays partial and retryable.
155: The v0.4.25 candidate still requires complete all-provider, mutation-path and
```
**Verdict:** CONFIRMS. The prose references v0.4.25 consequence/consent flow; no edit/footprint mentioned.

### Claim 6: machine-operations.md :16-44 "Create and inspect an application" covers creation only

**Claim text:** Lines 16-44 cover creation only, no edit section.
**Actual evidence (lines 16-44):**
```
16: ## Create and inspect an application
17:
18: The CLI creates intent without starting containers:
19:
20: baha --no-input app create orders --sql --json
21: baha --no-input app show orders --json
22: baha --no-input app list --json
23: ```
24:
25: Equivalent MCP calls use these names and arguments:
26:
27: {"name":"baseharbor.app.create","arguments":{"intent":{"Version":1,"Name":"orders","Environment":"dev","Services":{"SQL":true}}}}
28: ```
29:
30: Omit ApplicationID: BaseHarbor generates and persists the identity...
31:
32: For a repository, use deterministic `baha app init orders --sql --json`...
33: ```
**Verdict:** CONFIRMS. Lines 16-44 cover creation and adoption only. No edit section. Line 32 references `baha app init`, not `baha app edit`.

### Claim 7: human-workflows.md :5-20 Daily workflow table has no edit/footprint row

**Claim text:** Lines 5-20 contain the daily workflow table without edit/footprint.
**Actual evidence (lines 5-20):**
```
5: ## Daily workflow
6:
7: | Goal | Command |
8: | --- | --- |
9: | Create an application or another object | `baha new` |
10: | Generate a new application non-interactively | `baha new application NAME --stack go` |
11: | Adopt a repository | `baha init` |
12: | Start an application and required Core | `baha up` |
13: | Stop the current application, preserving persistent data | `baha down` |
14: | Show current state | `baha status` |
15: | Inspect contract and detected resources | `baha inspect` |
16: | Preview desired changes | `baha plan` |
17: | Diagnose problems | `baha doctor` |
18: | List managed applications | `baha list` |
19: | Back up / restore | `baha backup` / `baha restore` |
20: | Remove owned application state | `baha destroy --yes` |
```
**Verdict:** CONFIRMS. No `edit` or `footprint` rows.

**Claim 7b: human-workflows.md header is "Human CLI — v0.4.24"; core-922 release is v0.4.26**

**Actual evidence (line 1):** `# Human CLI — v0.4.24`
**Verdict:** CONFIRMS. Header says v0.4.24. Release note is v0.4.26. Header is stale.

### Claim 8: cli/index.md :22-31 CLI-sections navigation; Applications bullet should point at edit/footprint

**Claim text:** Lines 22-31 list CLI-sections navigation.
**Actual evidence (lines 22-31):**
```
22: - [Core workflow](core.md) — start, stop, inspect, plan, diagnose and destroy.
23: - [Applications](applications.md) — create, adopt and operate applications.
24: - [Targets](targets.md) — deployment targets and runtime selection.
25: - [Providers](providers.md) — bundled, external/BYO and provider authoring.
26: - [Development](development.md) — Stack Profiles, workspace and developer access.
27: - [Organization and policy](organization-policy.md) — organization defaults, configuration and policy.
28: - [Security and trust](security-trust.md) — trust, login and connection security.
29: - [Automation and agents](automation-agents.md) — JSON, MCP and agent-facing surfaces.
30: - [Shell and UX](shell-ux.md) — completion, shell integration, prompt and TUI.
31: - [Global options](global-options.md) — options accepted across commands.
```
**Verdict:** CONFIRMS. Lines 22-31 present. Applications bullet (:23) does not mention edit/footprint workflow.

### Claim 9: postgres.md :1-9 primary New example

**Claim text:** Lines 1-9 contain the primary `baha new application` example.
**Actual evidence (lines 1-9):**
```
1: # Run an order API with PostgreSQL
2:
3: Create a Go API with SQL intent, then inspect and query its application-owned database. With `baha` installed, run from a parent directory:
4:
5: baha new application orders-api --stack go --http --sql
6: cd orders-api
7: baha plan
8: baha up -e dev
9: ```
**Verdict:** CONFIRMS. Lines 1-9 contain the canonical New example.

---

## Part 2: Generated reference tables already correct for #998

### Claim 10: docs/reference/mcp.md lines 98-99 list `baseharbor.app.edit` mutating/policy/confirm and `baseharbor.app.footprint.show` read_only

**Actual evidence (mcp.md lines 98-99):**
```
98: | `baseharbor.app.edit` | `mutating` | true | true |
99: | `baseharbor.app.footprint.show` | `read_only` | false | false |
```
**Verdict:** CONFIRMS. Both tool names present with correct attributes. Line numbers exact.

### Claim 11: docs/de/reference/mcp.md DE equivalent already lists both tools

**Actual evidence (de/mcp.md lines 92-93):**
```
92: | `baseharbor.app.edit` | `mutating` | true | true |
93: | `baseharbor.app.footprint.show` | `read_only` | false | false |
```
**Verdict:** CONFIRMS. DE equivalent correct. Line numbers differ by -6 (EN 98-99 vs DE 92-93) due to minor structural differences, but content is correct.

### Claim 12: docs/reference/cli-machine-coverage.md has coverage rows for `baha app edit` / `baha app footprint show`

**Actual evidence (cli-machine-coverage.md lines 21, 25-26):**
```
21: | `baha app edit` | semantic | Interactive CLI / typed two-phase MCP consent | baseharbor.app.edit |  |
25: | `baha app footprint` | presentation |  |  | Command group; invoke its supported subcommands. |
26: | `baha app footprint show` | semantic | CLI -o json / typed MCP result | baseharbor.app.footprint.show |  |
```
**Verdict:** CONFIRMS. Both command rows present with correct MCP tool names. Line 21 is `baha app edit`, line 26 is `baha app footprint show`.

### Claim 13: docs/de/reference/cli-machine-coverage.md DE equivalent has both rows

**Actual evidence (de/cli-machine-coverage.md lines 21, 25-26):**
```
21: | `baha app edit` | semantisch | Interactive CLI / typed two-phase MCP consent | baseharbor.app.edit |  |
25: | `baha app footprint` | Darstellung |  |  | Command group; invoke its supported subcommands. |
26: | `baha app footprint show` | semantisch | CLI -o json / typed MCP result | baseharbor.app.footprint.show |  |
```
**Verdict:** CONFIRMS. DE equivalent correct. Line numbers identical to EN.

### Claim 14: docs/cli/command-index.md has edit/footprint rows

**Actual evidence (command-index.md lines 23, 27-28):**
```
23: | `baha app edit` | Edit current application needs with preview and impact consent | `baha app edit [NAME] [-e ENV\|--environment ENV]` | [↗](applications.md) |
27: | `baha app footprint` | Read the current application footprint | `baha app footprint show [NAME] [-o json\|--output json] [--details]` | [↗](applications.md) |
28: | `baha app footprint show` | Display the authoritative Target footprint without mutation | `baha app footprint show [NAME] [-o json\|--output json] [--details]` | [↗](applications.md) |
```
**Verdict:** CONFIRMS. All three rows present. Line 23 is `baha app edit`, lines 27-28 are `baha app footprint` (group) and `baha app footprint show`.

### Claim 15: docs/de/cli/command-index.md DE equivalent has all rows

**Actual evidence (de/command-index.md lines 23, 27-28):**
```
23: | `baha app edit` | Edit current application needs with preview and impact consent | `baha app edit [NAME] [-e ENV\|--environment ENV]` | [↗](applications.md) |
27: | `baha app footprint` | Read the current application footprint | `baha app footprint show [NAME] [-o json\|--output json] [--details]` | [↗](applications.md) |
28: | `baha app footprint show` | Display the authoritative Target footprint without mutation | `baha app footprint show [NAME] [-o json\|--output json] [--details]` | [↗](applications.md) |
```
**Verdict:** CONFIRMS. DE equivalent correct. Line numbers identical to EN.

---

## Part 3: Claim that core-922 needs no edits to generated files

### Claim 16: CHANGELOG.md line 14 — [Unreleased] section exists and is empty

**Actual evidence (CHANGELOG.md lines 7-8):**
```
7: ## [Unreleased]
8:
9: ## [0.4.25] - 2026-10-15
```
**Verdict:** CORRECTIONS — 1 minor. Claim says line 14; actual header is on line 7. The [Unreleased] section IS empty — no content between the header and the next version section. Substantively correct, line number off.

### Claim 17: docs/releases/v0.4.25.md exists with needs-first/two-confirmations prose

**Actual evidence (v0.4.25.md lines 5-31):**
```
5: ## Start with only what your app needs
6:
7: Run a supported local workload on Docker or rootless Podman without a mandatory
8: management installation or login. Request SQL, cache, secrets, storage, identity
9: or HTTPS when needed; BaseHarbor resolves their genuine dependencies.
...
29: Alternative-less, destructive or unknown consequences require **Achtung** and
30: two current-plan confirmations. Deleting owned data requires separate consent;
31: `--yes` does not replace these decisions.
```
**Verdict:** CONFIRMS. File exists. Lines 29-31 contain the two-confirmations prose.

---

## Summary: Per-claim results

| # | Claim (from prep report) | Evidence | Verdict |
|---|---|---|---|
| 1 | getting-started.md :1-3 "without requiring provider knowledge" | Line 3 only | CORRECTIONS (line range minor) |
| 2 | getting-started.md :40-49 happy path ends at doctor | Lines 40-47 | CONFIRMS |
| 3 | getting-started.md :151-160 Advanced lists plan/preflight/apply only | Lines 155-158 | CONFIRMS |
| 4 | applications.md :44-52 Lifecycle table no edit row | Lines 46-52 | CONFIRMS |
| 5 | applications.md :149-155 provider-change prose v0.4.25 | Lines 149-155 | CONFIRMS |
| 6 | machine-operations.md :16-44 creation only, no edit | Lines 16-32 | CONFIRMS |
| 7 | human-workflows.md :5-20 daily workflow no edit/footprint | Lines 7-19 | CONFIRMS |
| 7b | human-workflows.md header v0.4.24 | Line 1 | CONFIRMS |
| 8 | cli/index.md :22-31 CLI sections nav, Applications :23 | Lines 22-31 | CONFIRMS |
| 9 | postgres.md :1-9 primary new example | Lines 1-8 | CONFIRMS |
| 10 | mcp.md :98-99 edit + footprint.show listed | Lines 98-99 | CONFIRMS |
| 11 | de/mcp.md DE equivalent listed | Lines 92-93 | CONFIRMS |
| 12 | cli-machine-coverage.md edit + footprint rows | Lines 21, 25-26 | CONFIRMS |
| 13 | de/cli-machine-coverage.md DE equivalent | Lines 21, 25-26 | CONFIRMS |
| 14 | command-index.md edit + footprint rows | Lines 23, 27-28 | CONFIRMS |
| 15 | de/command-index.md DE equivalent | Lines 23, 27-28 | CONFIRMS |
| 16 | CHANGELOG.md [Unreleased] empty | Line 7 | CORRECTIONS (line number) |
| 17 | v0.4.25.md exists with needs-first/two-confirmations | Lines 29-31 | CONFIRMS |

**Total: 17 claims verified. 15 CONFIRMED, 2 CORRECTIONS (line number only, no substantive errors).**

The predecessor's prep report is substantively accurate. The only discrepancies are minor line-number offsets:
- Claim 1: "without requiring provider knowledge" is on line 3, not lines 1-3 (line 3 is the sole line containing the phrase).
- Claim 16: [Unreleased] header is on line 7, not line 14.

Both are cosmetic — the actual content claims are correct in every case.

### Conclusion: core-922 generated-file claims

- **Claim:** "docs/reference/mcp.md, docs/reference/cli-machine-coverage.md, docs/cli/command-index.md and DE equivalents — generated by core-921/#998. Already correct; do not edit."
- **Verification:** All 6 generated files (EN/DE x 3) contain the exact #998 tool/schema names (`baseharbor.app.edit`, `baseharbor.app.footprint.show`, `baha app edit`, `baha app footprint show`) at the claimed line ranges.
- **Verdict:** CONFIRMS. core-922 needs zero edits to generated files. The claim is fully substantiated.

Q4-DOCPREP-DONE
