import subprocess,pathlib,json,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf11-data')
a={}
for n in ['1056','1060']:
 a['B-pr'+n]=['gh','api','repos/mcpdev80/baseharbor/pulls/'+n]
 a['B-files'+n]=['gh','api','repos/mcpdev80/baseharbor/pulls/'+n+'/files?per_page=100']
 a['B-checks'+n]=['gh','pr','checks',n,'--repo','mcpdev80/baseharbor']
def f(t):
 n,c=t;r=subprocess.run(c,capture_output=True,text=True);(p/(n+'.txt')).write_text(r.stdout);(p/(n+'.stderr')).write_text(r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as e:print(list(e.map(f,a.items())))
