# GitLab CI consumption example

This pipeline accepts only a manually started **Run pipeline** action. Its job
also requires a manual start, and it does not run for pushes or merge requests.
The job uses one disposable runner so Core and application state stay together.

Copy `pipeline.yml` and `scripts/consumption-example.sh` into an application
repository that contains a valid `baseharbor.yaml` and workload source. Register
a disposable runner with the `baseharbor-ephemeral-docker` tag and a Docker
socket. Use a disposable application and disposable secrets; the example is
not for a customer Target.

Set `BASEHARBOR_VERSION` to a published release. Add `APP_API_KEY` as a masked
and protected CI/CD variable only when the application declares `API_KEY`. Set
`APP_CHECK_POSTGRES_CREDS=true` only when the application has a PostgreSQL
connection. See the [CI consumption guide](../../how-to/ci-consumption.md) for
the safety limits and other inputs.
