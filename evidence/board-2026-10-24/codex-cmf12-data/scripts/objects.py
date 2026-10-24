import pathlib,subprocess,json,hashlib,tarfile,io
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf12-data');g=['git','--git-dir=/tmp/cmf12-review/objects.git'];h='b0abe10526554960b827dd0f4a503a10cebff911';r='32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2';b=json.loads((p/'develop-before.txt').read_text())['sha']
def git(*a):return subprocess.check_output(g+list(a))
print('BASE',b);print('PARENTS',git('rev-list','--parents','-n','1',h).decode());assert git('rev-list','--parents','-n','1',h).decode().split()==[h,r,'65aa063e90767c85af976081c100be1817370e67'];print('ANCESTORS',subprocess.run(g+['merge-base','--is-ancestor',r,h]).returncode,subprocess.run(g+['merge-base','--is-ancestor',b,h]).returncode);print('DIFF',git('diff','--name-status',b,h).decode());print('COMMIT MESSAGE',git('log','-1','--format=%B',h).decode());tree=git('merge-tree','--write-tree',r,'65aa063e90767c85af976081c100be1817370e67').decode().splitlines()[0];assert tree==git('rev-parse',h+'^{tree}').decode().strip();print('CLEAN TREE',tree)
rows=[]
for f in ['scripts/pr-check-runner.py','scripts/pr-gate-plan.py','scripts/test_pr_gate_plan.py','docs/internal/ci-binding-qualifications.md']:
 d=git('show',h+':'+f);rows.append(dict(path=f,head=hashlib.sha256(d).hexdigest(),R=hashlib.sha256(git('show',r+':'+f)).hexdigest(),target=hashlib.sha256(git('show',b+':'+f)).hexdigest()));(p/('H-'+f.replace('/','_'))).write_bytes(d)
(p/'hashes.json').write_text(json.dumps(rows,indent=2));print(json.dumps(rows,indent=2))
for name,sha in [('H',h),('base',b)]:
 root=pathlib.Path('/tmp/cmf12-review')/name;root.mkdir(exist_ok=True)
 with tarfile.open(fileobj=io.BytesIO(git('archive',sha))) as tf:tf.extractall(root,filter='data')
 print('EXTRACTED',root)
pr=json.loads((p/'pr-before.txt').read_text());print('PR',pr['head']['sha'],pr['base']['sha'],pr['state'],pr['merged'],pr['mergeable_state'],'integration',pr['merge_commit_sha']);print('CHECKS',(p/'checks.txt').read_text());print('THREADS',(p/'threads.txt').read_text());print('RUN',json.loads((p/'run.txt').read_text())['head_sha'],json.loads((p/'run.txt').read_text())['conclusion'])
for x in json.loads((p/'check-runs.txt').read_text())['check_runs']:
 if x['name']=='PR Selected Checks':print('REQUIRED',x['id'],x['head_sha'],x['conclusion'],x['app']['id'])
