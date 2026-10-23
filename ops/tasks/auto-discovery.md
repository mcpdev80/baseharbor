# OPS-1 Auto-discovery + unmonitored-lane tripwire
Goal: a running lane can never be unwatched again.
1. wait-workers.py: each poll diffs `codeman agent list --json` vs registered;
   any running+unregistered lane is ADDED to observation automatically
   (idle/busy/gone/END detection identical to registered ones).
2. spawn-lane.sh: watcher-registration failure -> non-zero exit AND the failure
   line goes to STDOUT; sid also written to scripts/.last-spawn-<name>.sid.
3. basics-check.py: flag UNMONITORED for running-not-registered (before #1 ships).
Accept: fixtures for diff-based discovery + failure-exit; suite rc=0 twice.
