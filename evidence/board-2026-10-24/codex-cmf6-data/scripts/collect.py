import subprocess,json,pathlib,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf6-data')
def run(x):
 name,args=x;r=subprocess.run(args,capture_output=True,text=True);(p/(name+'.json')).write_text(r.stdout);(p/(name+'.stderr')).write_text(r.stderr);return name,r.returncode,r.stdout[:180]
jobs=[('identity',['gh','api','user']),('develop',['gh','api','repos/mcpdev80/baseharbor/branches/develop']),('rules',['gh','api','repos/mcpdev80/baseharbor/rules/branches/develop'])]
for n in [1063,1056,1061,1062,1050]:
 jobs.extend([(f'pr{n}',['gh','api',f'repos/mcpdev80/baseharbor/pulls/{n}']),(f'files{n}',['gh','api',f'repos/mcpdev80/baseharbor/pulls/{n}/files?per_page=100'])])
jobs.extend([('checks1063',['gh','pr','checks','1063','-R','mcpdev80/baseharbor','--json','name,state,link,bucket,workflow']),('threads1063',['gh','api','graphql','-f','query={repository(owner:"mcpdev80",name:"baseharbor"){pullRequest(number:1063){headRefOid baseRefOid mergeStateStatus reviewDecision reviewThreads(first:100){nodes{isResolved isOutdated comments(first:20){nodes{author{login}body url}}}}reviews(first:100){nodes{author{login}state commit{oid}body}}}}}'])])
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as e:
 for result in e.map(run,jobs): print(result)
