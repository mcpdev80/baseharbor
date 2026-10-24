import subprocess,pathlib,json,hashlib,re
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf6-data');g=['git','--git-dir=/tmp/cmf6-review/objects.git']
def git(*a):return subprocess.check_output(g+list(a))
base=json.loads((p/'develop.json').read_text())['commit']['sha'];q=json.loads((p/'pr1063.json').read_text())['head']['sha'];r='32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2';fetch=subprocess.run(g+['fetch','--no-tags','https://github.com/mcpdev80/baseharbor.git',r],capture_output=True);(p/'r-fetch.txt').write_bytes(fetch.stderr)
paths=['scripts/pr-check-runner.py','scripts/pr-gate-plan.py','scripts/test_pr_gate_plan.py'];doc='docs/internal/ci-binding-qualifications.md'
rec=json.loads(re.findall(r'```json\s*(.*?)```',git('show',q+':'+doc).decode(),re.S)[0]);out={'base':base,'q':q,'r':r,'record':rec,'hashes':{},'inventories':{}}
for label,sha in [('base',base),('q',q),*([('r',r)] if fetch.returncode==0 else [])]+[(str(n),json.loads((p/f'pr{n}.json').read_text())['head']['sha']) for n in [1056,1061,1062]]:
 d=p/'source'/label;d.mkdir(parents=True,exist_ok=True);out['hashes'][label]={}
 for path in paths+[doc,'.github/workflows/ci.yml']:
  raw=git('show',sha+':'+path);(d/path.replace('/','__')).write_bytes(raw);out['hashes'][label][path]={'sha256':hashlib.sha256(raw).hexdigest(),'blob':git('rev-parse',sha+':'+path).decode().strip()}
 out['inventories'][label]=git('diff','--name-status',base,sha).decode()
 (p/(label+'-diff.txt')).write_bytes(git('diff',base,sha,'--',*paths,doc))
for a,b in [('1056','1061'),('1056','1062'),('1061','1062'),('1056','1056')]:
 aa=r if a=='r' else json.loads((p/f'pr{a}.json').read_text())['head']['sha'];bb=json.loads((p/f'pr{b}.json').read_text())['head']['sha'];(p/(a+'-vs-'+b+'.diff')).write_bytes(git('diff',aa,bb,'--',*paths))
(p/'objects.json').write_text(json.dumps(out,indent=2));print(json.dumps(out,indent=2))
