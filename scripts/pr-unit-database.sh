#!/usr/bin/env bash
# Ephemeral PostgreSQL fixture for PR go-unit-tests only.
set -euo pipefail

name="baha-pr-unit-db-${GITHUB_RUN_ID:-local}-$$"
cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --detach --rm --name "$name" \
  --publish 127.0.0.1:5433:5432 \
  --env POSTGRES_DB=baseharbor_test \
  --env POSTGRES_USER=postgres \
  --env POSTGRES_PASSWORD=postgres \
  postgres:18-alpine >/dev/null

ready=false
for _ in $(seq 1 30); do
  if docker exec "$name" pg_isready -U postgres -d baseharbor_test >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 2
done
if [ "$ready" != true ]; then
  docker logs "$name" >&2 || true
  echo 'PR Go unit test database did not become ready' >&2
  exit 1
fi

export BASEHARBOR_TEST_DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5433/baseharbor_test?sslmode=disable"
go test ./... -count=1
