#!/usr/bin/env bash
# todo-check.sh - true per-session todo states from OpenCode DB (codeman GUI source)
set -euo pipefail
cp /home/marcel/.local/share/opencode/opencode.db /tmp/opencode/ro.db 2>/dev/null || true
python3 - <<'PY'
import sqlite3, shutil
shutil.copy('/tmp/opencode/ro.db','/tmp/opencode/ro2.db')
for f in ('/tmp/opencode/ro.db-wal','/tmp/opencode/opencode.db-wal'):
    try: shutil.copy(f, '/tmp/opencode/ro2.db-wal')
    except Exception: pass
con=sqlite3.connect('file:/tmp/opencode/ro2.db?mode=ro',uri=True)
for title,st,n in con.execute("""
 select s.title, t.status, count(*) from todo t join session s on t.session_id=s.id
 where s.time_updated > (strftime('%s','now')-7200)*1000
 group by s.title, t.status order by s.title"""):
    icon={'completed':'x','in_progress':'>','pending':' ',
          'cancelled':'-','error':'!'}.get(st,'?')
    print(f"[{icon}] {title[:52]:52} {st:12} x{n}")
PY
