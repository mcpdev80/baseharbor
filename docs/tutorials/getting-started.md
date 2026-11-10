# Getting started

Use BaseHarbor to run an existing application or create a new one. You need the `baha` CLI and a configured Docker or Podman Target. See [Target setup](../cli/targets.md) if none exists.

## Adopt an existing application

Open a supported application repository and run:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

`inspect` shows detected workloads without changing files. `init` creates the portable application contract and asks only for decisions it cannot make safely. `plan` previews the deployment; `up` applies it. `status` shows the result and application URL.

To diagnose a problem, run `baha doctor`. To stop the application's workload without deleting persistent data, run `baha down`. Run `baha up` to start it again.

For unambiguous repositories, automation can use `baha init --quick`. Ambiguous sources require an explicit choice.

## Create a new application

From a directory that does not contain `orders-api`:

```bash
baha new application orders-api --stack go --http --sql
cd orders-api
baha plan
baha up -e dev
baha status
```

This creates a Go HTTP example with PostgreSQL requirements. Open the HTTPS URL shown by `baha status` and request `/healthz`. Docker and rootless Podman may use different ports.

## What happens on the first run?

A local application without managed capabilities can run without Core. If your application requests SQL, secrets or other managed infrastructure, BaseHarbor uses a compatible installation or requests your consent to set one up. It never silently installs Core during non-interactive execution.

You may be asked for required application secrets, a recovery-file location or a port choice. Secrets are entered without echoing them in the terminal. Creating project files alone does not mean the application is running.

For a separate workstation and Core host, see [Connect a developer workstation](../how-to/remote-workstation.md). First-operator setup and remote node enrollment are separate from this local quick start.

## Next steps

[Application needs](../how-to/application-needs.md) · [Targets](../cli/targets.md) · [Environments](../how-to/environments.md) · [PostgreSQL](../how-to/postgres.md) · [Backup and restore](../how-to/backup-restore.md) · [CLI reference](../cli/command-index.md)
