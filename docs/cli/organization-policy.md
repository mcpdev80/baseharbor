# Organization configuration and policy commands

Organization/platform configuration provides company defaults without modifying each application's portable intent.

## Configuration

`baha config ...` manages/inspects configuration surfaces exposed by the current CLI.

Effective organization configuration can provide references for:

- Targets;
- Stack Profiles;
- provider defaults;
- External/BYO providers;
- trust/certificate material;
- policy references.

Git and OCI organization sources resolve to immutable revision/digest identity before use.

## Policy

```text
baha policy check
baha policy explain
```

Policy remains separate from organization defaults: a default may be overridden where allowed, while mandatory policy is authoritative.

See [Organization / Platform Configuration](../explanation/organization-configuration.md).

## Example: inspect a company configuration and test policy

Your platform team has supplied an existing, valid organization configuration directory at `$HOME/company/baseharbor-config`. Activate it for development, then inspect the resolved result:

```bash
baha config organization set --source local \
  --location "$HOME/company/baseharbor-config" --environment dev
baha config organization show --environment dev -o json
baha config organization check
```

The directory must contain a supported organization artifact, not an application `baseharbor.yaml`. Inside your application repository, evaluate the environment you plan to deploy:

```bash
baha policy check -e test
baha policy explain -e test
```

`check` reports whether the operation's resolved context satisfies policy; `explain` helps locate the governing rule. Fix a missing provider or denied policy requirement before applying. Activating defaults does not override mandatory policy.
