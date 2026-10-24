import pathlib,subprocess,importlib.util,json,hashlib,re
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf10-data');repo='/tmp/cmf10-review/objects.git';g=['git','--git-dir='+repo]
spec=importlib.util.spec_from_file_location('trusted_planner',p/'develop-scripts_pr-gate-plan.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
h=json.loads((p/'pr1046-before.txt').read_text())['head']['sha'];b=json.loads((p/'develop-before.txt').read_text())['sha'];r=m.APPID_ENV_ROBUST_TEST_REVIEWED_HEAD
print('current core752_reviewed_merge_chain',m.core752_reviewed_merge_chain(b,h,r,repo));print('zero step current reviewed anchor (mechanism, not changed policy)',m.core752_reviewed_merge_chain(b,h,h,repo))
files=json.loads((p/'files.txt').read_text());print('exact statuses match', {x['filename']:x['status'] for x in files}==m.APPID_ENV_ROBUST_TEST_REVIEWED_FILES)
rows=json.loads((p/'hashes.json').read_text());out=[];tests=[]
for f in files:
 path=f['filename'];a=next(x for x in rows if x['label']=='candidate' and x['path']==path);z=next(x for x in rows if x['label']=='reviewed' and x['path']==path);out.append(dict(path=path,status=f['status'],sha256=a['sha256'],blob=a['blob'],identical_to_reviewed=a['sha256']==z['sha256']))
 data=(p/('candidate-'+path.replace('/','_'))).read_text();tests+=re.findall(r'^func (Test\w+)\(',data,re.M)
(p/'qualification-measurements.json').write_text(json.dumps(dict(current_chain=False,head=h,target=b,old_reviewed_head=r,files=out,test_names=tests),indent=2));print(json.dumps(out,indent=2));print('TEST COUNT',len(tests));print('\n'.join(tests))
# Preserve actual worker logs as data, never run their contents.
wp=pathlib.Path('/tmp/bh-q118-1046');lp=p/'worker-logs';lp.mkdir(exist_ok=True)
for f in wp.glob('*.log'):(lp/f.name).write_bytes(f.read_bytes())
passes=set()
for f in lp.glob('*.log'):passes.update(re.findall(r'^--- PASS: (Test\w+) \(',f.read_text(),re.M))
print('worker top-level tested',len(passes),'missing',set(tests)-passes)
# Confirm same qualifier preserved by #1056's measured objects.
print('1056 anchor present',r in (p/'1056-scripts_pr-gate-plan.py').read_text())
