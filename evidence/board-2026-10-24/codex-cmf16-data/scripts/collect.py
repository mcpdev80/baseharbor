import pathlib,subprocess,json,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf16-data')
a={'identity':['gh','api','user','--jq','.login'],'pr-before':['gh','api','repos/mcpdev80/baseharbor/pulls/1052'],'develop-before':['gh','api','repos/mcpdev80/baseharbor/commits/develop'],'files':['gh','api','repos/mcpdev80/baseharbor/pulls/1052/files?per_page=100'],'diff':['gh','pr','diff','1052','--repo','mcpdev80/baseharbor'],'checks':['gh','pr','checks','1052','--repo','mcpdev80/baseharbor'],'pr1060':['gh','api','repos/mcpdev80/baseharbor/pulls/1060']}
def f(t):
 n,c=t;r=subprocess.run(c,capture_output=True,text=True);(p/(n+'.txt')).write_text(r.stdout);(p/(n+'.stderr')).write_text(r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=5) as e:print(list(e.map(f,a.items())))
h=json.loads((p/'pr-before.txt').read_text())['head']['sha'];print('head',h)
for n,u in [('check-runs','repos/mcpdev80/baseharbor/commits/'+h+'/check-runs'),('runs','repos/mcpdev80/baseharbor/actions/runs?head_sha='+h+'&per_page=100')]:
 r=subprocess.run(['gh','api',u],capture_output=True,text=True);(p/(n+'.txt')).write_text(r.stdout);print(n,r.returncode)
