# OPS-2 Fair scheduling + critical-reminder push
Rule (Owner 2026-10-23): spawn order ornith -> qwen -> flash -> ornith+qwen ...
balance HOST load, not just task speed.
1. spawn-lane.sh admission: when caller gives no explicit model flag, pick the
   model with most headroom vs target (tie-break ornith>qwen>flash); explicit
   flags remain allowed only when the chosen model is at/below target.
2. wait-workers.py: CRITICAL reminders (idlenoend, unmonitored, disk) also emit
   ONE watcher event line with dedup key, so a silent coordinator cannot miss
   them while idle (q44-class failure).
