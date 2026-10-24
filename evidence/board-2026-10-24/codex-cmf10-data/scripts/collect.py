import pathlib,subprocess,concurrent.futures,json
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf10-data')
a={'identity':['gh','api','user','--jq','.login'],'pr1046-before':['gh','api','repos/mcpdev80/baseharbor/pulls/1046'],'develop-before':['gh','api','repos/mcpdev80/baseharbor/commits/develop'],'files':['gh','api','repos/mcpdev80/baseharbor/pulls/1046/files?per_page=100'],'checks':['gh','pr','checks','1046','--repo','mcpdev80/baseharbor'],'diff':['gh','pr','diff','1046','--repo','mcpdev80/baseharbor'],'pr1056':['gh','api','repos/mcpdev80/baseharbor/pulls/1056'],'pr1060':['gh','api','repos/mcpdev80/baseharbor/pulls/1060'],'pr1063':['gh','api','repos/mcpdev80/baseharbor/pulls/1063'],'check-runs':['gh','api','repos/mcpdev80/baseharbor/commits/1ec67f8d4b259d62bd108ee678f25c288b8b0564/check-runs']}
def f(t):
 n,c=t;r=subprocess.run(c,capture_output=True,text=True);(p/(n+'.txt')).write_text(r.stdout);(p/(n+'.stderr')).write_text(r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as e:print(list(e.map(f,a.items())))
