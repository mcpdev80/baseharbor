import subprocess,json,pathlib,concurrent.futures,datetime
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf11-data');ev=p.parent/'codex-verdict-q1063-r2.md';Q='7170c1b67459d4c4fccb1a0bd41b11d5cec52f51';B='f39295680d41290fc505c0a8cc1769da51699293'
urls={'pr':'repos/mcpdev80/baseharbor/pulls/1063','base':'repos/mcpdev80/baseharbor/commits/develop','checks':'repos/mcpdev80/baseharbor/commits/'+Q+'/check-runs','rules':'repos/mcpdev80/baseharbor/rules/branches/develop','user':'user'}
def f(t):
 n,u=t;r=subprocess.run(['gh','api',u],capture_output=True,text=True);(p/('premerge-'+n+'.json')).write_text(r.stdout);assert r.returncode==0,(n,r.stderr);return n,json.loads(r.stdout)
with concurrent.futures.ThreadPoolExecutor(max_workers=5) as e:d=dict(e.map(f,urls.items()))
assert d['user']['login']=='mcpdev80' and d['user']['id']==22537373
assert d['pr']['state']=='open' and not d['pr']['merged'] and d['pr']['head']['sha']==Q and d['pr']['base']['sha']==B
assert d['base']['sha']==B and d['pr']['mergeable'] is True
assert d['rules']==json.loads((p/'rules.txt').read_text())
checks=d['checks']['check_runs'];assert checks and all(c['status']=='completed' and c['conclusion'] in ['success','skipped','neutral'] for c in checks)
required=[c for c in checks if c['name']=='PR Selected Checks'];assert len(required)==1 and required[0]['app']['id']==15368 and required[0]['conclusion']=='success' and required[0]['head_sha']==Q
r=subprocess.run(['gh','api','graphql','-f','query=query { repository(owner:"mcpdev80",name:"baseharbor") { pullRequest(number:1063) { headRefOid baseRefOid mergeable mergeStateStatus reviewDecision reviewThreads(first:100) { nodes { isResolved } pageInfo { hasNextPage } } } } }'],capture_output=True,text=True);(p/'premerge-threads.json').write_text(r.stdout);assert r.returncode==0;t=json.loads(r.stdout)['data']['repository']['pullRequest'];assert t['headRefOid']==Q and t['baseRefOid']==B and t['mergeable']=='MERGEABLE' and not t['reviewThreads']['pageInfo']['hasNextPage'] and all(z['isResolved'] for z in t['reviewThreads']['nodes'])
with ev.open('a') as e:e.write('\n## Phase A — ROLLE: MERGE\n\nUTC transition: '+datetime.datetime.now(datetime.timezone.utc).isoformat()+'\n\nFresh guarded checks passed for exact Q/B/account/rules/threads and app15368 green. Executing explicitly Owner-authorized formal-review exception with expected-head guard; no branch deletion, CI bypass or protection modification.\n')
cmd=['gh','pr','merge','1063','--squash','--admin','--match-head-commit',Q,'--repo','mcpdev80/baseharbor'];r=subprocess.run(cmd,capture_output=True,text=True);(p/'merge.stdout').write_text(r.stdout);(p/'merge.stderr').write_text(r.stderr);(p/'merge-result.json').write_text(json.dumps({'command':cmd,'returncode':r.returncode,'utc':datetime.datetime.now(datetime.timezone.utc).isoformat()},indent=2));print('merge returncode',r.returncode);print(r.stdout);print(r.stderr)
