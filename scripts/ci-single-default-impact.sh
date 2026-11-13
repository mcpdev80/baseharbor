#!/usr/bin/env bash
# Emit true for any change affecting the native single-Core footprint or enrollment.
set -euo pipefail
required=false
while IFS= read -r path; do
  case "$path" in
    cmd/baha/runtime*.go|cmd/baha/management_remote_installation_destroy.go|cmd/baha/installation_destroy_native_test.go|cmd/baha/app_workload_integration_test.go|cmd/baha/control_plane*.go|cmd/baha/serve*.go|cmd/baha/node*.go|cmd/baha/operator_*.go|cmd/baha/onboarding*.go|cmd/baha/app_remote*.go|internal/runtime/*.go|internal/runtime/assets/compose*.yaml|internal/controlplaneruntime/*.go|internal/coreinstallation/*.go|internal/onboardingwizard/*.go|internal/targetenrollment/*.go|internal/database/operator_bootstrap*.go|scripts/ci-owner-node-native.sh|scripts/ci-node-contract-acceptance.sh|scripts/ci-native-installation-destroy.sh|.github/workflows/docker-runtime-acceptance.yml|.github/workflows/podman-acceptance.yml|scripts/ci-single-default-impact.sh|scripts/test-ci-impact-selectors.sh)
      required=true
      ;;
  esac
done
printf '%s\n' "$required"
