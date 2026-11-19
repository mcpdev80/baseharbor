# Consume BaseHarbor from CI

Use a manually started pipeline to exercise a published BaseHarbor CLI against a
disposable application project. The example starts one local Core on the
ephemeral runner and keeps Core and application commands in the same job. It
does not rebuild BaseHarbor or the application, and it does not deploy to a
customer Target.

Copy the workflow and `scripts/ci-consumption-example.sh` into an application
repository that contains a valid `baseharbor.yaml` and its
workload source. Use only a disposable project and secrets for this example;
application commands may create local containers and access services declared
by that project.

## Start it manually

Both examples require a person to start the pipeline. GitHub Actions has only a
`workflow_dispatch` trigger. GitLab accepts only a web-started pipeline and
holds its job for a manual start. Neither runs on pushes or pull requests.

Choose a published semantic version, such as `0.4.28`. The workflow installs
the matching `baha` release; `baha up --control-plane-only` resolves the
matching runtime image for the local Core. Keep the version the same for the
entire job. Do not replace it with `latest` or a development build.

The runner needs a compatible Docker engine. GitHub-hosted Ubuntu runners
provide Docker. For GitLab, use a disposable runner registered with the
`baseharbor-ephemeral-docker` tag and a Docker socket; the sample installs the
Docker client and checks that it can reach the engine. Do not use a developer
workstation runner or a shared persistent Target.

## What the example does

The helper script runs these steps in one job:

1. Starts a local Core on the disposable runner.
2. Inspects the checked-out application and parses its JSON output.
3. Shows the application plan and runs preflight.
4. Optionally writes `APP_API_KEY` from a masked CI variable over stdin.
5. Runs `baha up --environment dev --yes` with a five-minute timeout.
6. Reads status and the environment contract as JSON, with a two-minute status
   timeout. PostgreSQL connection metadata is optional and written to a file,
   never the job log.
7. Stops the application when the script exits, including after a failure.

`--yes` approves only decisions that support that flag. It does not grant
provider-consequence consent or override other safety checks. A pipeline that
needs an additional decision fails closed under `--no-input`; resolve the
decision through the supported product flow before running the example.

## Configure inputs and secrets

- Set the released `BASEHARBOR_VERSION` in the GitLab pipeline variables or the
  GitHub Actions manual input.
- Set `APP_API_KEY` as a masked/protected secret only if the application
  declares the `API_KEY` secret. The helper sends the value on stdin and never
  prints it. Remove that optional block if the application does not use it.
- Set `APP_CHECK_POSTGRES_CREDS=true` only when the application declares a
  PostgreSQL connection. The resulting connection metadata stays in a local
  JSON file on the ephemeral runner.
- Change `APP_ENVIRONMENT` to an environment declared by the application.

The helper writes JSON to files and parses it before continuing. It propagates
non-zero command results and uses `timeout` for lifecycle/readiness operations.
The fixture tests run the exact helper with a controlled `baha` executable;
they do not contact GitHub, GitLab, a Core, or a customer service.

## Examples

- [GitHub Actions](../example/github-actions/README.md)
- [GitLab CI](../example/gitlab-ci/README.md)
