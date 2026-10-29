# README-activation.md

## Wann aktivieren - nach #1207-Merge + #1213-Wirkung
- ruleset 25223521 aktiviert: enable-25223521.json (enabled:true)
- Rollback-Test: rollback-25223521.json (enabled:false)
- apply.sh --enable|--rollback: DRYRUN default, hoelt live GET, merget Flags, PUT mit --match-etag falls verfuegbar, sonst etag-Pruefung manuell dokumentiert
- echo was passieren Wuerde ohne es zu tun (DRYRUN default an)

## was die Rehearsal als Engpass fand
- enable-25223521.json: enabled:true, REST identisch zum Live-Ist
- rollback-25223521.json: enabled:false (reverse enable)
- apply.sh: hoelt live GET, merget Flags, PUT mit --match-etag
- DRYRUN default: echo was passieren Wuerde ohne es zu tun
- API 404 BEHAEPTUNG ERSETZT: gh api repos/mcpdev80/baseharbor/rulesets/25223521 --jq .enabled liefert (empty), full JSON body available via gh api

## Rollback-Test ist Teil des Konzepts
- enable-25223521.json: enabled:true, REST identisch zum Live-Ist
- rollback-25223521.json: enabled:false
- apply.sh: hoelt live GET, merget Flags, PUT mit --match-etag
- DRYRUN default: echo was passieren Wuerde ohne es zu tun
