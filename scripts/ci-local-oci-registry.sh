#!/usr/bin/env bash
set -euo pipefail

action="${1:?usage: ci-local-oci-registry.sh publish <source> <target> | cleanup}"
runtime="${BASEHARBOR_TEST_RUNTIME:?BASEHARBOR_TEST_RUNTIME must be docker or podman}"
name="baseharbor-pre-release-registry"
local_registry_config="$HOME/.config/containers/registries.conf.d/99-baseharbor-pre-release-registry.conf"

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
    if [ "$runtime" = podman ]; then
      rm -f "$local_registry_config"
    fi
    ;;
  publish)
    source_image="${2:?source image is required}"
    target_image="${3:?target OCI image is required}"
    [[ "$target_image" == localhost:5000/*:* ]] || {
      echo "target image must use the isolated localhost:5000 registry" >&2
      exit 2
    }
    # A runtime reset (ci-runtime-reset.sh) destroys the loaded runtime image
    # and the localhost:5000 registry container before this publish runs. If
    # the source image is gone, re-load it from the candidate archive so the
    # tag/push/ORAS chain has an image to publish; without this the gate sees
    # an unknown blob/manifest on the runtime image it depends on.
    if ! runtime_cmd image inspect "$source_image" >/dev/null 2>&1; then
      archive="${BASEHARBOR_CANDIDATE_ARCHIVE:-$RUNNER_TEMP/pre-release-candidate/baseharbor-runtime-pre-release.tar}"
      if [ -s "$archive" ]; then
        if [ "$runtime" = podman ]; then
          env -u XDG_CONFIG_HOME -u XDG_DATA_HOME podman load -i "$archive"
        else
          docker load -i "$archive"
        fi
      else
        echo "runtime source image '$source_image' is missing and no candidate archive ($archive) is available" >&2
        exit 1
      fi
    fi
    runtime_cmd image inspect "$source_image" >/dev/null
    if [ "$runtime" = podman ]; then
      mkdir -p "$(dirname "$local_registry_config")"
      cat > "$local_registry_config" <<EOF
[[registry]]
location = "localhost:5000"
insecure = true
EOF
    fi
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
