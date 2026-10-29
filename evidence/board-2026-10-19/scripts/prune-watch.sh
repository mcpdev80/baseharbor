#!/usr/bin/env bash
# prune-watch.sh - drop dead sessions from watcher-state.json (run while daemon stopped)
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
STATE="$DIR/watcher-state.json"
AGENTS_JSON="$(codeman agent list --json 2>/dev/null)" || { echo "prune: agent list failed" >&2; exit 7; }
python3 - "$STATE" "$AGENTS_JSON" <<'PYEOF'
import json, sys
state_path, agents_raw = sys.argv[1], sys.argv[2]
d = json.loads(agents_raw)
agents = d if isinstance(d, list) else d.get("agents", [])
live = {a["id"] for a in agents}
st = json.load(open(state_path))
live.add((st.get("watcher") or {}).get("coordinator") or "")
pruned = 0
for section in ("lane_state", "idle_streak", "last_status", "names", "compact_watch", "busy_since"):
    sec = st.get(section) or {}
    if isinstance(sec, dict):
        dead = [k for k in sec if k not in live]
        for k in dead:
            del sec[k]
            pruned += 1
for k in [k for k in (st.get("delivered") or {}) if k.split(":")[0] not in live]:
    del st["delivered"][k]
    pruned += 1
json.dump(st, open(state_path, "w"), indent=1)
print("pruned:", pruned)
PYEOF
