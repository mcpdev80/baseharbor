#!/usr/bin/env bash
set -euo pipefail

required=(
  docs/spec/credential-access-v1.md
  docs/spec/management-access-v1.md
  docs/spec/availability-v1.md
  docs/spec/application-consumption-v1.md
  docs/spec/provider-management-acceptance-v1.md
  docs/releases/v0.4.21.md
  docs/releases/v0.4.21.demo-ref
)

for file in "${required[@]}"; do
  test -s "$file" || {
    echo "v0.4.21 readiness: missing required artifact: $file" >&2
    exit 1
  }
done

grep -Fq 'Class C' docs/spec/credential-access-v1.md
grep -Fq 'native-oidc' docs/spec/management-access-v1.md
grep -Fq 'standards-auth-adapter' docs/spec/management-access-v1.md
grep -Fq 'UNSUPPORTED' docs/spec/provider-management-acceptance-v1.md
grep -Fq 'ha: true' docs/spec/availability-v1.md
grep -Fq 'application_id' docs/spec/application-consumption-v1.md
grep -Fq 'AvailabilitySupportForProvider' internal/capability/availability.go
grep -Fq 'ResolveAvailability' internal/application/availability_resolution.go
grep -Fq 'application secret confirmation does not match; no changes were made' cmd/baha/app_secret.go

release_internal="$(
  grep -n -Ei 'unit test|conformance|candidate sha|gofmt|go test|go vet|implementation evidence|validation|CI gate' docs/releases/v0.4.21.md || true
)"
if [ -n "$release_internal" ]; then
  echo "v0.4.21 readiness: internal validation detail leaked into release notes:" >&2
  echo "$release_internal" >&2
  exit 1
fi

echo "v0.4.21 readiness PASS"
