# F51 - Demo PR #121 Scope-Haerte (Diagnose-Datei-Nachweis)

Mission: ops/missions/f51.txt
Datum: 2026-10-24
Lane/Session: F51 (diese Session)
Repository: baseharbor-demo (git NUR hier; Root-Repo fuer git verboten)

## PLAN

- [x] 0. Evidence-Datei mit ## PLAN + nativen Todos anlegen (vor Analyse)
- [ ] 1. basics-check FLAGS abarbeiten (Events/Reminders lesen, --ack)
- [ ] 2. PR #121 exakt identifizieren: head-SHA, base-SHA, gh pr diff 121 -> Dateiliste (Erwartung: genau baseharbor-core.ref)
- [ ] 3. git -C baseharbor-demo fetch origin; sha256 von scripts/failure-runtime-diagnostics.sh messen an:
      (a) origin/main, (b) PR-Head, (c) Head von Run 39990941852 (headSha via gh run view --json headSha);
      nur `git show <sha>:<pfad> | sha256sum`, kein Checkout
- [ ] 4. Ergebnis-Tabelle (alle SHA/Pfad-Kombis) + Festellung: geaenderte Diagnose-Datei im Kandidaten ja/nein
- [ ] 5. Verzweigung:
      NEIN -> Schluss: Review-Punkt "changed diagnostic behavior" trifft PR #121 nicht, Uebertragung aus PR #120, Beleg mit datei:zeile
      DOCH -> Korrekturbedarf mit exakter Datei/Diff-Stelle (kein Selbst-Fix, andere Lane)
- [ ] 6. Run 39990941852: Jobs/Marker + Einordnung "compatible input" fuer Pin-only-Kandidaten (begruendet)
- [ ] 7. Evidence finalisieren, END-Marker

## Constraints (aus Mission)

- pwd -P vor JEDEM git-Befehl; git NUR in baseharbor-demo
- KEIN Merge, KEIN Review-Kommando, KEIN Push, KEIN CI-Dispatch, KEINE VM/Docker-Laeufe
- Nur lesende Objektzugriffe im neutralen Objekt-Store (git show), keine fremden Worktrees

## Befund

(noch ausstehend)

## repo-rule compliance

(noch ausstehend)
