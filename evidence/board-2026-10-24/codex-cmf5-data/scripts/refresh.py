from pathlib import Path
import json,subprocess,concurrent.futures,base64,hashlib
from datetime import datetime,timezone
p=Path('evidence/board-2026-10-24/codex-cmf5-data')
endpoints={'core-develop':'repos/mcpdev80/baseharbor/commits/develop','core-pr1056':'repos/mcpdev80/baseharbor/pulls/1056','demo-pr121':'repos/mcpdev80/baseharbor-demo/pulls/121','demo-main':'repos/mcpdev80/baseharbor-demo/commits/main','demo-release':'repos/mcpdev80/baseharbor-demo/commits/work/v0.4.26-ecosystem-release'}
def run(t):
 name,url=t;r=subprocess.run(['gh','api',url],capture_output=True,text=True);assert r.returncode==0,r.stderr;(p/(name+'-after.json')).write_text(r.stdout);return name
with concurrent.futures.ThreadPoolExecutor(max_workers=5) as ex:print(list(ex.map(run,endpoints.items())))
(p/'refresh-utc.txt').write_text(datetime.now(timezone.utc).isoformat())
