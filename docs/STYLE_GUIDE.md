# Documentation style

BaseHarbor documentation follows one rule above all:

> So little as possible, as much as necessary.

## One page, one purpose

Every page is primarily one of:

- Tutorial — learn by doing.
- How-to — solve one task.
- Explanation — understand a concept.
- Reference — look up exact behavior.
- Spec — normative contract.
- ADR — decision and rationale.
- Release — delivered history.

Do not mix these casually.

## Human docs

Use short sections, active voice and concrete examples.

Start with what the reader can do or needs to know. Do not start with implementation history.

Avoid:

- vague architecture language;
- repeated principles;
- roadmap detail;
- release-by-release history;
- provider internals that do not help the task.

## Reference

Prefer tables, schemas and concise definitions over prose.

Do not manually duplicate syntax that can be generated from code or schema.

## Specs

Use MUST/SHOULD/MAY sparingly and only for real compatibility, interoperability or security requirements.

Prefer machine-readable contracts where practical.

## Source of truth

- JSON Schema 2020-12 -> portable service/provider configuration structure and validation;
- schema/types -> internal implementation structure;
- OpenAPI -> REST;
- Protobuf -> provider process protocol;
- code/generated help -> CLI syntax;
- specs -> normative semantics;
- ADRs -> rationale;
- GitHub Issues -> future planning;
- releases -> history.

A detailed fact should have one authoritative home.
