#!/usr/bin/env python3
"""R1 review proof: verify PR1-Zweitblock candidate 83d53e86 mapping + gates."""
import importlib.util, json, os

WT = '/home/marcel/dev/baseharbor-ecosystem/worktrees/l6-ustrack'
ROOT = os.path.join(WT, 'scripts')


def load(name, fname):
    spec = importlib.util.spec_from_file_location(name, os.path.join(ROOT, fname))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


planner = load('planner', 'pr-gate-plan.py')
runner = load('check_runner', 'pr-check-runner.py')
MAP = json.loads(open(os.path.join(ROOT, 'pr-gate-map-v1.json')).read())
GATES = ['integration/static/public-contracts']
REQ = {'releases': ['v0.4.26'],
       'gates': [{'id': g, 'required_for': ['v0.4.26']} for g in GATES]}

CID = {c['id'] for c in planner.CI_CHECKS}
SID = {c['id'] for c in planner.WORKFLOW_SYNTAX_CHECKS}


def ch(p):
    return {'path': p, 'status': 'modified'}


def diff(*ps):
    return {'target_base_sha': 'a' * 40, 'head_sha': 'b' * 40,
            'merge_base_sha': 'c' * 40, 'integration_sha': 'd' * 40,
            'broad_required': False, 'reasons': [], 'changes': [ch(p) for p in ps]}


inv = (sorted(planner.RECEIPT_PILOT_SCRIPTS) + sorted(planner.CI_POLICY_SOURCES)
       + sorted(planner.RECEIPT_DORMANT_WORKFLOWS)
       + ['docs/development/minimal-ci-receipts.md',
          'docs/de/development/minimal-ci-receipts.md',
          'scripts/pr-gate-plan.py', 'scripts/test_pr_gate_plan.py'])
r = planner.plan(diff(*inv), REQ, MAP)
print('paths=', len(r['changes']) if 'changes' in r else 'n/a')
print('coverage_status=', r['coverage_status'])
print('unresolved=', r['unresolved_coverage'])
print('selected_gates=', list(r['selected_gates']))
ids = {c['id'] for c in r['required_validation_checks']}
print('ids<=VALIDATIONS=', ids <= runner.VALIDATIONS)
print('broad_required=', r['broad_required'])

for p in sorted(planner.CI_POLICY_SOURCES):
    rr = planner.plan(diff(p), REQ, MAP)
    got = {c['id'] for c in rr['required_validation_checks']}
    assert got == CID, (p, got)

for p in sorted(planner.RECEIPT_DORMANT_WORKFLOWS):
    rr = planner.plan(diff(p), REQ, MAP)
    got = {c['id'] for c in rr['required_validation_checks']}
    assert got == CID | SID, (p, got)

r2 = planner.plan(diff('.github/workflows/runtime-s3-acceptance.yml'), REQ, MAP)
assert r2['coverage_status'] == 'unresolved', r2

REQUIRED_IDS = ['ci-actionlint', 'ci-executor-verifier-tests', 'ci-policy',
                'documentation-audit', 'mkdocs-de-strict', 'mkdocs-en-strict',
                'pages-candidate-build', 'pr-planner-tests']
for i in REQUIRED_IDS:
    assert i in runner.VALIDATIONS, i

print('ALL-8-VALIDATIONS-OK=True')
print('R1-PROOF-OK')
