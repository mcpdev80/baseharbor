#!/usr/bin/env bash
# todo-check.sh - true per-session todo states from OpenCode DB (codeman GUI source)
set -euo pipefail
python3 - <<'PY'
import sqlite3, shutil
con=sqlite3.connect('file:/home/marcel/.local/share/opencode/opencode.db?mode=ro',uri=True,timeout=5)
for title,st,n in con.execute("""
 select s.title, t.status, count(*) from todo t join session s on t.session_id=s.id
 where s.time_updated > (strftime('%s','now')-7200)*1000
 group by s.title, t.status order by s.title"""):
    icon={'completed':'x','in_progress':'>','pending':' ',
          'cancelled':'-','error':'!'}.get(st,'?')
    print(f"[{icon}] {title[:52]:52} {st:12} x{n}")
PY