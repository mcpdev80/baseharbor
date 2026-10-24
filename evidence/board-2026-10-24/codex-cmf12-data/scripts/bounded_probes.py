import importlib.util,pathlib,subprocess,tempfile,json,sys
p=pathlib.Path('/home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-24/codex-cmf12-data')
def load(n,path):
 s=importlib.util.spec_from_file_location(n,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
H=pathlib.Path('/tmp/cmf12-review/H');B=pathlib.Path('/tmp/cmf12-review/base')
sys.path.insert(0,str(H/'scripts'));test=load('test_helpers',H/'scripts/test_pr_gate_plan.py');runner=load('candidate_runner',H/'scripts/pr-check-runner.py')
results=[]
for label,root in [('base',B),('H',H)]:
 planner=load('planner_'+label,root/'scripts/pr-gate-plan.py');resolver=load('resolver_'+label,root/'scripts/pr-diff-base.py')
 with tempfile.TemporaryDirectory(prefix='cmf12-probe-') as td:
  repo=pathlib.Path(td)
  def git(*args):return subprocess.check_output(['git','-C',td,*args],stderr=subprocess.STDOUT).decode().strip()
  git('init','-q','-b','develop');git('config','user.name','Review fixture');git('config','user.email','review@example.invalid')
  f=repo/'docs/internal/ci-binding-qualifications.md';f.parent.mkdir(parents=True);f.write_text((H/'docs/internal/ci-binding-qualifications.md').read_text());(repo/'baseline.txt').write_text('baseline\n');git('add','.');git('commit','-qm','trusted fixture base');base=git('rev-parse','HEAD')
  f=repo/'internal/application/unreviewed_probe.go';f.parent.mkdir(parents=True);f.write_text('package application\n');git('add','.');git('commit','-qm','fixture references docs/internal/ci-binding-qualifications.md');head=git('rev-parse','HEAD')
  tree=git('merge-tree','--write-tree',base,head).splitlines()[0];integration=git('commit-tree',tree,'-p',base,'-p',head,'-m','review fixture integration')
  diff=resolver.resolve_pr_diff(repo,base,head,integration)
  strict=planner.trusted_binding_checks(diff,repo,'mcpdev80/baseharbor','develop');bootstrap=planner.bootstrap_eligible(diff,repo,'mcpdev80/baseharbor','develop');plan=planner.plan(diff,test.REQ,test.MAP,repository=repo,context_repository='mcpdev80/baseharbor',context_target_branch='develop')
  results.append(dict(probe='unknown production path plus message reference',planner=label,paths=diff['changes'],strict_binding=strict,bootstrap_ids=[c['id'] for c in bootstrap] if bootstrap else [],coverage=plan['coverage_status'],unresolved=plan['unresolved_coverage'],reason=plan.get('reasons')))
valid={'schema':'baseharbor.consumer-evidence-pins/v1','gates':{'integration/static/live-console':{'connector':{'repository':'mcpdev80/baseharbor-node-connector','commit':'d'*40},'console':{'repository':'mcpdev80/baseharbor-console','commit':'c'*40}}},'ecosystem':{role:{'repository':r,'commit':c*40,'consumes':deps} for role,r,c,deps in [('core','mcpdev80/baseharbor','a',{}),('demo','mcpdev80/baseharbor-demo','b',{'core':'a'*40}),('console','mcpdev80/baseharbor-console','c',{'core':'a'*40,'demo':'b'*40}),('connector','mcpdev80/baseharbor-node-connector','d',{'core':'a'*40,'demo':'b'*40,'console':'c'*40})]}}
import copy
for label in ['valid','wrong-console-origin','console-repo-as-connector','disconnected-ecosystem']:
 pins=copy.deepcopy(valid)
 if label=='wrong-console-origin':pins['gates']['integration/static/live-console']['console']={'repository':'other/repo','commit':'not-a-sha'}
 if label=='console-repo-as-connector':pins['gates']['integration/static/live-console']['connector']['repository']='mcpdev80/baseharbor-console'
 if label=='disconnected-ecosystem':pins['ecosystem']['connector']['commit']='e'*40;pins['ecosystem']['console']['consumes']['demo']='f'*40
 with tempfile.TemporaryDirectory(prefix='cmf12-pins-') as td:
  path=pathlib.Path(td)/'docs/releases';path.mkdir(parents=True);(path/'v0.4.26.consumer-pins.json').write_text(json.dumps(pins))
  try:out=runner.validate_pins_data('pins-data-validation',td);results.append(dict(probe=label,accepted=True,result=out))
  except Exception as e:results.append(dict(probe=label,accepted=False,error=str(e)))
(p/'bounded-probes.json').write_text(json.dumps(results,indent=2));print(json.dumps(results,indent=2))
