#!/usr/bin/env bash
# Emit true only for changes that can alter the native control-plane footprint.
# Used by the existing Docker/Podman acceptance workflows on develop pushes.
set -euo pipefail

required=false
while IFS= read -r path; do
  case "$path" in
    cmd/baha/runtime*.go|cmd/baha/path_prompt_context.go|cmd/baha/control_plane_single_native_ci_test.go|internal/runtime/stack*.go|internal/runtime/assets/compose*.yaml|internal/runtime/*control*profile*.go)
      required=true
      ;;
  esac
done
printf '%s\n' "$required"
