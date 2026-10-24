import pathlib,subprocess,json,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf13-data');h='7c7cda7cc32423405d3cabe3ecf46d61289b0598'
a={'identity':['gh','api','user','--jq','.login'],'pr-before':['gh','api','repos/mcpdev80/baseharbor/pulls/1052'],'develop-before':['gh','api','repos/mcpdev80/baseharbor/commits/develop'],'files':['gh','api','repos/mcpdev80/baseharbor/pulls/1052/files?per_page=100'],'diff':['gh','pr','diff','1052','--repo','mcpdev80/baseharbor'],'checks':['gh','pr','checks','1052','--repo','mcpdev80/baseharbor'],'check-runs':['gh','api','repos/mcpdev80/baseharbor/commits/'+h+'/check-runs'],'pr1060':['gh','api','repos/mcpdev80/baseharbor/pulls/1060'],'files1060':['gh','api','repos/mcpdev80/baseharbor/pulls/1060/files?per_page=100'],'run':['gh','api','repos/mcpdev80/baseharbor/actions/runs/40092965161']}
def f(t):
 n,c=t;r=subprocess.run(c,capture_output=True,text=True);(p/(n+'.txt')).write_text(r.stdout);(p/(n+'.stderr')).write_text(r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=5) as e:print(list(e.map(f,a.items())))
