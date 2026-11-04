"""Read immutable consumer source pins without inspecting CI results."""
import json
import os
import re
import stat

REPOSITORIES = {'console': 'mcpdev80/baseharbor-console',
                'connector': 'mcpdev80/baseharbor-node-connector'}
WORKFLOW = '.github/workflows/release-integration.yml'


def source_origin(role, gate, pin):
    """Validate repository/commit pins and the optional shared source origin."""
    if (role not in REPOSITORIES or not isinstance(pin, dict) or set(pin) not in
            ({'repository', 'commit', 'workflow'}, {'repository', 'commit', 'workflow', 'origin'}) or
            pin.get('repository') != REPOSITORIES[role] or pin.get('workflow') != WORKFLOW or
            not isinstance(pin.get('commit'), str) or not re.fullmatch('[0-9a-f]{40}', pin['commit'])):
        raise ValueError('consumer source pin differs')
    if 'origin' not in pin:
        return pin['repository'], pin['commit'], gate, role
    origin = pin['origin']
    if (gate != 'integration/static/live-console' or not isinstance(origin, dict) or
            set(origin) != {'repository', 'commit', 'gate'} or
            origin['repository'] != REPOSITORIES['connector'] or
            origin['gate'] != 'integration/docker/remote-target' or
            not isinstance(origin['commit'], str) or not re.fullmatch('[0-9a-f]{40}', origin['commit']) or
            (role == 'connector' and origin['commit'] != pin['commit'])):
        raise ValueError('joint consumer source origin differs')
    return origin['repository'], origin['commit'], origin['gate'], 'connector'


def load_source_pins(path, public=False):
    # This file contains private source identities, never authentication tokens.
    # Reject symlinks and group/world access before opening it.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or
                info.st_mode & (0o022 if public else 0o077) or info.st_size > 1024 * 1024):
            raise ValueError('source pin file protection differs')
        with os.fdopen(fd, 'r', closefd=False) as source:
            value = json.load(source)
    finally:
        os.close(fd)
    if (not isinstance(value, dict) or
            value.get('schema') != ('baseharbor.consumer-evidence-pins/v1' if public else 'baseharbor.private-evidence-pins/v1') or
            not isinstance(value.get('gates'), dict)):
        raise ValueError('source pin configuration schema differs')
    return value['gates']

