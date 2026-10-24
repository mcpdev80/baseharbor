import subprocess,json,pathlib,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf9-data')
calls={'identity':['gh','api','user','--jq','.login'],'pr1052-before':['gh','api','repos/mcpdev80/baseharbor/pulls/1052'],'files':['gh','api','repos/mcpdev80/baseharbor/pulls/1052/files?per_page=100'],'checks':['gh','pr','checks','1052','--repo','mcpdev80/baseharbor'],'develop-before':['gh','api','repos/mcpdev80/baseharbor/commits/develop'],'pr1060':['gh','api','repos/mcpdev80/baseharbor/pulls/1060'],'files1060':['gh','api','repos/mcpdev80/baseharbor/pulls/1060/files?per_page=100'],'diff':['gh','pr','diff','1052','--repo','mcpdev80/baseharbor'],'plan-tree':['gh','api','repos/mcpdev80/baseharbor-plan/git/trees/main?recursive=1'],'rules':['gh','api','repos/mcpdev80/baseharbor/rules/branches/develop']}
def f(a):
 n,c=a;r=subprocess.run(c,capture_output=True,text=True);(p/(n+'.txt')).write_text(r.stdout);(p/(n+'.stderr')).write_text(r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as e:print(list(e.map(f,calls.items())))
