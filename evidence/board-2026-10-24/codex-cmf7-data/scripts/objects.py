import pathlib,subprocess,json,hashlib
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf7-data');g=['git','--git-dir=/tmp/cmf7-review/objects.git']
def git(*a):return subprocess.check_output(g+list(a))
refs={'main':json.loads((p/'main.txt').read_text())['commit']['sha'],'candidate':json.loads((p/'pr121.txt').read_text())['head']['sha'],'run':json.loads((p/'run.txt').read_text())['head_sha']};out={}
for label,sha in refs.items():
 d=p/'source'/label;d.mkdir(parents=True,exist_ok=True);out[label]={'sha':sha,'tree':git('rev-parse',sha+'^{tree}').decode().strip(),'files':{}}
 for name in ['scripts/failure-runtime-diagnostics.sh','baseharbor-core.ref','.github/workflows/v0424-targeted.yml','scripts/acceptance.sh']:
  raw=git('show',sha+':'+name);(d/name.replace('/','__')).write_bytes(raw);out[label]['files'][name]={'blob':git('rev-parse',sha+':'+name).decode().strip(),'sha256':hashlib.sha256(raw).hexdigest()}
(p/'hashes.json').write_text(json.dumps(out,indent=2));(p/'base-candidate.diff').write_bytes(git('diff',refs['main'],refs['candidate']));(p/'run-candidate.diff').write_bytes(git('diff',refs['run'],refs['candidate']));print(json.dumps(out,indent=2))
