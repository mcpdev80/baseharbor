#!/usr/bin/env bash
# Emit true only for changes that can alter the native control-plane footprint.
# Used by the existing Docker/Podman acceptance workflows on develop pushes.
set -euo pipefail

required=false
while IFS= read -r path; do
  case "$path" in
    cmd/baha/runtime.go|cmd/baha/runtime_start.go|cmd/baha/runtime_ports_test.go|cmd/baha/control_plane_single_native_ci_test.go|internal/runtime/stack.go|internal/runtime/stack_test.go|internal/runtime/control_plane_profile.go|internal/runtime/control_plane_profile_test.go|internal/runtime/assets/compose*.yaml)
      required=true
      ;;
  esac
done
printf '%s\n' "$required"
