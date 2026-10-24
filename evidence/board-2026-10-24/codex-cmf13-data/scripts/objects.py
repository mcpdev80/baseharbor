import pathlib,subprocess,json,hashlib,tarfile,io
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf13-data');g=['git','--git-dir=/tmp/cmf13-review/objects.git'];h=json.loads((p/'pr-before.txt').read_text())['head']['sha'];b=json.loads((p/'develop-before.txt').read_text())['sha'];old='789299a9364750773ac90ccefe9e6d540a2732ff'
def git(*a):return subprocess.check_output(g+list(a))
print('HEAD',h,'develop',b);pr=json.loads((p/'pr-before.txt').read_text());print('PR',pr['base']['sha'],pr['state'],pr['mergeable_state'],pr['mergeable']);print('BASE DRIFT',git('diff','--name-status',b,h).decode());print('FIX DIFF',git('diff','--stat',old,h).decode());print('CHECKS',(p/'checks.txt').read_text());print('1060',json.loads((p/'pr1060.txt').read_text())['head']['sha'],json.loads((p/'pr1060.txt').read_text())['state'],json.loads((p/'pr1060.txt').read_text())['mergeable_state'])
rows=[]
for f in ['internal/openbao/bootstrap.go','internal/openbao/bootstrap_test.go']:
 d=git('show',h+':'+f);rows.append(dict(path=f,blob=git('rev-parse',h+':'+f).decode().strip(),sha256=hashlib.sha256(d).hexdigest()));(p/f.split('/')[-1]).write_bytes(d)
(p/'hashes.json').write_text(json.dumps(rows,indent=2));print(json.dumps(rows,indent=2));(p/'fix.diff').write_bytes(git('diff',old,h));(p/'against-develop.diff').write_bytes(git('diff',b,h));root=pathlib.Path('/tmp/cmf13-review/candidate');root.mkdir(exist_ok=True)
with tarfile.open(fileobj=io.BytesIO(git('archive',h))) as tf:tf.extractall(root,filter='data')
