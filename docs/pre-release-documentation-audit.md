# Pre-release documentation audit

Run this check before every release. Keep the result short.

## Human docs

- [ ] User-visible behavior is documented where needed.
- [ ] Human docs are concise and understandable.
- [ ] Examples match shipped behavior.
- [ ] Future behavior is clearly marked.

## Reference

- [ ] CLI reference/help matches implementation.
- [ ] Manifest/configuration reference matches schema/types.
- [ ] API/MCP reference matches the implemented surface.
- [ ] Typed errors/outcomes match implementation.

## Specs

- [ ] Changed public contracts have matching spec changes.
- [ ] Compatibility/version effects are documented.
- [ ] Security invariants remain explicit.

## Architecture

- [ ] Architecture describes current architecture, not release history.
- [ ] Provider/runtime-specific details did not leak into portable concepts.
- [ ] New decisions have an ADR when rationale must be preserved.

## Staleness and duplication

- [ ] Roadmap detail is not duplicated from GitHub Issues.
- [ ] Release history is not embedded in current-state guides.
- [ ] No obsolete page contradicts current behavior.
- [ ] Navigation and links remain valid.

## Result

```text
Documentation audit

Human docs        PASS
Reference         PASS
Specs             PASS
Architecture      PASS
Links/staleness   PASS

Notes:
- only meaningful exceptions
```

Materially incorrect or missing documentation blocks a release. Trivial wording issues do not.
