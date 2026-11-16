#!/usr/bin/env python3
"""Resolve exact native PR gates from the trusted coverage catalog."""
from __future__ import annotations

import argparse
import importlib.util
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parent


def load_validator():
    spec = importlib.util.spec_from_file_location(
        'pr_gate_coverage_validator', ROOT / 'validate-pr-gate-coverage.py')
    if spec is None or spec.loader is None:
        raise RuntimeError('cannot load trusted PR gate coverage resolver')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def resolve_matrix(changed_paths: list[str], coverage: dict) -> dict:
    validator = load_validator()
    ids = validator.resolve_native_checks(changed_paths, coverage)
    definitions = {row['id']: row for row in coverage.get('native_checks', [])}
    include = []
    for gate_id in ids:
        row = definitions.get(gate_id)
        if row is None or row.get('runtime') not in {'docker', 'podman'}:
            raise ValueError(f'invalid native gate selected by coverage catalog: {gate_id}')
        include.append({'id': gate_id, 'runtime': row['runtime']})
    return {'include': include}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument('--changed-paths-file', type=Path, required=True)
    parser.add_argument('--coverage', type=Path, default=ROOT / 'pr-gate-coverage.json')
    parser.add_argument('--requirements', type=Path, default=ROOT / 'release-requirements.json')
    args = parser.parse_args()
    try:
        coverage = json.loads(args.coverage.read_text())
        requirements = json.loads(args.requirements.read_text())
        changed_paths = args.changed_paths_file.read_text().splitlines()
    except (OSError, json.JSONDecodeError) as exc:
        print(f'invalid native gate plan input: {exc}', file=sys.stderr)
        return 2
    errors = load_validator().validate(coverage, requirements)
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    try:
        print(json.dumps(resolve_matrix(changed_paths, coverage), separators=(',', ':')))
    except (KeyError, ValueError, RuntimeError) as exc:
        print(str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
