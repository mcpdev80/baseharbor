# Declarative input resolution

v0.4 introduces a reusable input resolver so CLI prompts are a presentation layer rather than the source of application/deployment question logic.

The resolver lives in `internal/applicationinput` and models four concerns independently:

- **default** values: safe values BaseHarbor can choose without operator interaction;
- **generated** values: values produced through an explicit generator callback;
- **external** values: values that must come from protected state, explicit automation input or a user/operator;
- **conditional** values: inputs that become required only when another resolved value selects a matching path.

Every definition can also be marked `Secret`. Secret values remain available to the caller for delivery to a secret/capability provider, but `PersistableValues` deliberately excludes them and `Value.String()` renders them as `<redacted>`.

## Resolution order

For each declared input BaseHarbor resolves, in order:

1. an explicitly supplied value;
2. the declared safe default;
3. a generated value, when generation is declared and required;
4. otherwise an unresolved requirement when the input is required or its `required-if` condition matches.

The resolver itself never reads stdin, writes files, selects a provider or logs values. That makes the same model reusable by CLI/TUI, API, future GUI, agents and CI.

## v0.4 CLI reference integration

Repository deployment initialization uses the resolver for:

- `hostname`
- `tls_mode`
- `cert_dir`, required only when `tls_mode=existing`

Interactive `baha app init` asks only for unresolved values. Existing protected deployment state and explicitly supplied flags are treated as already resolved.

Automation can inject declared non-secret deployment values explicitly:

```bash
baha app init --yes \
  --input hostname=mail.example.com \
  --input tls_mode=existing \
  --input cert_dir=/secure/certificates
```

The existing dedicated flags remain compatible:

```bash
baha app init --yes \
  --hostname mail.example.com \
  --tls existing \
  --cert-dir /secure/certificates
```

Conflicting values supplied through both mechanisms fail instead of silently choosing one.

`baha up` uses the same declarations. When the repository contract and protected deployment state already satisfy every required input, it asks no questions. If values are missing in an interactive terminal, only those unresolved values are requested. Non-interactive mode uses only defined safe defaults/derivations and never invents an external certificate path.

## TLS reference cases

- Local/non-public deployment: `tls_mode=local` is a safe default when public TLS is not required.
- Public TLS with a known non-local hostname: non-interactive safe resolution may select `acme`; issuance remains a provider capability outside the resolver.
- Existing/BYOC certificate: choosing `existing` makes `cert_dir` conditionally required; the existing certificate detection, key matching, SAN/FQDN validation and normalized protected storage continue to run after resolution.

The resolver therefore decides **which inputs are needed**, while the TLS implementation continues to validate and apply the selected mode.

## Boundary with the portable application contract

An application may eventually declare generic required inputs in the portable contract. Runtime/provider products, secret values and operator security policy do not belong there. The resolver consumes declarations and supplied context; provider selection and secret storage remain separate concerns.
