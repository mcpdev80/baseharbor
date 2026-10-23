# feat(core-921): register app edit and footprint across CLI/MCP

## Description

Registers already-accepted handler constructors through the real CLI/JSON/MCP entry points: `app edit` interactive CLI and two-phase typed MCP Edit (proposal/consent lifecycle), `app footprint show` CLI and read-only MCP. Shared registration/CLI files change in this one integration PR; predecessor tasks deliver compiling constructors/adapters.

### Two-stage `app edit` wiring (round-3)

Codex 6.1 round 3 **CHANGES_REQUESTED** on candidate `{{FINAL_SHA_998}}`: the native lifecycle test drives `baseharbor.plan`/`baseharbor.apply`, never `baseharbor.app.edit`. The smallest sufficient gate is one isolated Docker test:

```
app.edit propose → JSON round-tripped proposal → app.edit apply
  with matching consent and verified execution.
  stale proposal + wrong consent → structured errors with unchanged persisted state.
```

A fresh correction budget opens for this independent cause. The PR body will be updated to `{{FINAL_SHA_998}}` once the corrected test lands.

### Evidence trail (native MCP gate PROVEN)

- **o10-native-mcp-proof.md** — real native lifecycle on baha-docker (202 s, exit 0): valid `propose->apply` converges a supported SQL add with verified complete receipt; stale proposal → structured `conflict` with unchanged binding; missing second stage → `approval-required` with unchanged binding.
- **q1-native-reverify.md** — qwen3.6 independent re-run (fresh isolated dir, exit 0, ~235 s, structured negatives confirmed).
- **r3-998-native-delta-review.md** — APPROVE: implements exactly the four Codex round-2 requirements against the real `baseharbor.apply` MCP entry.

### Full-source review history

- **r1-998-prereview.md** — APPROVE (read-only pre-review at 6931ba9c): snake_case schemas, typed `machine.NewError(machine.ErrorConflict,...)` paths, structured handler negatives with zero-write proof.
- **r4-998-source-review.md** — APPROVE (full-source at 08d8f498): all schema/typing/consent defects corrected.
- **o4-998-rereview.md** — APPROVE (independent rereview at 08d8f498): byte-unchanged guards, phase-conditional proposal/consent.

## What changed

- `cmd/baha` command tree registration (human_new routing, machine_coverage, CLI entry points)
- MCP tool registry (app edit, app footprint show)
- Help/discovery goldens and generated CLI/MCP references
- Negative-consent tests (in-process MCP client, CLI smokes)
- Machine Discovery fixture

## Review trail

Independent review requested: see HANDOFF-CODEX-ROUND3.md Part 2. Codex round 3 verdict at codex-verdict-core921-round3.md.

## Dependencies

- Depends on core-917, core-918, core-919, core-920 (all accepted/closed).
- See t-20261019-core921-coverage-binding (PR #1007) for CI coverage binding.

## History

see commits on `work/core-921-central-cli-mcp-registration` (parent lineage from 6931ba9c through a66e5de6).

DRAFT-PR998-BODY
