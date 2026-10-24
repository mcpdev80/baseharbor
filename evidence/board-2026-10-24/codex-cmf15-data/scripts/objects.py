import pathlib,subprocess,json,hashlib,tarfile,io,re
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf15-data');g=['git','--git-dir=/tmp/cmf15-review/objects.git'];h=json.loads((p/'pr-before.txt').read_text())['head']['sha'];b=json.loads((p/'develop-before.txt').read_text())['sha'];old='6a0d36dad2a778ab61a58594489c9fedc88e4c0f';local='1c2f1fc476794b6da7c315082cf155b2d6e680bb'
def git(*a):return subprocess.check_output(g+list(a))
print('PR',h,b,json.loads((p/'pr-before.txt').read_text())['base']['sha']);print('FILES',[(x['filename'],x['status']) for x in json.loads((p/'files.txt').read_text())]);print('CHECKS',(p/'checks.txt').read_text(),(p/'checks.stderr').read_text());print('RUNS',json.loads((p/'runs.txt').read_text())['total_count'],'CHECKCOUNT',json.loads((p/'check-runs.txt').read_text())['total_count']);print('OLD RUNS',json.loads((p/'old-runs.txt').read_text())['total_count']);print('LOG',git('log','-7','--format=%H %aI %cI %s',h).decode());print('OLD ANCESTOR',subprocess.run(g+['merge-base','--is-ancestor',old,h]).returncode)
for label,a,z in [('old-current',old,h),('local-current',local,h),('target-current',b,h)]:
 d=git('diff',a,z);(p/(label+'.diff')).write_bytes(d);print(label,git('diff','--stat',a,z).decode());
 if label=='old-current':print(d.decode())
rows=[]
for label,sha in [('H',h),('old',old),('base',b)]:
 for f in ['.github/workflows/ci.yml','scripts/pr-check-runner.py','scripts/pr-gate-plan.py','scripts/test_pr_gate_plan.py','docs/internal/ci-binding-qualifications.md']:
  d=git('show',sha+':'+f);(p/(label+'-'+f.replace('/','_'))).write_bytes(d);rows.append(dict(label=label,path=f,blob=git('rev-parse',sha+':'+f).decode().strip(),sha256=hashlib.sha256(d).hexdigest()))
(p/'hashes.json').write_text(json.dumps(rows,indent=2));root=pathlib.Path('/tmp/cmf15-review/H');root.mkdir(exist_ok=True)
with tarfile.open(fileobj=io.BytesIO(git('archive',h))) as t:t.extractall(root,filter='data')
print('EVENTS',[(x.get('event'),x.get('created_at'),x.get('before_commit',{}).get('sha'),x.get('after_commit',{}).get('sha')) for x in json.loads((p/'events.txt').read_text()) if x.get('event') in ['head_ref_force_pushed','base_ref_force_pushed','closed','reopened','head_ref_deleted']])
