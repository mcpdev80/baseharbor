# BaseHarbor Review Instructions

`docs/DEVELOPMENT_GUIDELINES.md` is the canonical and mandatory engineering standard for this repository.

When reviewing or generating changes:

1. Read and apply `docs/DEVELOPMENT_GUIDELINES.md` first.
2. Review the actual diff, not only the PR description.
3. Preserve documented architecture and task scope; do not invent a new architecture silently.
4. Treat security, application/tenant isolation, secret leakage, migration safety, restore safety, and false success reporting as high-priority concerns.
5. Require negative tests when a security or isolation guarantee is claimed.
6. Prefer small, explicit, maintainable changes over speculative abstractions or new dependencies.
7. Keep `baha` stable, discoverable, fail-closed, and truthful about operational success.
8. Do not claim CI, runtime, backup, restore, TLS, or service readiness without observed evidence.
9. If code and canonical documentation disagree, report and resolve the inconsistency rather than choosing a new design implicitly.
10. Do not approve merge or roadmap progression on behalf of the human maintainer.

Use review severities from the canonical guidelines: `BLOCKER`, `MAJOR`, `MINOR`, and `INFO`.
