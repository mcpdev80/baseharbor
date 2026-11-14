#!/usr/bin/env bash
set -euo pipefail

action="${1:?usage: ci-local-oci-registry.sh publish <source> <target> | cleanup}"
runtime="${BASEHARBOR_TEST_RUNTIME:?BASEHARBOR_TEST_RUNTIME must be docker or podman}"
name="baseharbor-pre-release-registry"

runtime_cmd() {
  if [ "$runtime" = podman ]; then
    env -u XDG_CONFIG_HOME -u XDG_DATA_HOME podman "$@"
  else
    docker "$@"
  fi
}

case "$action" in
  cleanup)
    runtime_cmd rm --force "$name" >/dev/null 2>&1 || true
    ;;
  publish)
    source_image="${2:?source image is required}"
    target_image="${3:?target OCI image is required}"
    [[ "$target_image" == localhost:5000/*:* ]] || {
      echo "target image must use the isolated localhost:5000 registry" >&2
      exit 2
    }
    runtime_cmd image inspect "$source_image" >/dev/null
    runtime_cmd run --detach --name "$name" --label baseharbor.pre-release.registry=true \
      --publish 127.0.0.1:5000:5000 docker.io/library/registry:2.8.3 >/dev/null
    ready=false
    for _ in $(seq 1 40); do
      if curl --silent --show-error --fail http://127.0.0.1:5000/v2/ >/dev/null; then
        ready=true
        break
      fi
      sleep 0.5
    done
    $ready || {
      runtime_cmd logs "$name" >&2 || true
      echo "isolated local OCI registry did not become ready" >&2
      exit 1
    }
    runtime_cmd image tag "$source_image" "$target_image"
    if [ "$runtime" = podman ]; then
      runtime_cmd push --tls-verify=false "$target_image"
    else
      runtime_cmd push "$target_image"
    fi
    digest="$(oras resolve --plain-http "$target_image")"
    [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || {
      echo "ORAS did not resolve the published runtime image to an immutable digest" >&2
      exit 1
    }
    echo "BASEHARBOR_RUNTIME_IMAGE=$target_image" >> "$GITHUB_ENV"
    echo "Published verified runtime image $target_image ($digest)." >> "$GITHUB_STEP_SUMMARY"
    ;;
  *)
    echo "unsupported action: $action" >&2
    exit 2
    ;;
esac
