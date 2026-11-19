# GitHub Actions consumption example

This example starts only with a manual **Run workflow** action. It has no
`push` or pull-request trigger. It runs one job on a disposable GitHub-hosted
Ubuntu runner so the local Core and application state remain available for the
whole example.

Copy `consume.yml` and `scripts/consumption-example.sh` into an application
repository that contains a valid `baseharbor.yaml` and workload source. The
job uses the runner's Docker engine and stops the application before it exits.
Use a disposable application and disposable secrets; the workflow is not for a
customer Target.

Set the release version in the manual input. Add `APP_API_KEY` as a masked
GitHub Actions secret only when the application declares `API_KEY`. Set
`APP_CHECK_POSTGRES_CREDS=true` only for an application with a PostgreSQL
connection. See the [CI consumption guide](../../how-to/ci-consumption.md) for
the safety limits and other inputs.
