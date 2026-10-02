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
