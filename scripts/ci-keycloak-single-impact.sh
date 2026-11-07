#!/usr/bin/env bash
# Native Identity+dedicated SQL only. Shared Core SQL/HA has separate acceptance.
set -euo pipefail
required=false
while IFS= read -r path; do
  case "$path" in
    internal/identityprovider/keycloak*.go|internal/identityprovider/core.go|cmd/baha/application_identity*.go|cmd/baha/app_identity*.go)
      required=true
      ;;
  esac
done
printf '%s\n' "$required"
