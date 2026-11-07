#!/usr/bin/env bash
set -euo pipefail

check() {
  local script="$1" path="$2" expected="$3" actual
  actual="$(printf '%s\n' "$path" | bash "$script")"
  if [[ "$actual" != "$expected" ]]; then
    printf 'FAIL: %s: %s expected %s, got %s\n' "$script" "$path" "$expected" "$actual" >&2
    exit 1
  fi
}

for path in \
  cmd/baha/operator_tenant_bootstrap.go \
  cmd/baha/serve_operator_oidc.go \
  cmd/baha/onboarding_command.go \
  internal/onboardingwizard/onboardingwizard.go \
  cmd/baha/node.go \
  internal/targetenrollment/http.go \
  internal/database/operator_bootstrap.go; do
  check scripts/ci-keycloak-single-impact.sh "$path" true
  check scripts/ci-single-default-impact.sh "$path" true
done

for path in \
  cmd/baha/serve.go \
  cmd/baha/app_remote_lifecycle.go \
  internal/controlplaneruntime/server_run.go \
  scripts/ci-owner-node-native.sh; do
  check scripts/ci-single-default-impact.sh "$path" true
done

check scripts/ci-keycloak-single-impact.sh internal/identityprovider/keycloak.go true
check scripts/ci-single-default-impact.sh internal/runtime/stack.go true

for script in scripts/ci-keycloak-single-impact.sh scripts/ci-single-default-impact.sh; do
  check "$script" docs/tutorials/getting-started.md false
  check "$script" internal/unrelated/example.txt false
done

printf 'PASS: native Core and Keycloak develop impact selectors\n'
