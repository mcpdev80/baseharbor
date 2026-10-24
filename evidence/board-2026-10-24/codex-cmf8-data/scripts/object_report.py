import pathlib,json,subprocess,hashlib
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf8-data');g=['git','--git-dir=/tmp/cmf8-review/objects.git']
def git(*args):return subprocess.check_output(g+list(args))
pr=json.loads((p/'demo-before.txt').read_text());main=pr['base']['sha'];head=pr['head']['sha'];run=json.loads((p/'run.txt').read_text());old=run['head_sha']
print('PR',pr['state'],main,head,'run',old,run['conclusion'],run['run_attempt']);print('host',(p/'host.txt').read_text());print('rules',(p/'rules.txt').read_text());print('checks',(p/'checks.txt').read_text(),(p/'checks.stderr').read_text())
refs=git('show','FETCH_HEAD').decode(errors='replace')[:0]
release=git('rev-parse','FETCH_HEAD').decode().strip()
fetch=(pathlib.Path('/tmp/cmf8-review/objects.git/FETCH_HEAD').read_text());(p/'fetch-head.txt').write_text(fetch);print(fetch)
release=[x.split('\t')[0] for x in fetch.splitlines() if "branch 'work/" in x][0]
print('release',release,'aheadbehind',git('rev-list','--left-right','--count',release+'...'+head).decode())
records=[]
for label,sha in [('main',main),('candidate',head),('run',old),('release',release)]:
 print(label,sha,'tree',git('rev-parse',sha+'^{tree}').decode().strip())
 for f in ['baseharbor-core.ref','scripts/failure-runtime-diagnostics.sh','.github/workflows/v0424-targeted.yml','scripts/acceptance.sh']:
  b=git('show',sha+':'+f);records.append(dict(label=label,sha=sha,path=f,blob=git('rev-parse',sha+':'+f).decode().strip(),sha256=hashlib.sha256(b).hexdigest()))
  (p/(label+'-'+f.replace('/','_'))).write_bytes(b)
(p/'hashes.json').write_text(json.dumps(records,indent=2));print(json.dumps(records,indent=2))
for name,a,b in [('base-candidate',main,head),('run-candidate',old,head)]:
 diff=git('diff',a,b).decode();(p/(name+'.diff')).write_text(diff);print(name,diff)
for n in ['console9','connector10']:
 d=json.loads((p/(n+'.txt')).read_text());print(n,d['state'],d['head']['sha'],d['base']['sha']);print((p/(n.split('9')[0].split('10')[0]+'-diff.txt')).read_text() if False else d['body'])
for j in json.loads((p/'jobs.txt').read_text())['jobs']:print('JOB',j['id'],j['name'],j['conclusion'])
t=json.loads((p/'plan-tree.txt').read_text());print('TASKS',[x['path'] for x in t['tree'] if x['path'].startswith('tasks/') and any(w in x['path'] for w in ['freeze','repin','ecosystem','candidate'])])
