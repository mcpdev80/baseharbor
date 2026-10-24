import subprocess,json,concurrent.futures,pathlib
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf8-data')
calls={
'identity':['gh','api','user','--jq','.login'],
'demo-before':['gh','api','repos/mcpdev80/baseharbor-demo/pulls/121'],
'demo-main-before':['gh','api','repos/mcpdev80/baseharbor-demo/commits/main'],
'checks':['gh','pr','checks','121','--repo','mcpdev80/baseharbor-demo'],
'diff':['gh','pr','diff','121','--repo','mcpdev80/baseharbor-demo'],
'run':['gh','api','repos/mcpdev80/baseharbor-demo/actions/runs/39990941852'],
'jobs':['gh','api','repos/mcpdev80/baseharbor-demo/actions/runs/39990941852/jobs'],
'rules':['gh','api','repos/mcpdev80/baseharbor-demo/rules/branches/main'],
'runs':['gh','api','repos/mcpdev80/baseharbor-demo/actions/runs?per_page=10'],
'console9':['gh','api','repos/mcpdev80/baseharbor-console/pulls/9'],
'connector10':['gh','api','repos/mcpdev80/baseharbor-node-connector/pulls/10'],
'console-diff':['gh','pr','diff','9','--repo','mcpdev80/baseharbor-console'],
'connector-diff':['gh','pr','diff','10','--repo','mcpdev80/baseharbor-node-connector'],
'plan-tree':['gh','api','repos/mcpdev80/baseharbor-plan/git/trees/main?recursive=1'],
'host':['bash','-c','date -u --iso-8601=seconds; type -p podman; printf "podman_type_rc=%s\\n" "$?"; type -p docker; uname -sr'],
}
def f(item):
 n,c=item;r=subprocess.run(c,capture_output=True,text=True);(p/(n+'.txt')).write_text(r.stdout);(p/(n+'.stderr')).write_text(r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as ex: print(json.dumps(dict(ex.map(f,calls.items()))))
