import pathlib,subprocess,json,tarfile,io,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf6-data');g=['git','--git-dir=/tmp/cmf6-review/objects.git'];b=json.loads((p/'develop-after-a.json').read_text())['commit']['sha'];q=json.loads((p/'pr1063.json').read_text())['head']['sha']
summary={}
for n in [1056,1061,1062]:
 sha=json.loads((p/f'pr{n}.json').read_text())['head']['sha'];root=pathlib.Path(f'/tmp/cmf6-review/test{n}');root.mkdir(exist_ok=True)
 data=subprocess.check_output(g+['archive',sha,'scripts']);tarfile.open(fileobj=io.BytesIO(data)).extractall(root,filter='data')
 summary[n]={'sha':sha,'parents':subprocess.check_output(g+['rev-list','--parents','-n','1',sha]).decode().strip(),'diff_if_Q_landed':subprocess.check_output(g+['diff','--name-status',q,sha]).decode()}
def test(n):
 root=pathlib.Path(f'/tmp/cmf6-review/test{n}');r=subprocess.run(['python3','-m','unittest','scripts.test_pr_gate_plan.PinsDataQualificationTests','-v'],cwd=root,capture_output=True,text=True);(p/f'focused{n}.txt').write_text(r.stdout+r.stderr);return n,r.returncode,(r.stdout+r.stderr)[-450:]
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as e:
 for n,code,msg in e.map(test,[1056,1061,1062]):summary[n]['focused_exit']=code;print(n,code,msg)
(p/'comparison.json').write_text(json.dumps(summary,indent=2));print(json.dumps(summary,indent=2))
