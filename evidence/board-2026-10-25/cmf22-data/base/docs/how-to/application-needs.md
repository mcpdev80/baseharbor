# Create and edit application needs

Choose what your application needs; BaseHarbor resolves compatible providers on
the selected Target. You can keep the automatic choice or select an available
instance without putting provider products into the portable application contract.

## Create an application

Check the effective Target from a parent directory with room for a new project:

```bash
baha target show
baha app new
```

Enter an application name, project location and development stack. Keep
`create-only` to generate project files without starting containers or deploying
providers. Choose `run` only when you also want to continue into the separately
authorized application lifecycle.

The checklist offers SQL, cache, protected application secrets, application login
and TLS/HTTPS. Additional needs are available through the checklist's disclosure
row. Review preselected needs and deselect anything you do not need. Selecting
nothing is valid; a usable application still needs a supported workload.
Application login means end-user OIDC, not operator access or service credentials.
TLS and secrets do not implicitly select application login.

Use the arrow keys to move and Space to toggle a need. Enter opens the preview.
For example, select SQL for an order API; select cache only if the application
uses one. You do not need to choose a database product to keep the automatic
provider selection.

## Choose an optional provider

The selection view shows compatible instances, the selected instance and its
reason for each requirement on the current Target. Keep **Automatic** unless a
specific available instance is required. Press P in the New checklist to enter
the displayed requirement and instance IDs. For example, if these exact IDs are
listed:

```text
database.sql/default postgresql/shared/team
```

In New, entering `database.sql/default automatic` resets that preference.
An instance from another Target, a missing instance or an incompatible instance
is not a valid substitute. Resolve the displayed provider or policy blocker
before continuing; confirmation does not make an unsupported provider executable.

Application needs belong to `baseharbor.yaml`. Explicit provider preferences
belong to protected Target/deployment state. Different Targets can bind the same
portable intent differently. BaseHarbor assigns the application ID; do not add
`app.id` to authored YAML.

## Review before writing

The preview explains selected providers, reused or new dependencies, containers
and memory where attribution is available. Measured, estimated and unknown values
remain distinct. Provider consequences describe affected applications, access and
retained or deleted material; a retained file is not proof of continued access.

**No** returns to the checklist with need and provider selections preserved.
You can repeat this loop. Cancel or EOF before confirmation writes nothing.
A changed Target plan requires a refreshed preview and new consent.

Confirming `create-only` writes the project and its selected Target preferences;
it does not prove runtime readiness. Enter the generated project directory and
start it when ready:

```bash
baha up
baha status
baha doctor
```

## Edit an existing application

Run inside its repository, with the intended Target selected:

```bash
baha app edit
```

Existing named needs and provider preferences are loaded first. Edit currently
uses line-oriented input: type `up`, `down` or `space`, then Enter to submit each
action. A blank line opens the preview. To select an instance actually shown in
the view, enter:

```text
provider database.sql/default postgresql/shared/team
```

To reset an explicit Edit preference, enter `provider database.sql/default automatic`
for the requirement shown in the view. The next preview uses automatic resolution.

At the Edit preview, type `yes` to continue or `no` to return to selection.
A blank line also returns; `cancel` exits without applying. Edit rechecks the
current intent, ownership and consequence plan before using the existing
apply/verify lifecycle. Removing a need, changing a provider and deleting data
are separate decisions. Required consequence acknowledgements and separate data
deletion consent are not replaced by `--yes`. Failed or interrupted execution
reports partial results rather than an unverified success.

Interactive New and Edit require a terminal. For deterministic creation, use
explicit New arguments from the [command index](../cli/command-index.md).
MCP exposes `baseharbor.app.edit` as `propose` followed by `apply` with the matching
proposal and fresh consent; use the [MCP reference](../reference/mcp.md) for the
current schema. Existing Console and Connector consumers retain their supported
operations; these CLI commands do not add a new Console wizard or remote adapter.

## Inspect the footprint

These commands read the same Target-bound footprint without changing resources:

```bash
baha app footprint show
baha app footprint show --details
baha app footprint show -o json
```

Requested or planned resources are not live observations. Use status and doctor
after execution to verify actual readiness. For limits, see
[application commands](../cli/applications.md) and
[platform support](../reference/platform-support.md).
