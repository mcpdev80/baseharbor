import json,pathlib,subprocess,re,hashlib,base64,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf11-data');g=['git','--git-dir=/tmp/cmf11-review/objects.git'];R='32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2';Q='7170c1b67459d4c4fccb1a0bd41b11d5cec52f51';B='f39295680d41290fc505c0a8cc1769da51699293'
def git(*a):return subprocess.check_output(g+list(a))
raw=git('show',Q+':docs/internal/ci-binding-qualifications.md');(p/'record.md').write_bytes(raw);rec=json.loads(re.findall(rb'^```json\n(.*?)\n```$',raw,re.M|re.S)[0]);print('RECORD',json.dumps(rec,indent=2));assert rec['reviewed_head']==R
print('Q DIFF',git('diff',B,Q).decode());print('R DIFFSTAT',git('diff','--stat',B,R).decode());print('R parent',git('rev-parse',R+'^').decode());assert git('diff','--name-only',B,Q).decode().splitlines()==['docs/internal/ci-binding-qualifications.md']
expected=['scripts/pr-check-runner.py','scripts/pr-gate-plan.py','scripts/test_pr_gate_plan.py'];assert rec['files']=={f:'modified' for f in expected};assert set(rec['required_checks'])=={'pr-planner-tests','ci-policy','ci-executor-verifier-tests'}
assert rec['repository']=='mcpdev80/baseharbor' and rec['target_branch']=='develop'
assert git('diff','--name-only',B,R).decode().splitlines()==expected
hashes=[]
def f(path):
 data=git('show',R+':'+path);oid=git('rev-parse',R+':'+path).decode().strip();z=subprocess.check_output(['gh','api','repos/mcpdev80/baseharbor/git/blobs/'+oid]);(p/(path.replace('/','_')+'.api.json')).write_bytes(z);remote=base64.b64decode(json.loads(z)['content']);assert remote==data
 digest=hashlib.sha256(data).hexdigest();assert rec['file_hashes'][path]==digest;assert re.fullmatch('[0-9a-f]{64}',digest)
 (p/('R-'+path.replace('/','_'))).write_bytes(data);return dict(path=path,blob=oid,local_sha256=digest,github_blob_sha256=hashlib.sha256(remote).hexdigest(),match=True)
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as e:hashes=list(e.map(f,expected))
(p/'hashes.json').write_text(json.dumps(hashes,indent=2));print('HASHES',json.dumps(hashes,indent=2))
print('CHECKS',(p/'checks.txt').read_text());print('THREADS',(p/'threads.txt').read_text())
pr=json.loads((p/'pr1063-before.txt').read_text());print('PR',pr['head']['sha'],pr['base']['sha'],pr['mergeable'],pr['mergeable_state'],pr['state'],pr['merged'])
for c in json.loads((p/'check-runs.txt').read_text())['check_runs']:
 print('CHECK',c['name'],c['head_sha'],c['status'],c['conclusion'],c['app']['id'])
for n in ['rule-review','rule-checks']:
 d=json.loads((p/(n+'.txt')).read_text());print(n,d['enforcement'],d.get('bypass_actors'),d['rules'])
for t in json.loads((p/'plan-tree.txt').read_text())['tree']:
 if '1056' in t['path'] or 'binding' in t['path']:print('PLAN',t['path'])
