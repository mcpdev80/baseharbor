# Y3 – cmf23 C1/C2 Triage (switch-back provenance / consent-drift)

Mission: ops/missions/y3.txt | ROLLE: DIAGNOSE (read-only Produkt, tmp-ops erlaubt, KEIN Produkt-/Plan-Push)
Datum: 2026-10-27 | Session-Lane: y3

## PLAN

- [x] 0. Evidence-Datei + native Todos angelegt (pwd-Pflicht: /home/marcel/dev/baseharbor-ecosystem)
- [ ] 1. PR #1077 Head beschaffen (fetch refs/pull/1077/head; tmp-Worktree /home/marcel/tmp-ops/wt-y3 oder gh api head.sha)
- [ ] 2. C1 reproduzieren: switch-back stellt Credentials wieder her OHNE Provenance-Check (kleinstes Szenario: go test gefiltert oder repro-Skript in evidence/board-2026-10-26/y3-scripts/)
- [ ] 3. Status auf HEUTIGEM develop pruefen (git log / grep – Pfad bereits anders/behoben?)
- [ ] 4. Bewertung: Fix-Kandidat (klein, Regressionstest) ODER Karte noetig – Empfehlung mit file:line. NICHTS pushen.
- [ ] 5. Abschluss: END Y3-DONE / END Y3-BLOCKED

## Kontext (Review-Befunde, Referenz #1077-Thread, verworfen)

- C1: "switch-back stellt Credentials wieder her OHNE Provenance-Check"
- C2: "credential placement transitions stale-impact-consent drift"
- develop-SHA a39e6bf5 nur in alten Branches, NICHT in develop.

## Befunde

(noch offen)

## Befunde (coord-verified 2026-10-27, y3 stallte 2x auf PLAN -> Coordinator uebernahm)
- origin/pr1077 head = a39e6bf5 (bestaetigt).
- develop vs pr1077 in runtime_environment.go: develop FEHLEN 3 Zeilen ensureNativeBackendCredentialValues-Call; die Funktion selbst existiert NUR in pr1077-Zweigdateien (runtime_environment.go, runtime_native_binding_projection.go). GIT GREP 0 Treffer auf origin/develop.
- KONSEQWENZ: der credential-retention-Pfad aus #1077 wurde via v0.4.26 Owner-Exception NIE in develop集成; cmf23 C1/C2 bleiben auf develop UN adressiert (develop-Code hat den Retention-Zweig schlicht nicht).
- EMPFEHLUNG: Portierung+Provenance-Haertung als eigene v0.4.27-Karte (Design: y4-credential-port-spec.md, Laeuft).
END Y3-DONE (coord-fulfilled)
