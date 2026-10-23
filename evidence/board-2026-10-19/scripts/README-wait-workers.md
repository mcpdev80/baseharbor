# wait-workers.py

Read-only observer for codeman worker sessions; never starts/sends/removes anything.
Usage: `python3 wait-workers.py <session-id-or-name>...` polls every ~5s and
prints `EVENT <id> <status> <exit_code>` when a session hits idle/exit/blocked or `gone`.
Add `--once` to print a single snapshot (`STATUS <id> <status> <exit_code>`) and exit 0.
No overall timeout by design; this is a status-driven helper only.

## WATCHER mode

`watch` is an opt-in background observer (plain python, `nohup`-style fork, a
PID file and a JSON state file next to this script). It polls `codeman agent
list --json` every ~10s and, on the first transition of a *registered* worker to
`idle`/`exit`/`blocked`/`gone`, sends the coordinator exactly one message
`WATCHER EVENT: <worker-id> <status>`. It is edge-triggered: an unchanged status
(e.g. repeated idle) never re-fires, unzustellbare events and registrations
survive a watcher restart, and the coordinator's own session id is never
triggered. Manage it while running:

```
python3 wait-workers.py watch start <coordinator-session-id> [--poll-interval 10] [--dry-run]
python3 wait-workers.py watch add <worker-id> [--coordinator <id>]
python3 wait-workers.py watch remove <worker-id>
python3 wait-workers.py watch list
python3 wait-workers.py watch stop
```

`--dry-run` logs wake-ups instead of sending (safe to try). `WATCHER_STATE=<path>`
overrides the state-file location (used by tests). Live activation stays with the
coordinator; `watch` never starts/sends-to/removes other sessions.

## fix

The WATCHER poll loop now reloads the state file every iteration (merging fresh
`registered`/`delivered` while keeping its own `watcher` fields), so workers
added after start are monitored and stale `delivered` data can no longer be
overwritten. It also enforces a single instance: a second `watch start` is
refused while one runs, and the loop aborts cleanly if its pid no longer matches
the pidfile. On `watch start`, only a leftover dry-run watcher (cmdline contains
`coord-... --dry-run`) is killed; the live coordinator watcher is preserved.
