# Automate supported operations

`baha app down` and `baseharbor.app.stop` share the current consequence plan.
Review affected functions, consumers and retained owner locations before
confirming. Unknown or alternative-less consequences require **Achtung** and
two explicit acknowledgements. MCP supplies `provider_change_consent` with the
displayed `plan_id`, `first` and `second`; noninteractive CLI uses the same
bounded JSON object through `BASEHARBOR_PROVIDER_CHANGE_CONSENT`. `--yes` grants
neither stage nor data deletion. Stop retains desired intent, provider bindings
and shared providers for a later `app up`. The structured
`provider_change_result` distinguishes verified runtime/recovery ownership from
consumer effects that remain unverified and partial.

Use `baha agent describe -o json` and the MCP client's `tools/list` to inspect current operation IDs, required inputs, safety and approval. On supported local workloads, CLI/stdio can use verified local runtime access without a separate OIDC login, including `test`/`prod` when no operator policy or management consumer requires OIDC. Remote, protected HTTP, tenant-scoped and explicitly managed operation paths retain authenticated operator requirements. File references refer to the execution host where `baha mcp serve` runs.

## Create and inspect an application

The CLI creates intent without starting containers:

```bash
baha --no-input app create orders --sql --json
baha --no-input app show orders --json
baha --no-input app list --json
```

Equivalent MCP calls use these names and arguments:

```json
{
  "name": "baseharbor.app.create",
  "arguments": {
    "intent": {
      "Version": 1,
      "Name": "orders",
      "Environment": "dev",
      "Services": {"SQL": true}
    }
  }
}
```

Omit `ApplicationID`: BaseHarbor generates and persists the identity during authorized creation and returns it in the result. Client-supplied IDs are rejected before mutation. Discovery shows the exact authoring schema property names. Inspect the result with `baseharbor.app.show`, arguments `{"name":"orders"}`. Creation never replaces existing intent.

For a repository, use deterministic `baha app init orders --sql --json`, or `baseharbor.app.adopt` with `repository` and complete `intent`. Existing repository deployment inputs use `baseharbor.app.configure` with explicit `hostname`, `tls_mode` and, for existing TLS, `certificate_directory`; ambiguous or missing inputs produce errors rather than prompts.

## Adopt and validate detected HTTPS exposure

For the reference demo:

```bash
git clone https://github.com/mcpdev80/baseharbor-demo.git
cd baseharbor-demo
git checkout b2b2a91cb8a78aa180abd8ca90f61b3b42cd5d63
baha app inspect .
baha --no-input app init --quick --json
baha app preflight --json
```

Quick init includes the detected `demo-app` HTTPS exposure on container port 8080. If a labeled workload has multiple target ports, adoption stops before writing; select an explicit exposure contract. Preflight also checks runtime availability and workload security, so it requires a usable selected runtime for a full pass.

Running quick init again reports the existing manifest without changing its stable identity or accepting newly detected capabilities. Review repository evolution and edit the existing `exposure.http` entries explicitly; `baha app init` configures deployment inputs after the contract exists.

## Map a multi-repository workspace

Inside the repository with `baseharbor.yaml` and an `api` component:

```bash
baha --no-input app workspace init \
  --source backend=https://github.com/acme/api.git \
  --component api=backend -o json
baha --no-input app workspace map backend "$HOME/src/api" -o json
baha --no-input app workspace show -o json
```

MCP `baseharbor.workspace.init` accepts typed `sources` and `components`; `baseharbor.workspace.map` accepts `source` and `path`. Local checkout paths stay in local workspace state, separate from portable source identity. The checkout must exist; replace the illustrative Git URL and path with your repository.

## Set a protected secret

Apply a repository application with managed secrets first. Create the protected input file with an operator-owned secret manager or secure editor, then restrict its permissions:

```bash
chmod 600 /secure/orders-api-token
baha --no-input app secret set API_TOKEN --file /secure/orders-api-token --json
baha --no-input app secret list --json
```

MCP `baseharbor.secret.set` uses `{"key":"API_TOKEN","file":"/secure/orders-api-token"}` in the current repository. It reads the protected regular file after authorization and secret-scope validation. Results contain metadata only. Delete through `baseharbor.secret.delete` with `{"key":"API_TOKEN","approval":true}` after reviewing the intended deletion. TLS material uses `secret.tls-set` with `certificate_file` and protected `private_key_file`.

`app.environment` always masks credentials, including unknown provider fields. `app.connection` returns host/port/database/username metadata without passwords or credential-bearing URIs. Interactive database clients remain operator-local.

## Review and install public host trust

Once the managed-local issuer is ready:

```bash
baha --no-input trust status --json
baha --no-input trust export --output /tmp/baseharbor-public-ca.pem --json
baha --no-input trust install --yes --json
```

MCP `trust.status` inspects public CA trust, `trust.export` writes a new public CA file, and `trust.install` requires `approval=true`. Installation affects the MCP execution host. External/operator-owned PKI is never claimed or installed as BaseHarbor-owned trust. No private issuer keys leave the provider.

## Operate the control plane

`baseharbor.control-plane.up` accepts explicit `postgres_port`, `openbao_port` and protected `recovery_file`. It retains host memory and port preflight. Inspect with `control-plane.status` and `control-plane.doctor`; reconverge existing configuration through `control-plane.repair`; stop owned resources through `control-plane.stop` without deleting persistent data.

`openbao.bootstrap`, `openbao.unseal` and `openbao.rotate` use a protected recovery-file reference, never a key/token in tool arguments. `openbao.status` reports initialization, seal and manager readiness only.

Destruction is separate: `control-plane.destroy` requires explicit approval and refuses active application-owned bindings. `installation.destroy` authorizes every discovered registered deployment before any cleanup and requires explicit approval; it preserves external application source/data. Review the full intended scope before invoking it.

## Test an external provider

Pin the public module version or immutable tested source commit in your own Go module. Implement the isolated hooks documented by the [public conformance profile](../spec/provider-conformance-v1.md), then run:

```go
report := provider.RunFull(ctx, target)
if err := provider.WriteJSON(os.Stdout, report); err != nil {
    os.Exit(2)
}
os.Exit(provider.ExitCode(report))
```

The full suite checks lifecycle, drift, outage, malformed binding, verification failure, recovery and ownership-safe destroy. Missing fixture hooks fail the suite. A conformance pass does not mean the publisher is trusted: [artifact verification and trust policy](../spec/extension-artifact-trust-v1.md) remain independent results.
