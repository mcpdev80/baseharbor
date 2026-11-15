# GitLab CI consumption example (documentation)

This directory illustrates how a **GitLab CI** pipeline consumes an already
produced BaseHarbor release. It is documentation, not a BaseHarbor test gate:
it builds nothing, contains no secrets, and does not trigger a real
environment.

## What it demonstrates

- Pins the released `baha` binary and runtime image by release version
  (`BASEHARBOR_VERSION`, `BASEHARBOR_IMAGE`); the same artifact runs across
  stages.
- Uses the shared GitLab runner only (`tags: [shared]`); no self-hosted owner
  machine.
- Drives the `baha` surface headlessly: read-only `inspect` and `plan`,
  `preflight`, `apply --yes`, and structured `status`/`env`/`creds` output.
- Fails closed with `--no-input` and checks exit codes before using output.
- Seeds one required secret from a masked CI/CD variable via `--stdin`; no
  value is ever logged or committed.
- Stops the application in a `teardown` stage that always runs.

## How to use it

1. Replace `<X.Y.Z>` with the release that the acceptance gate accepted, and
   the `BASEHARBOR_IMAGE` tag with the matching runtime image tag.
2. Add the required masked CI/CD variables (for example `BASEHARBOR_API_KEY`)
   in the project settings, not in this file.
3. Adjust `TAGS` to the runtime the acceptance gate validated
   (`docker` or `podman`).
4. Gate the `accept` job on manual approval if it points at a real
   environment.

## The consumption pattern

```text
inspect (read-only)
        |
        v
plan (read-only)  →  seed (masked var, --stdin)
        |
        v
apply --yes  →  status  →  env  →  creds
        |
        v
down (teardown, always)
```

## See also

- [CI consumption guide](../../how-to/ci-consumption.md)
