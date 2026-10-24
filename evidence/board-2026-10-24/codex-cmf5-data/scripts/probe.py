from pathlib import Path
import subprocess,json,importlib.util,sys,os,hashlib
p=Path('evidence/board-2026-10-24/codex-cmf5-data').resolve();g=Path('/tmp/cmf5-review/objects.git');os.chdir(g)
def git(*a):
 r=subprocess.run(['git','--git-dir='+str(g),*a],capture_output=True,text=True,env={**os.environ,'GIT_AUTHOR_NAME':'Independent review fixture','GIT_AUTHOR_EMAIL':'review@example.invalid','GIT_COMMITTER_NAME':'Independent review fixture','GIT_COMMITTER_EMAIL':'review@example.invalid'});assert r.returncode==0,r.stderr;return r.stdout.strip()
def module(path,name):
 s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);sys.modules[name]=m;s.loader.exec_module(m);return m
out=[]
for n in [1044,1047,1053,1058,1059,1056,1060]:
 x=json.loads((p/f'core-pr{n}.json').read_text());b=x['base']['sha'];h=x['head']['sha'];d=Path('/tmp/cmf5-review')/str(n);d.mkdir(exist_ok=True)
 for f in ['pr-gate-plan.py','pr-diff-base.py','release-requirements.json','pr-gate-map-v1.json']:(d/f).write_text(git('show',b+':scripts/'+f)+'\n')
 m=module(d/'pr-gate-plan.py','planner'+str(n));r=module(d/'pr-diff-base.py','resolver'+str(n));tree=git('merge-tree','--write-tree',b,h).splitlines()[0];i=git('commit-tree',tree,'-p',b,'-p',h,'-m','bounded neutral review integration')
 diff=r.resolve_pr_diff(str(g),b,h,i)
 result=m.plan(diff,json.loads((d/'release-requirements.json').read_text()),json.loads((d/'pr-gate-map-v1.json').read_text()),repository=str(g),context_repository='mcpdev80/baseharbor',context_target_branch='develop')
 z={'pr':n,'target':b,'head':h,'local_integration_not_CI':i,'diff':diff,'plan':result,'trusted_binding':m.trusted_binding_checks(diff,str(g),'mcpdev80/baseharbor','develop') if hasattr(m,'trusted_binding_checks') else 'absent','bootstrap':m.bootstrap_eligible(diff,str(g),'mcpdev80/baseharbor','develop') if hasattr(m,'bootstrap_eligible') else 'absent'}
 out.append(z);print(n, 'trusted',bool(z['trusted_binding']) if z['trusted_binding']!='absent' else 'absent','bootstrap',bool(z['bootstrap']) if z['bootstrap']!='absent' else 'absent',result['coverage_status'],result.get('reasons'),result.get('unresolved_coverage'))
(p/'probe-results.json').write_text(json.dumps(out,indent=2))
