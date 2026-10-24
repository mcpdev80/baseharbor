import pathlib,json,importlib.util,subprocess,os,hashlib
r=pathlib.Path('/tmp/cmx1060-review');s=r/'head/scripts'
spec=importlib.util.spec_from_file_location('reviewed_planner',s/'pr-gate-plan.py');planner=importlib.util.module_from_spec(spec);spec.loader.exec_module(planner)
mapdata=json.loads((s/'pr-gate-map-v1.json').read_text())
req=json.loads((s/'release-requirements.json').read_text())
def diff(path,status='modified'):
 return {'target_base_sha':'a'*40,'head_sha':'b'*40,'merge_base_sha':'c'*40,'integration_sha':'d'*40,'broad_required':False,'reasons':[],'changes':[{'status':status,'path':path}]}
results={'probe_scope':'Bounded deterministic source review; synthetic planner inputs, no native tests or CI execution.'}
for path,status in [('internal/openbao/bootstrap_test.go','modified'),('internal/openbao/unreviewed_security_mutation.go','added'),('internal/openbao/bootstrap.go','deleted'),('internal/unreviewed_security_mutation.go','added')]:
 plan=planner.plan(diff(path,status),req,mapdata,context_repository='mcpdev80/baseharbor',context_target_branch='develop')
 results[path+' '+status]={'coverage_status':plan['coverage_status'],'selected_gates':plan['selected_gates'],'validations':[c['id'] for c in plan['required_validation_checks']],'unresolved':plan['unresolved_coverage']}
workflow=(r/'head/.github/workflows/ci.yml').read_text();start=workflow.index('          case "$VALIDATION" in\n',workflow.index('  pr_validation:') if '  pr_validation:' in workflow else 0);end=workflow.index('\n  pr_public_contracts:',start)
lines=workflow[start:end].rstrip().splitlines();script='\n'.join(line[10:] if line.startswith('          ') else line for line in lines)+'\n'
assert script.rstrip().endswith('esac')
p=subprocess.run(['bash','-c',script],env={**os.environ,'VALIDATION':'openbao-package-tests'},capture_output=True,text=True)
results['actual_validation_case_with_openbao_id']={'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr,'expected_failure':True}
assert p.returncode==2
(r/'validation-case.sh').write_text(script)
obj=r/'objects.git';head=json.loads((r/'manifest-before.json').read_text())['head'];p=subprocess.run(['git','--git-dir',str(obj),'grep','-n','^func TestOpenbao',head,'--','cmd/baha'],text=True,capture_output=True);results['cmd_baha_TestOpenbao_source_search']={'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
(r/'review-probes.json').write_text(json.dumps(results,indent=2));print(json.dumps(results,indent=2))
