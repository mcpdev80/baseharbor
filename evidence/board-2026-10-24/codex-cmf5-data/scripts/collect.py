import subprocess,json,concurrent.futures
from pathlib import Path
p=Path('evidence/board-2026-10-24/codex-cmf5-data')
tasks={'identity':['gh','api','user'], 'core-develop':['gh','api','repos/mcpdev80/baseharbor/commits/develop'], 'demo-main':['gh','api','repos/mcpdev80/baseharbor-demo/commits/main'],'demo-rules':['gh','api','repos/mcpdev80/baseharbor-demo/rules/branches/main'],'run':['gh','run','view','39990941852','--repo','mcpdev80/baseharbor-demo','--json','headSha,event,status,conclusion,jobs,url,workflowName,createdAt,updatedAt'],'demo-checks':['gh','pr','checks','121','--repo','mcpdev80/baseharbor-demo'],'demo-release':['gh','api','repos/mcpdev80/baseharbor-demo/commits/work/v0.4.26-ecosystem-release']}
for n in [1044,1047,1053,1058,1059,1056,1050,1060]:tasks[f'core-pr{n}']=['gh','api',f'repos/mcpdev80/baseharbor/pulls/{n}']
tasks['demo-pr121']=['gh','api','repos/mcpdev80/baseharbor-demo/pulls/121']
def run(item):
 n,c=item;r=subprocess.run(c,capture_output=True,text=True);(p/(n+('.json' if r.returncode==0 and c[1]!='pr' else '.txt'))).write_text(r.stdout+r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as ex:
 for r in ex.map(run,tasks.items()):print(r)
