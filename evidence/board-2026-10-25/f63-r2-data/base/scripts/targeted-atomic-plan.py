"""Select remaining native atomic diagnostics from the release requirements."""

import argparse
import json
from pathlib import Path


def remaining_plan(requirements, tag):
    if tag not in requirements['releases']:
        raise ValueError('Unsupported release')
    lanes = {'docker': [], 'podman': []}
    for row in requirements['gates']:
        if tag not in row['required_for'] or not row['id'].startswith('atomic/'):
            continue
        family, runtime, gate = row['id'].split('/')
        if family == 'atomic' and runtime in lanes and gate != 'guided':
            lanes[runtime].append({'runtime': runtime, 'gate': gate})
    if not all(lanes.values()):
        raise ValueError('Both native runtime lanes are required')
    # Interleave runtimes so neither takes all initially available runners.
    return [lane[index] for index in range(max(map(len, lanes.values())))
            for lane in lanes.values() if index < len(lane)]


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    args = parser.parse_args()
    requirements = json.loads(Path(__file__).with_name('release-requirements.json').read_text())
    print(json.dumps(remaining_plan(requirements, args.tag), separators=(',', ':')))
