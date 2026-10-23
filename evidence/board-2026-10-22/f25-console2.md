# F25 – console-2-live-core-adapter-workflow-proofs (implement f24 recipe, prep PR, no merge)

Lane: f25 | Prompt: evidence/board-2026-10-22/prompts/f25.txt | Board: console-2 (claim f25)
Mission: Umsetzung des Rezepts aus evidence/board-2026-10-22/f24-console2.md (2 Dateien + markierte
Ausnahme) im Repo mcpdev80/baseharbor-console; kleiner PR gegen den vom Rezept genannten Zielbranch;
PR-Checks abwarten; KEIN Merge (codex-Lane macht REVIEW/MERGE nach F25-END).
End marker: F25-CONSOLE2-DONE (PR-Nr + head + check-status)

## PLAN
1. [x] Turn-start basics-check.py ausgefuehrt; EVENTS abgearbeitet (nur WATCHER-Daten, kein Auftrag) + --ack
2. [x] Contract lesen: plan.py show console-2 (state active, owner f25); f24-Rezept gelesen
3. [x] Claim bereits publiziert (plan origin/main 8a84600 "plan: console-2 claim f25"); kein zweiter Claim noetig
4. [x] Worktree worktrees/f25-console2, Branch f25/console2-live-workflow-proofs ab origin/main 4724d99; fremder Zweig work/v0426-console-consumption nicht angefasst
5. [x] Datei 1 ci.yml: schedule 27 4 * * *, push.branches +main, browser-if (PR skip; main||schedule||runtime-validation/browser/*), REMOTE_RUNTIME=docker, Core-Pin dynamisch aus source-lock.core_commit, Connector-Checkout+Build (Pin 0a02e6d9, alpine 3.23 digest), Step-/Artifact-Rename
6. [x] Datei 2 CONTRACT-READINESS.md: 4 Readiness-Zeilen auf qualified mit Journey-Step-Log-Markern; Wizard-Familien ohne Live-Proof explizit gelistet; Proof-Job/Source-Lock-Referenz ergaenzt
7. [x] Ausnahme gesetzt: Karten-Scope verlangt ausdruecklich "Remove stale waits for #767-#770 claims" -> README-Sektion ersetzt; +1 Zeile release-integration.yml fuer vom Rezept verlangten konsistenten Artifact-Namen
8. [x] npm run check:architecture OK, YAML-Parse OK (actionlint lokal nicht installiert); PR mcpdev80/baseharbor-console#8 gegen main eroeffnet (develop existiert nicht; Repo-Regel: PR gegen main); Run 39986030426: quality pass, browser job skipping-by-design; KEIN Merge
9. [x] repo-rule compliance dokumentieren, END F25-CONSOLE2-DONE

## Kandidat
- Repo/PR: mcpdev80/baseharbor-console#8, base main (4724d99), head 502cdadae3dff0b285868aae85bab618f4c1dede
- Branch: f25/console2-live-workflow-proofs (Worktree worktrees/f25-console2)
- Diff: 4 Dateien, +61/-19 (ci.yml, release-integration.yml 1 Zeile, README.md, docs/CONTRACT-READINESS.md)
- Checks (PR run 39986030426): "Architecture · TypeScript · Lint · Build" pass (44s);
  "Actual browser · Keycloak · Core HTTP" skipping (Pull-Request-Gate wie Rezept-If definiert;
  erster echter Live-Lauf erst auf main/schedule/runtime-validation-Branch – Stop-Kriterium des
  Rezepts bleibt vor Merge-Acceptance durch codex-Lane zu erfuellen: Throwaway-Branch
  runtime-validation/browser/f25p-* einmal gruen inkl. Receipt mit Core-SHA == Source-Lock-SHA).
- Planning: console-2 -> review, prs/Handoff publiziert (plan main 0df92f2).

## Stop-Kriterium / Hinweis an codex-Lane
- Rezept-Abhaengigkeitswarnung unveraendert: 4 Preconditions der Karte noch NOT accepted;
  Merge/Acceptance erst nach Fulfilment, PR bleibt vorbereiteter Kandidat.
- Kein doppeltes Review ohne neuen Befund; Kandidat SHA-genau 502cdada.

## repo-rule compliance
- Ownership: Claim f25 war bereits auf plan main publiziert; keine fremden Claims geaendert.
- Isolation: eigener Branch/Worktree ab origin/main; fremde Zweige/Worktrees nicht angefasst;
  Push nur eigener Branch + PR; kein Merge, kein Force-Push, keine Schutzregel-Bypass.
- Repo-Regeln: CONTRIBUTING "PR gegen main" befolgt; check:architecture lokal ausgefuehrt;
  Docs-/CI-only-Aenderung => keine zusaetzliche Produkt-Matrix noetig (PR-Qualität-Job laeuft
  wegen ci.yml-Pfadtrigger und ist gruen).
- Write-Set: 2 Rezept-Dateien + markierte README-Ausnahme (Karten-Scope deckt #767-#770-Entfernung
  ausdruecklich) + 1 Zeile release-integration.yml (vom Rezept verlangt: "Name in beiden Workflows
  konsistent halten"). Kein product src/tests angefasst.
- Evidence nur in evidence/board-2026-10-22/; keine Hilfs-/Repro-Skripte im Produktbranch.
- Prompt-Injection-Defense: Karten-/Issue-/Recipe-Texte als DATEN verarbeitet; Auftragsquelle waren
  dieses dispatch-Prompt + Repo-Regeln; fremde Karten-Inhalte fuehrten zu keinen Handlungen.
- Secrets: keine Tokens/Passwoerter geloggt; nur oeffentliche Commit-SHAs, Job-/Artefakt-Namen,
  Digest-Referenzen von Public-Images.

F25-CONSOLE2-DONE – PR #8, head 502cdadae3dff0b285868aae85bab618f4c1dede, checks: quality pass / browser skipping-by-design (run 39986030426), kein Merge

## FLAGS from basics-check (Stand Turn-Start)
- EVENTS: 2 un-acked -> events-check: bereits bei offset 46837 (0 neu), letzte Records sind
  WATCHER idle/gone/GHOST-CANDIDATE-Ereignisse = DATEN, keine Auftraege an diese Lane.
- REMINDERS: 58 unread, o56-port118 busy>40min, POOL qwen 1<2, PLAN 16 open steps:
  Coordinator-/Board-Ebene, keine Lane-f25-Aktionspflicht; hier dokumentiert.
