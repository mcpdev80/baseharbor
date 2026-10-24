# F42 — PR #1056 "PR Selected Checks" lastfail diagnosis

Auftragsquelle: ops/missions/f42.txt (Coordinator / Marcel-Pool)
Scope-Grenze: git NUR im Repo `baseharbor` (ROOT-git verboten). PR #1056 head = 19e7533d.

## PLAN

- [x] 0. Diese Evidence-Datei mit PLAN anlegen + native Todos ([wip:f42]) — ERSTER SCHRITT laut Auftrag
- [x] 1. Actions-Run(s) zu head 19e7533d von PR #1056 abrufen; Meta-Log + Steps des Jobs "PR Selected Checks" lesen
- [x] 2. Hook-Regel in pr-gate-plan.py prüfen: erwartet sie bereits korrekt die Akten-Referenz docs/internal/ci-binding-qualifications.md (aus #1057, noch NICHT in develop)?
- [x] 3. Entscheidung: EXPECTED-FAIL (Merge-Reihenfolge-Zwang) vs. echter Code-Fehler
- [x] 4a. EXPECTED bestätigt: Merge-Reihenfolge dokumentiert (kein Code-Fix, kein Push) — Präzisierung: Akte-PR #1058 muss auf den FINAL-Head gezogen werden, echte Simultan-Merge-Variante scheitert am 3-Pfade-Inventar
- [x] 4b. Nicht zutreffend: kein echter Code-Fehler im Hook; o90-planner-pr unverändert gelassen
- [x] 5. Entscheidung + Status in diese Datei; Abschlussmarker F42-LASTFAIL-DONE + nächster Schritt

## Befund-Rohdaten

Run/Meta-Log (head 19e7533d86a935dc5faefd9969c1b9318e4c3a04):
- Workflow "CI · Source & Quality", run 40064492835, attempt 1, event 2026-10-24T04:59Z.
- Job "PR Selected Checks" (job 120785092463) -> Step 4 "Enforce every required check
  (unqualified runtime remains blocked)" FEHLT als einziger; alle anderen Checks SUCCESS/SKIPPED.
- Fehlerzeilen (Meta-Log):
  `BLOCKED: coverage-unresolved: diff-base requires unresolved coverage: [{"code":
  "integration_sha_mismatch", "message": "integration commit must have exactly target base and
  PR head as its first and second parents, with no extra parents or commits"}]; empty or
  unverified changes cannot exempt checks` + `BLOCKED: no required gates or validations`.
- Plan-Artefakt trusted-pr-plan (artifact 12548357942): coverage_status=unresolved,
  selected_gates=[], target_base_sha=merge_base=1861ed07, integration_sha=72957d23,
  validation_contract: unresolved => final check MUST fail closed.
- Git-Realität: 72957d23 = "Merge 19e7533d into 79e64084" (Parents 79e64084+19e7533d).
  Der PR-Event pinnte base 1861ed07; develop ist inzwischen 79e64084 (#1053 lief nach dem
  letzten synchronize von #1056 ein). GitHub merge-ref gegen NEUES develop regeneriert =>
  first-parent-Regel von pr-diff-base strukturell verletzt => diff ohne Inventar => Hook wird
  nie ausgewertet (früher Return bei broad_required). Das ist designedes fail-closed Verhalten,
  kein Code-Fehler.

Hook-Regel-Prüfung (Kern des Auftrags):
- Head-Version scripts/pr-gate-plan.py (19e7533d, trusted_binding_checks ab Zeile 615,
  core752_reviewed_merge_chain ab 531): liest die Akte korrekt aus dem TARGET-BASE-Blob
  (`git show <target_base>:docs/internal/ci-binding-qualifications.md`), akzeptiert jetzt
  MEHRBLOCK-Register ("may accumulate one entry per reviewed candidate") und verlangt pro
  Eintrag review-ancestry-Kette zum EXAKTEN Kandidaten + Frozen-Blob-Beweis. => Erwartung an
  die Akten-Referenz ist korrekt implementiert.
- Die im Trusted Plan laufende PLANNER-KOPIE kommt aus develop (basisgepinnt, nie PR-Content).
  Develop-Version (Zeilen 592–676) enthält denselben Aktenpfad bereits, toleriert aber nur
  GENAU EIN Register-Block (`if len(blocks) != 1 ... return None`).
- Akten-Status in develop: Datei IST vorhanden (Eintrag reviewed_head=48006064, #1007,
  eingebracht via #1010/#1013). Die Auftragsprämisse "Akte noch nicht in develop" ist überholt.
- #1057 (o102-ci-binding-akte) wurde CLOSED ohne Merge; Nachfolger #1058 (o102-akte-clean) ist
  OPEN und ersetzt den Block durch reviewed_head=4e737e26.
  ACHTUNG STALE: 19e7533d (f40-Hook-Commit) ist ein NORMALER (Einzel-Parent-)Commit AUF
  4e737e26. Der Chain-Walker akzeptiert ab head nur saubere First-Parent-Merges und bricht am
  normalen Commit ab; 4e737e26 ist über 19e7533d niemals per Merge-Kette erreichbar => der
  #1058-Eintrag bindet den aktuellen Head NIE. Zusätzlich mismatchen 2 von 3 registrierten
  Hashes gegen die Blobs von 19e7533d (pr-gate-plan.py 7c2857.. statt 6e5d49..;
  test_pr_gate_plan.py 8564c6.. statt 6d2ea0..; pr-check-runner.py 05f244.. identisch).

## Entscheidung

EXPECTED-FAIL — KEIN Code-Fix, KEIN Push (Auftragsast 4a). Der "PR Selected Checks"-Fail auf
19e7533d ist die vorgesehene fail-closed-Blockade, bis (a) der Branch mit develop synchron ist
und (b) ein den exakten Final-Head bindender Akten-Eintrag IN develop liegt. Der o90-Branch
bleibt unverändert.

Merge-Reihenfolge-Zwang (korrigierte Fassung; "beide simultan" ist NICHT gangbar):
1. #1058 aktualisieren: reviewed_head + file_hashes auf den FINAL-Head von #1056 setzen.
   Exakt EIN JSON-Block behalten (develop-Planner toleriert bis #1056-Merge nur Single-Block).
2. #1058 zuerst in develop mergen (docs-only, leichter Pfad).
3. #1056-Branch per sauberem First-Parent-Merge mit develop synchronisieren (KEIN Rebase/
   Cherry-Pick — der Chain-Walker prüft merge-tree == git merge-tree), EIN push -> neuer
   synchronize-Event mit base=develop-tip -> integration_sha_mismatch entfällt; bei head==
   reviewed_head greift der Ketten-Kurzschluss (loop-erster Check), sonst M -> merge ->
   reviewed-Head-Kette.
4. #1056 mergen. Erst DANCH darf das Register Mehrblock-Einträge (develop-Planner mit
   Head-Hook) annehmen.
Begriffskorrektur zum Auftrag: Nicht "Hook + #1057" fehlt in develop (Hook ist bereits in
#1056 enthalten und ein Akten-Eintrag für #1007 bereits in develop), sondern der CURRENT-HEAD-
bindende Akten-Eintrag (Nachfolger von #1057 = #1058, aktuell stale auf 4e737e26) + Branch-Sync.
Ein Simultan-Merge beider PRs über #1056 (Akte im PR-Diff) würde 4 Paths im Inventar ergeben
und am `changes == 3 Dateien`-Check des Hooks scheitern — Sequenz #1058-first ist zwingend.

Belege (Shas): base 1861ed07 / head 19e7533d (parent 4e737e26) / develop 79e64084 /
merge-ref 72957d23 / #1058-head 9715a722-Zweig o102-akte-clean (Akte reviewed_head 4e737e26).
Logs: /tmp/opencode/f42-job.log (Selected Checks), /tmp/opencode/f42-plan.log (Trusted Plan),
/tmp/opencode/f42-art/pr-plan.json (Plan-Artefakt).
Regelkonformität: git ausschließlich in baseharbor, nur fetch/log/show/diff/cat-file
(read-only); ROOT-git nicht benutzt; keine Produktänderungen; Auftragsdatei als Datenquelle
behandelt; Repos-Regel "unresolved => fail-closed" nicht aufgeweicht.

## Nächster Schritt (Coordinator)

#1058-Eintrag auf den Final-Head von #1056 nachziehen (Single-Block), #1058 mergen, dann
#1056-Branch per erstparent-erhaltenem develop-Merge synchronisieren und EINEN push auslösen;
Run der "PR Selected Checks" gegen resolvedes Coverage verifizieren. #1055 bleibt unabhängiger
Trigger-Härtungstrack.

END F42-LASTFAIL-DONE — Entscheidung: EXPECTED-FAIL, kein Code-Fix; Zwang: Akte-auf-Final-Head
(#1058 refresh) -> develop -> #1056-Branch-Sync -> Merge. Nächster Schritt: #1058-Eintrag auf
19e7533d (bzw. finalen Merge-Head) aktualisieren.
