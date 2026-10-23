"""Read-only review assertions; run after the documented resolver probe."""
import hashlib
import json
from pathlib import Path

from ecosystem_release_pins import validate_ecosystem_graph
from private_consumer_evidence import load_private_pins, source_origin

root = Path(__file__).resolve().parent
pins = json.loads((root / 'candidate.json').read_text())
resolved = json.loads((root / 'resolved.json').read_text())
old = json.loads((root / 'v0423-example.json').read_text())
requirements = json.loads((root / 'requirements.json').read_text())
assert pins == resolved
assert (root / 'candidate.json').read_bytes() == (root / 'resolved.json').read_bytes()
assert pins['schema'] == old['schema']
assert set(pins['gates']) == set(old['gates'])
validate_ecosystem_graph(pins['ecosystem'])
# Resolver deliberately emits mode 0644; a raw snapshot inherits shell umask.
assert load_private_pins(root / 'resolved.json', public=True) == pins['gates']
for gate, roles in pins['gates'].items():
    requirement = next(g for g in requirements['gates'] if g['id'] == gate)
    assert 'v0.4.26' in requirement['required_for']
    assert set(roles) == set(requirement['dependencies']) - {'core', 'demo'}
    for role, pin in roles.items():
        source_origin(role, gate, pin)
    print(gate, 'PASS', requirement['dependencies'])
print('PASS: exact resolver bytes, supported schema, gate roles, DAG, origins, release requirements')
print('candidate sha256:', hashlib.sha256((root / 'candidate.json').read_bytes()).hexdigest())
