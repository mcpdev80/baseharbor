#!/usr/bin/env bash
# Setzt den C2-final Core-SHA in baseharbor-demo/baseharbor-core.ref (D2-Pin-Draft, o49).
# usage: apply_d2_pin_sha.sh <C2-final-40-hex-sha>
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <C2-final-40-hex-sha>" >&2
  exit 2
fi

C2="$1"
if ! printf '%s' "$C2" | grep -Eq '^[0-9a-f]{40}$'; then
  echo "fehler: <C2-final> muss ein 40-hex SHA sein" >&2
  exit 2
fi

file="baseharbor-demo/baseharbor-core.ref"
printf '%s\n' "$C2" > "$file"
echo "geschrieben: $file = $C2"
echo "Nächstes (in baseharbor-demo): git add baseharbor-core.ref && git commit -m 'demo: pin core auf C2-final' && (späterer normaler Push durch MERGE-Lane)"
