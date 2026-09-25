# Security Policy

BaseHarbor handles secrets, credentials, TLS material, runtime access and deployment state. Security reports are treated as private until a fix and disclosure plan are ready.

## Supported versions

BaseHarbor is currently pre-v1.

Security fixes are applied to the latest released version. Older releases may not receive backports unless a specific issue requires it.

## Reporting a vulnerability

Please do **not** open a public GitHub issue for a suspected security vulnerability.

Use GitHub's private vulnerability reporting / Security Advisory flow for this repository:

1. Open the repository's **Security** tab.
2. Choose **Report a vulnerability**.
3. Include enough detail to reproduce and assess the issue.

Useful information includes:

- affected BaseHarbor version or commit;
- affected runtime/provider (for example Docker or Podman);
- reproduction steps;
- expected and observed behavior;
- security impact;
- relevant logs or configuration with all secrets removed.

Do not include passwords, tokens, private keys, recovery material or other live credentials.

## What to expect

Security reports will be triaged privately. Confirmed issues will be handled through a coordinated fix and disclosure process appropriate to their impact.

## Scope

Security-sensitive areas include, among others:

- secret handling and OpenBao integration;
- TLS, PKI and trust boundaries;
- runtime and provider credentials;
- authorization and ownership checks;
- application-to-application connectivity;
- deployment state and backup material;
- CLI, JSON and MCP boundaries;
- container/runtime isolation and privilege handling.

For normal bugs, feature requests and documentation issues, use the public GitHub issue tracker.
