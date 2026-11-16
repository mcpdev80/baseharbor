#!/usr/bin/env bash
# Native Identity/Keycloak acceptance on develop. Keep in sync with operator onboarding.
set -euo pipefail
required=false
while IFS= read -r path; do
  case "$path" in
    internal/identityprovider/*.go|internal/operatorauth/*.go|internal/identity/*.go|internal/database/operator_bootstrap*.go|cmd/baha/application_identity*.go|cmd/baha/app_identity*.go|cmd/baha/operator_*.go|cmd/baha/serve_operator_oidc*.go|cmd/baha/onboarding*.go|internal/onboardingwizard/*.go|cmd/baha/node*.go|internal/targetenrollment/*.go|scripts/ci-keycloak-single-impact.sh|scripts/test-ci-impact-selectors.sh)
      required=true
      ;;
  esac
done
printf '%s\n' "$required"
