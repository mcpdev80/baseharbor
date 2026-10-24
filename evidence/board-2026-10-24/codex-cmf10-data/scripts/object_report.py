import json,pathlib,subprocess,hashlib,re
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf10-data');g=['git','--git-dir=/tmp/cmf10-review/objects.git']
def git(*a):return subprocess.check_output(g+list(a))
pr=json.loads((p/'pr1046-before.txt').read_text());h=pr['head']['sha'];b=json.loads((p/'develop-before.txt').read_text())['sha'];r='3558250837759113b760a112e99994135d31bc00'
print('PR',h,pr['base']['sha'],b,pr['mergeable'],pr['mergeable_state']);print('CHECKS',(p/'checks.txt').read_text());print('PR body',pr['body'])
for n in ['1056','1060','1063']:
 d=json.loads((p/('pr'+n+'.txt')).read_text());print(n,d['head']['sha'],d['state'],d['merged'])
files=json.loads((p/'files.txt').read_text());paths=[f['filename'] for f in files];print('FILES',[(f['filename'],f['status']) for f in files]);hashes=[]
for label,sha in [('candidate',h),('develop',b),('reviewed',r),('1056',json.loads((p/'pr1056.txt').read_text())['head']['sha'])]:
 for f in paths+['scripts/pr-gate-plan.py','scripts/test_pr_gate_plan.py','scripts/pr-check-runner.py','docs/internal/ci-binding-qualifications.md','.github/workflows/ci.yml']:
  c=subprocess.run(g+['show',sha+':'+f],capture_output=True)
  if c.returncode:continue
  d=c.stdout;(p/(label+'-'+f.replace('/','_'))).write_bytes(d);hashes.append(dict(label=label,sha=sha,path=f,sha256=hashlib.sha256(d).hexdigest(),blob=git('rev-parse',sha+':'+f).decode().strip()))
  if label=='candidate' and f in paths:print('TESTS',f,re.findall(rb'^func (Test\w+)\(',d,re.M))
(p/'hashes.json').write_text(json.dumps(hashes,indent=2))
for n,a,z in [('against-develop',b,h),('reviewed-to-candidate',r,h)]:
 d=git('diff',a,z,'--',*paths).decode();(p/(n+'.diff')).write_text(d);print(n,d)
print('CHAIN',git('log','--oneline','--max-count=12',h).decode());print('reviewed ancestor?',subprocess.run(g+['merge-base','--is-ancestor',r,h]).returncode)
for c in json.loads((p/'check-runs.txt').read_text())['check_runs']:
 if c['name'] in ['PR Selected Checks','PR · Selected Runtime']:print('CHECK',c['name'],c['conclusion'],c['app']['id'],c['details_url'])
