# README-activation.md

## Wann aktivieren - nach #1207-Merge + #1213-Wirkung
- ruleset 25223521 aktiviert: enable-25223521.json (enabled:true)
- Rollback-Test: rollback-25223521.json (enabled:false)
- apply.sh --enable|--rollback: hoelt live GET, merget Flags, PUT mit --match-etag falls verfuegbar, DRYRUN default
- echo was passieren Wuerde ohne es zu tun (DRYRUN default an)

## was die Rehearsal als Engpass fand
- enable-25223521.json: enabled:true, REST identisch zum Live-Ist
- rollback-25223521.json: enabled:false (reverse enable)
- apply.sh: hoelt live GET, merget Flags, PUT mit --match-etag
- DRYRUN default: echo was passieren Wuerde ohne es zu tun
- API returns real values: .enabled=(empty), full JSON body available via gh api
