#!/bin/bash
# f65 gate runner: $1 = run number (1|2)
set -u
N="$1"
E=/home/marcel/dev/baseharbor-ecosystem
D=$E/evidence/board-2026-10-25/f65-gate-data
mkdir -p "$D/artifacts-$N"
if pgrep -f "acceptance.sh" | grep -qv $$; then
  echo "another acceptance.sh is running - refusing"; exit 3
fi
cd /home/marcel/tmp-ops/demo-run
env \
 DOCKER_HOST=unix:///run/user/1000/docker.sock \
 BASEHARBOR_TARGET=atomic-docker-data-capabilities \
 BASEHARBOR_PREBUILT_BINARY=/home/marcel/tmp-ops/bin-f65/baha \
 BASEHARBOR_RUNTIME_IMAGE=localhost/baseharbor-runtime:f65 \
 BASEHARBOR_SOURCE_REF="$F65_COMMIT" \
 ARTIFACT_DIR="$D/artifacts-$N" \
 bash scripts/acceptance.sh data-capabilities > "$D/gate-run$N.log" 2>&1
echo "gate-exit=$?" >> "$D/gate-run$N.log"
