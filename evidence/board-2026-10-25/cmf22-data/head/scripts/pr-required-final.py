#!/usr/bin/env python3
"""Fail-closed verification of selected PR gate and validation results."""
import argparse
import json
import re
from pathlib import Path

SHA = re.compile(r'^[0-9a-f]{40}$')
SUCCESS = 'success'


def verify(plan, report, catalog):
    errors = []
    if not isinstance(plan, dict) or plan.get('schema') != 'baseharbor.pr-gate-plan/v1':
        return ['invalid PR gate plan schema']
    if not isinstance(report, dict) or report.get('schema') != 'baseharbor.pr-check-results/v1':
        return ['invalid PR check results schema']
    required = plan.get('selected_gates')
    checks = plan.get('required_validation_checks')
    if not isinstance(required, list) or len(required) != len(set(required)) or not all(isinstance(x, str) for x in required):
        return ['selected gate set missing, empty or malformed']
    if not isinstance(checks, list) or any(not isinstance(c, dict) or not isinstance(c.get('id'), str) for c in checks):
        return ['validation check set malformed']
    if plan.get('coverage_status') != 'resolved':
        errors.append('coverage-unresolved: ' + '; '.join(
            str(x) for x in plan.get('unresolved_coverage', [])
        ) if isinstance(plan.get('unresolved_coverage'), list) and plan.get('unresolved_coverage')
        else 'coverage-unresolved: missing explicit coverage resolution')
    elif plan.get('unresolved_coverage') != []:
        errors.append('resolved coverage has nonempty or missing unresolved reasons')
    if not required and not checks:
        errors.append('no required gates or validations')
    validation = [c['id'] for c in checks]
    if len(validation) != len(set(validation)):
        errors.append('duplicate validation checks')
    eligible = {g['id'] for g in catalog.get('gates', []) if 'v0.4.26' in g.get('required_for', [])}
    if not set(required) <= eligible:
        errors.append('gate not in trusted release inventory')
    if plan.get('release') != 'v0.4.26':
        errors.append('incorrect release')
    for k in ('target_base_sha', 'head_sha', 'merge_base_sha'):
        if not SHA.fullmatch(str(plan.get(k, ''))):
            errors.append('invalid plan ' + k)
    for k in ('target_base_sha', 'head_sha', 'merge_base_sha', 'integration_sha'):
        if report.get(k) != plan.get(k):
            errors.append('mismatched integration input ' + k)
    expected_jobs = ['PR · Focused Python', 'Workflow policy']
    if any(isinstance(gate, str) and gate.startswith(
            ('atomic/docker/', 'atomic/podman/', 'integration/docker/', 'integration/podman/'))
           for gate in required):
        expected_jobs += ['PR · Exact Candidate', 'PR · Runner Interface']
        deps = plan.get('required_consumer_dependencies')
        if not isinstance(deps, list) or not set(deps) <= {'core', 'demo', 'console', 'connector'}:
            errors.append('unqualified gate consumer dependency projection')
        elif set(deps) & {'console', 'connector'}:
            expected_jobs.append('PR · Consumer Pins')
        elif 'demo' in deps:
            expected_jobs.append('PR · Demo Pin')
    if 'integration/static/public-contracts' in required:
        expected_jobs.append('PR · Public Contracts')
    rows = report.get('required_job_conclusions')
    if not isinstance(rows, list):
        errors.append('missing authenticated job prerequisite conclusions')
    else:
        seen_jobs = {}
        for row in rows:
            if not isinstance(row, dict) or not isinstance(row.get('name'), str):
                errors.append('invalid authenticated job prerequisite')
                continue
            name = row['name']
            if name in seen_jobs:
                errors.append('duplicate authenticated prerequisite: ' + name)
            seen_jobs[name] = row.get('conclusion')
        for name in expected_jobs:
            if name not in seen_jobs or seen_jobs[name] != SUCCESS:
                errors.append('required authenticated job not successful: ' + name)
        if set(seen_jobs) != set(expected_jobs):
            errors.append('authenticated job prerequisite set differs from trusted plan')
    entries = report.get('results')
    if not isinstance(entries, list):
        return errors + ['results must be an array']
    index = {}
    for row in entries:
        if not isinstance(row, dict) or not isinstance(row.get('id'), str):
            errors.append('malformed result')
            continue
        key = (row.get('kind'), row['id'])
        if key in index:
            errors.append('duplicate result: ' + str(key))
        index[key] = row
    for kind, ids in (('gate', required), ('validation', validation)):
        for identifier in ids:
            row = index.get((kind, identifier))
            if not row:
                errors.append('missing required ' + kind + ': ' + identifier)
            elif row.get('conclusion') != SUCCESS:
                errors.append('required ' + kind + ' not successful: ' + identifier + ' (' + str(row.get('conclusion')) + ')')
    for (kind, identifier), row in index.items():
        if kind not in ('gate', 'validation'):
            errors.append('unknown result kind: ' + str(kind))
        if kind == 'gate' and identifier not in required and row.get('conclusion') == SUCCESS:
            errors.append('not-required gate incorrectly reported PASS: ' + identifier)
        if kind == 'validation' and identifier not in validation and row.get('conclusion') == SUCCESS:
            errors.append('not-required validation incorrectly reported PASS: ' + identifier)
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--plan', type=Path, required=True)
    parser.add_argument('--results', type=Path, required=True)
    parser.add_argument('--requirements', type=Path, required=True)
    args = parser.parse_args()
    errors = verify(json.loads(args.plan.read_text()), json.loads(args.results.read_text()), json.loads(args.requirements.read_text()))
    for error in errors:
        print('BLOCKED: ' + error)
    if errors:
        raise SystemExit(1)
    print('PR Selected Checks: PASS')


if __name__ == '__main__':
    main()
