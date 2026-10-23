#!/usr/bin/env python3
"""Show un-acked watcher events from events.log; --ack marks exactly the
records THIS run observed as processed.

Audit finding 8: the old cursor was wall-clock-seconds based and selected
`ts > last_ts`, so (a) events appended in the same second as an ACK were
permanently hidden, (b) backward clock jumps hid newer records, and (c)
`--ack` re-read and acknowledged everything currently in the log, including
records nobody had processed. The cursor is now an APPEND POSITION (byte
offset): monotonic, immune to wall-clock edits, and --ack covers exactly the
snapshot that was printed. Rotation is detected through file identity and retained archives;
malformed lines are preserved and displayed, never dropped or fatal.
"""
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
LOG = os.environ.get("EVENTS_LOG") or os.path.join(HERE, "events.log")
STATE = os.environ.get("EVENTS_ACK") or os.path.join(HERE, "events-ack.json")


def load_state():
    try:
        with open(STATE) as fh:
            d = json.load(fh)
    except Exception:
        return {}
    if "pos" not in d and "last_ts" in d:
        # Migration: translate the legacy seconds cursor to a byte position
        # (best effort: offset after the last record with ts <= cursor).
        d["pos"] = _legacy_pos(d.get("last_ts", 0))
        d["migrated_ts"] = d.pop("last_ts")
    return d


def _legacy_pos(last_ts):
    pos = 0
    try:
        with open(LOG, "rb") as fh:
            for raw in fh:
                parts = raw.split(None, 1)
                try:
                    if int(parts[0]) <= last_ts:
                        pos += len(raw)
                    else:
                        break
                except (IndexError, ValueError):
                    break
    except OSError:
        return 0
    return pos


def store_state(d):
    tmp = STATE + ".tmp"
    with open(tmp, "w") as fh:
        json.dump(d, fh, sort_keys=True)
        fh.write("\n")
    os.replace(tmp, STATE)


def read_new(pos):
    """(records, end_pos, rotated) - records is [(offset,len,line,ok)] for
    COMPLETE lines only; a torn trailing write stays unread until next pass."""
    rotated = False
    try:
        size = os.path.getsize(LOG)
    except OSError:
        return [], pos, rotated
    if size < pos:
        rotated = True
        pos = 0
    records = []
    with open(LOG, "rb") as fh:
        fh.seek(pos)
        offset = pos
        while True:
            raw = fh.readline()
            if not raw:
                break
            if not raw.endswith(b"\n"):
                break  # torn write at EOF: do not consume or acknowledge
            line = raw.decode("utf-8", "replace").rstrip("\n")
            ok = False
            parts = line.split(" ", 1)
            if len(parts) == 2:
                try:
                    int(parts[0])
                    ok = True
                except ValueError:
                    ok = False
            records.append((offset, len(raw), line, ok))
            offset += len(raw)
        end_pos = offset
    return records, end_pos, rotated


def main(argv):
    import log_queue
    reminders = "--reminders" in argv
    path = (os.environ.get("REMINDERS_LOG") or os.path.join(HERE, "reminders.log")) if reminders else LOG
    ackfile = (os.environ.get("REMINDERS_ACK") or os.path.join(HERE, "reminders-ack.json")) if reminders else STATE
    try:
        with open(ackfile) as fh:
            previous = json.load(fh)
        stat = os.stat(path)
        if stat.st_size < previous.get("pos", 0) or (previous.get("identity") and previous["identity"] != "%s:%s" % (stat.st_dev, stat.st_ino)):
            sys.stderr.write("events-check: LOG ROTATED detected - scanning retained records\n")
    except (OSError, ValueError):
        pass
    records, cursor = log_queue.read(path, ackfile, "--ack" in argv)
    if "--json" in argv:
        print(json.dumps({"unread": len(records), "entries": records, "cursor": cursor}))
    else:
        for line in records:
            try:
                int(line.split(" ", 1)[0])
                print(line)
            except ValueError:
                print("[preserved-malformed] " + line)
    if "--ack" in argv:
        sys.stderr.write("events-check: acked {} record(s) up to offset {}\n".format(len(records), cursor.get("pos", 0)))
    elif not records:
        sys.stderr.write("events-check: no un-acked records\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
