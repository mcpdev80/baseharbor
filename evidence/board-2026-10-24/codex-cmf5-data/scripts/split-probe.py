exec(compile(open('evidence/board-2026-10-24/codex-cmf5-data/scripts/probe.py').read().split('out=[]')[0], 'probe-helpers','exec'))
import re,tempfile,ast,unittest
x=json.loads((p/'core-pr1056.json').read_text());b=x['base']['sha'];old=x['head']['sha'];doc='docs/internal/ci-binding-qualifications.md';index='/tmp/cmf5-review/split-index'
def igit(*a):
 r=subprocess.run(['git','--git-dir='+str(g),*a],capture_output=True,text=True,env={**os.environ,'GIT_INDEX_FILE':index});assert r.returncode==0,r.stderr;return r.stdout.strip()
igit('read-tree',old)
blob=git('rev-parse',b+':'+doc);igit('update-index','--cacheinfo','100644,'+blob+','+doc)
r=git('commit-tree',igit('write-tree'),'-p',old,'-m','REVIEW FIXTURE restore record to target; freeze scripts')
files={f:'modified' for f in ['scripts/pr-check-runner.py','scripts/pr-gate-plan.py','scripts/test_pr_gate_plan.py']}
hashes={f:hashlib.sha256(subprocess.check_output(['git','--git-dir='+str(g),'cat-file','blob',r+':'+f])).hexdigest() for f in files}
record={'repository':'mcpdev80/baseharbor','target_branch':'develop','reviewed_head':r,'files':files,'file_hashes':hashes,'required_checks':['pr-planner-tests','ci-policy','ci-executor-verifier-tests']}
raw='# Core-921 CI binding qualification (#1056)\n\n```json\n'+json.dumps(record,indent=2)+'\n```\n'
blob=subprocess.check_output(['git','--git-dir='+str(g),'hash-object','-w','--stdin'],input=raw.encode()).decode().strip();igit('read-tree',b);igit('update-index','--cacheinfo','100644,'+blob+','+doc)
q=git('commit-tree',igit('write-tree'),'-p',b,'-m','REVIEW FIXTURE documentation only record')
tree=git('merge-tree','--write-tree',r,q).splitlines()[0];h=git('commit-tree',tree,'-p',r,'-p',q,'-m','REVIEW FIXTURE clean target merge')
i=git('commit-tree',git('merge-tree','--write-tree',q,h).splitlines()[0],'-p',q,'-p',h,'-m','REVIEW FIXTURE integration')
m=module(Path('/tmp/cmf5-review/1056/pr-gate-plan.py'),'splitplanner');resolver=module(Path('/tmp/cmf5-review/1056/pr-diff-base.py'),'splitresolver');diff=resolver.resolve_pr_diff(str(g),q,h,i)
checks=m.trusted_binding_checks(diff,str(g),'mcpdev80/baseharbor','develop');assert checks==m.CI_CHECKS
result={'fixture_only_not_remote_commits':True,'R':r,'Q':q,'H':h,'H_parents':git('rev-list','--parents','-n','1',h),'record':record,'diff':diff,'binding_checks':checks,'zero_merge_head_equals_reviewed':m.core752_reviewed_merge_chain(q,r,r,str(g)),'clean_two_parent_merge':m.core752_reviewed_merge_chain(q,h,r,str(g)),'real_head_stale_record_chain':m.core752_reviewed_merge_chain(b,old,'4e737e26af38e7f712f2de1f631633d8d59477b9',str(g))}
(p/'split-probe.json').write_text(json.dumps(result,indent=2));print(json.dumps(result,indent=2))
# Run only the original trusted-binding class, unmodified, with original imports;
# avoid loading unrelated registration test modules in the full suite.
s=(p/'test_pr_gate_plan.py').read_text();tree=ast.parse(s);node=next(n for n in tree.body if isinstance(n,ast.ClassDef) and n.name=='TrustedBindingActivationTests');prefix=s.split('class AppIDFixtureExactQualificationTests')[0];class_source=ast.get_source_segment(s,node)
file=Path('/tmp/cmf5-review/1056/isolated_trusted_tests.py');file.write_text(prefix+class_source+'\n');t=module(file,'isolated_trusted_tests');suite=unittest.defaultTestLoader.loadTestsFromTestCase(t.TrustedBindingActivationTests)
with (p/'focused-tests-final.txt').open('w') as out:
 result=unittest.TextTestRunner(stream=out,verbosity=2).run(suite)
assert result.wasSuccessful()
