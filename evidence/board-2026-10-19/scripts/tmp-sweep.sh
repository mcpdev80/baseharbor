#!/usr/bin/env bash
# tmp-sweep.sh [ALTER_STUNDEN]  (Default 6)
# RAM-Retter: /tmp ist tmpfs und zaehlt als RAM. Dieses Script loescht
# Altlasten (Lane-Klone, DB-Kopien, Repro-Verzeichnisse, Extrakte) die
# aelter als ALTER_STUNDEN sind, ohne laufende Arbeit zu zerstoeren.
# Schuetzt: registrierte git-worktrees, tmux/ssh/runtime-Sockets, node-compile-cache,
# /tmp/opencode (Tool-Outputs juinger als das Limit) und alles unter $BH_EPHEMERAL_DIR.
set -uo pipefail
AGE_H="${1:-6}"
ROOT="${BH_ROOT_DIR:-/home/marcel/dev/baseharbor-ecosystem}"
PROTECT_LIST=()
for r in baseharbor baseharbor-plan baseharbor-console baseharbor-demo baseharbor-node-connector; do
  while read -r wt; do [[ -n "$wt" ]] && PROTECT_LIST+=("$wt"); done \
    < <(git -C "$ROOT/$r" worktree list --porcelain 2>/dev/null | awk '/^worktree /{print $2}')
done
BEFORE=$(df -k --output=avail /tmp 2>/dev/null | tail -1)
find /tmp -maxdepth 1 -mindepth 1 ! -newermt "-${AGE_H} hours" 2>/dev/null | while read -r p; do
  base="$(basename "$p")"
  case "$base" in
    tmux-*|systemd-*|.X*-lock|.ICE-*|.X11-*|ssh-*|node-compile-cache|opencode|baha-*-bin|snap.*|fontconfig-*|dbus-*|keyring-*|polkitd-*|ksocket-*) continue ;;
  esac
  # Bun/V8 write .<hex>-<n>.so payload files per process start; ~560 of them at
  # 14MB each were the real RAM hog on this host (5GB in tmpfs). Old ones are
  # pure garbage and get recreated on demand.
  case "$base" in
    .[0-9a-f]*-*.so) rm -f "$p" 2>/dev/null || true; continue ;;
  esac
  for w in "${PROTECT_LIST[@]:-}"; do
    [[ -n "$w" && "$(readlink -f "$p" 2>/dev/null)" == "$(readlink -f "$w" 2>/dev/null)" ]] && continue 2
  done
  if [[ -d "$p" && ! -L "$p" ]]; then rm -rf "$p" 2>/dev/null || true; else rm -f "$p" 2>/dev/null || true; fi
done
AFTER=$(df -k --output=avail /tmp 2>/dev/null | tail -1)
# Ephemera auf Disk altern lassen: $TMPDIR (sollte /home/marcel/tmp-ops sein) nur >24h
DISK_TMP="${TMPDIR:-/home/marcel/tmp-ops}"
if [[ "$DISK_TMP" == /home/* ]]; then
  # Keep-Guard: coord-tick.sh hat ein Sweep am 30.10. weggelegt (laufender Daemon verlor seine Datei)
  for keep in coord-tick.sh coord-tick.log coord-tick.lastfire coord-tick.pid watcher-state.json prune-watch.sh ci-watch.sh; do
    [ -e "$DISK_TMP/$keep" ] && find "$DISK_TMP/$keep" -exec touch -d "now" {} + 2>/dev/null || true
  done
  find "$DISK_TMP" -maxdepth 1 -mindepth 1 ! -newermt "-24 hours" -exec rm -rf {} + 2>/dev/null || true
fi
echo "tmp-sweep: /tmp avail ${BEFORE}kB -> ${AFTER}kB (age>${AGE_H}h, protected=${#PROTECT_LIST[@]} worktrees)"
