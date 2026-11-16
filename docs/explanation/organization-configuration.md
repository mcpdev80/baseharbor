# Organization / Platform Configuration

Organization configuration lets platform teams distribute company defaults without rewriting each application's portable contract.

## What it can supply

Organization configuration can reference:

- Targets;
- Development Stack Profiles;
- provider defaults;
- External/BYO provider registrations;
- trust and certificate material;
- policy references.

## Distribution

v0.4.19 supports organization configuration distributed through local/system sources, Git and OCI artifacts.

Git/OCI sources are resolved to immutable revision/digest identity before becoming effective configuration.

## Precedence

Explicit operator/application choices remain distinct from organization defaults. Mandatory policy is a separate layer and must not be confused with a default.

The effective model exposes provenance so users and automation can see where a value came from.

## Portability

Application Intent remains portable when organization configuration is absent, changed or supplied by another platform team.
