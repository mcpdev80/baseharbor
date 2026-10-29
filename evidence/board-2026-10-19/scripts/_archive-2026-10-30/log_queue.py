"""Locked, lossless log rotation. Archives remain until explicitly retired.

Cursor identity is (device, inode, complete-line offset), never wall time.
All writers and readers must use the same .lock. Archives are ordered by a
persistent monotonic generation; retaining archives preserves unread records.
"""
import contextlib
import fcntl
import glob
import json
import os

LIMIT = 512 * 1024

@contextlib.contextmanager
def locked(path):
    with open(path + '.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        yield


def append(path, line):
    with locked(path):
        if os.path.exists(path) and os.path.getsize(path) > LIMIT:
            seqfile = path + '.generation'
            try:
                with open(seqfile) as fh:
                    generation = int(fh.read()) + 1
            except (OSError, ValueError):
                generation = 1
            while os.path.exists(path + '.%020d' % generation):
                generation += 1
            with open(seqfile, 'w') as fh:
                fh.write(str(generation))
            os.rename(path, path + '.%020d' % generation)
        with open(path, 'a') as fh:
            fh.write(line)


def snapshot(path, state):
    """Return complete unread lines and cursor; call while holding locked()."""
    paths = sorted(p for p in glob.glob(path + '.*') if p.rsplit('.', 1)[-1].isdigit())
    if os.path.exists(path):
        paths.append(path)
    records = []
    cursor = dict(state)
    identities = [(p, '%s:%s' % (os.stat(p).st_dev, os.stat(p).st_ino)) for p in paths]
    known = state.get('identity')
    start = next((i for i, (_, ident) in enumerate(identities) if ident == known), 0)
    for i, (p, ident) in enumerate(identities):
        if i < start:
            continue
        pos = state.get('pos', 0) if (ident == known or (not known and i == 0)) else 0
        if os.path.getsize(p) < pos:
            pos = 0
        with open(p, 'rb') as fh:
            fh.seek(pos)
            while True:
                raw = fh.readline()
                if not raw or not raw.endswith(b'\n'):
                    break
                records.append(raw.decode('utf-8', 'replace').rstrip('\n'))
                pos += len(raw)
            cursor = {'identity': ident, 'pos': pos}
            # Do not skip a torn write in an archived segment.
            if raw and not raw.endswith(b'\n'):
                break
    return records, cursor


def read(path, ackfile, ack=False):
    with locked(path):
        try:
            with open(ackfile) as fh:
                state = json.load(fh)
        except (OSError, ValueError):
            state = {}
        # Preserve migration from the historical timestamp cursor. Conversion
        # stops at the first unseen/malformed record and never drops new data.
        if "pos" not in state and "last_ts" in state:
            pos = 0
            try:
                with open(path, "rb") as fh:
                    for raw in fh:
                        try:
                            if not raw.endswith(b"\n") or int(raw.split(None, 1)[0]) > state["last_ts"]:
                                break
                        except (IndexError, ValueError):
                            break
                        pos += len(raw)
            except OSError:
                pass
            state = {"pos": pos}
        records, cursor = snapshot(path, state)
        if ack:
            tmp = ackfile + '.tmp'
            with open(tmp, 'w') as fh:
                json.dump(cursor, fh)
            os.replace(tmp, ackfile)
        return records, cursor
