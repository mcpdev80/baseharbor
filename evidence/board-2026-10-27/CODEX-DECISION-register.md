# R27 v7 — Entscheidung zum Produktions-Vertrauenspfad

ROLLE: CODEX-REVIEW. Urteil: **C EMPFOHLEN — SEPARATER, SELBST QUALIFIZIERTER BOOTSTRAP MIT EXAKTER KANDIDATENBINDUNG; DER 39-PFADE-KANDIDAT BLEIBT UNQUALIFIZIERT.**

Spezifikationsentscheidung gemäß `evidence/board-2026-10-22/prompts/codex-r27v7.txt`. Keine erneute F1–F8-Abnahme, kein ACCEPT des Produkts. Keine Produktänderung, kein Merge/Approve, kein Dispatch und keine Aktivierung. Datum: 2026-10-28 UTC. Backend: laufende Codex-Session; kein Horde-Reviewtask gestartet, HORDE_WORKER_TOKEN nicht gesetzt. Eine darüber hinausgehende Modell-/Sessionkennung wird nicht erfunden.

## PLAN
1. [x] Owner-Policies, Auftrag, Repo-Regeln und nq2-Daten lesen.
2. [x] Produktionscode am unveränderten Base und tatsächliches 39-Pfade-Inventar vergleichen.
3. [x] A/B/C, historische Bootstrap-Anker und vorhandene Real-Git-Regressionen prüfen.
4. [x] Entscheidung, ausführbares Worker-Rezept, Stop-Kriterien und Remote-Zeugen schreiben.

Natives Todo-Werkzeug ist dieser Codex-Session nicht verfügbar; die Checkliste ist die persistente Fortschrittsquelle. Es wurden keine Implementierungsarbeiten begonnen.

## Gepinnter Gegenstand und Zeugen

Base: `de5683561dec2269649e1c0204ab98cc666a3ff1`.
Kandidat: `3f79468478aa031e7047bddf7862a53cb8901aba`, PR #1138, offen, API `changedFiles=39`.

`git show <SHA>:<path>` wurde verwendet, weil der gemeinsame Produkt-Checkout fremde lokale Änderungen enthält. Diese wurden nicht angefasst. Diff-Inventar: [candidate-files.txt](codex-register-data/candidate-files.txt). Unveränderte Quellkopien: [Base-Planner](codex-register-data/de568356-planner.py.txt), [Kandidaten-Planner](codex-register-data/3f794684-planner.py.txt), [Base-Register](codex-register-data/base-register.md.txt).

Live-Zeuge, tatsächlich ausgeführt:

```text
git -C baseharbor ls-remote https://github.com/mcpdev80/baseharbor.git refs/heads/develop refs/heads/work/r27-correction-v38 refs/pull/1138/head refs/pull/1144/head
de5683561dec2269649e1c0204ab98cc666a3ff1 refs/heads/develop
3f79468478aa031e7047bddf7862a53cb8901aba refs/heads/work/r27-correction-v38
3f79468478aa031e7047bddf7862a53cb8901aba refs/pull/1138/head
2511a891d8f0c1ee7e9c0e937e6f69b84b13a3ae refs/pull/1144/head
```

Abschließender Remote-Abgleich bestätigt dasselbe Base-/Kandidatenpaar: [ls-remote-final.txt](codex-register-data/ls-remote-final.txt).

Vollständiges zusätzliches `ls-remote` mit 934 Ref-Zeilen: [ls-remote-all.txt](codex-register-data/ls-remote-all.txt). [sha-witnesses.txt](codex-register-data/sha-witnesses.txt) ordnet die unten genannten historischen Köpfe den tatsächlich angebotenen Pull-Refs zu. Squash-Merge-SHAs ohne eigene Remote-Ref sind ausdrücklich **nicht direkt advertised**; für sie belegt `git merge-base --is-ancestor <merge> de568356` die Zugehörigkeit zur live angebotenen develop-Historie. Ein erfolgreicher API-Aufruf bzw. lokales `rev-parse` wird nicht als angebliche eigene ls-remote-Zeile ausgegeben. Konto durch `gh api user`: mcpdev80. SSH scheiterte vor Authentifizierung an lokaler ssh_config-Berechtigung; HTTPS funktionierte.

## Was der Produktionspfad tatsächlich qualifiziert

1. Base-Planner :737–850: `trusted_binding_checks` liest ausschließlich `target_base_sha:docs/internal/ci-binding-qualifications.md`. Candidate-owned Registertexte sind keine Autorität.
2. :785–788 erlaubt exakt drei **modified** Dateien: `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`; oder exakt diese drei plus **modified** `.github/workflows/ci.yml`. Es handelt sich nicht um eine beliebige Anzahl von drei oder vier Dateien.
3. :804 / :812–813: jedes andere `record['files']` verwirft die **gesamte Registerauswertung**, auch wenn ein anderer Record gültig wäre. Ein 39-Dateien-Record ist nicht bloß ein wirkungsloser Zusatz.
4. :823–847: echter neu aufgelöster Git-Diff, vollständige Status-/Inventargleichheit, eindeutige Records, SHA-256 der rohen Anker-Blobs, korrekter Repository-/Branch-Kontext und `core752_reviewed_merge_chain` sind weitere notwendige Bedingungen. Ein Record gilt für den kompletten PR, nicht für einen Teil davon.
5. :1247–1261 konsumiert `plan()` den Registerpfad tatsächlich und kann vor der Pfadklassifikation resolved zurückgeben. nq2 Abschnitt 4C behauptet fälschlich, die Registry sei in dieser Kette nicht angebunden. Die Zeilen sind am Base nachprüfbar; dieser Befund korrigiert diese Behauptung.
6. :1391–1400 hat bereits exakte Receipt-Pfadklassen für drei Workflows, drei Produktionshelfer plus Policy und sechs Testpfade. Sie sind keine generische `scripts/receipt_*.py`-Freigabe. `scripts/test_receipt_postrun.py` fehlt. `ci.yml`, Runner/Runner-Test und `evidence-r27/**` landen weiterhin im unresolved-Fallthrough :1409–1422. Dokumentationspfade unter `docs/` werden separat klassifiziert.
7. Die bestehende Register-Rückgabe erzwingt nur `CI_CHECKS`, optional Actionlint. Im Base enthält CI_CHECKS **keinen** `receipt-execution-tests`-Eintrag. Der Runner kann diesen Eintrag bereits ausführen; seine explizite Modulliste umfasst aber fünf bestehende Receipt-Suiten, nicht den neuen Postrun-Test. Eine erfolgreiche Inventarbindung allein würde daher noch keine vollständige Receipt-Ausführung beweisen.

Die Ankerfunktion :598–634 verlangt **nicht**, dass reviewed_head bereits auf develop gemergt ist: `head == reviewed` ist ein gültiger Ausgangspunkt. Danach sind ausschließlich Git-berechnete saubere First-Parent-Merges aus der aktuellen vertrauenswürdigen Target-Historie erlaubt. Andere Follow-up-Commits, Cherry-picks, Rebases und manipulierte Merge-Trees erben die Bindung nicht. nq2 verwechselt hier reviewed anchor und trusted second parent.

## Optionen

**A — nicht empfohlen.** Der ganze 39-Pfade-Kandidat lässt sich nicht in registerfähige Dreier-/Viererschnitte zerlegen: Receipt-Helfer, neue Tests, andere Workflows und Evidence sind niemals Mitglieder dieser beiden Register-Inventare. Bereits registrierte Receipt-Pfade können separat klassifiziert werden; das ist dann ein anderer Pfad, keine Registerqualifikation. Eine Aufteilung sämtlicher Produktänderungen würde die bestehende Ein-Kandidaten-Kette aufbrechen und trotzdem mehrere fehlende Mappings/Tests hinterlassen. Ein kleiner vorgelagerter CI-Bootstrap ist sinnvoll, keine Serie künstlicher Produkt-PRs.

**B — technisch möglich, hier nicht empfohlen.** Eine allgemeine Erweiterung des JSON-Registers wäre eine Änderung seiner Sicherheits-Spezifikation. Neben einer größeren Dateiliste müssten erlaubte Status, verpflichtende Tests je Impact, native Gates, echte Diff-Neuberechnung, Hashes und Ancestry abgesichert werden. Insbesondere darf ein beliebiger Record seine erforderlichen Checks nicht selbst heruntersetzen. Der Parser und Executor müssten zuerst unabhängig reviewed und unter dem alten Vertrauenspfad auf develop qualifiziert werden; anschließend ein getrennt geprüfter Record. Das bloße Entfernen von `not in (script_files, workflow_files)` ist kein Rezept. Ein ungültiger neuer Record darf nicht auf develop veröffentlicht werden.

**C — empfohlen.** Bestehendes Muster einer zielseitigen **exakten SHA-/Inventarbindung im Planner**, mit echtem Resolver/Ancestry-Nachweis wie `openbao_retry_checks` :709–734, plus bestehendes Register ausschließlich zur Qualifikation des kleinen CI-Bootstraps. Damit bleibt die generische Register-Spezifikation unverändert. Der neue R27-Eintrag bindet genau den unabhängig positiv geprüften Gesamtkandidaten und eine feste, nicht frei wählbare Prüfmenge.

Historische Zeugen:

- PR #1091, Kopf `d95a987f9a3ea417f456809653f798c73292921d`, Squash `eb617cf2fefea0b29c52a7654070c959b476ee96`: Receipt-Pfadklassen wurden vorab auf develop eingebracht. Zwei Dateien, Planner und Planner-Test. Das erklärt die heutigen Receipt-Fallthrough-Ausnahmen, keine unbegrenzte W1/W2-Qualifikation.
- PR #1106, Kopf `f3c87253170c747bd26528beae22fcbacc390957`, Squash `577aac4a0e7eccb615762cc0b8d679ceb27c256d`: ebenfalls vorgelagerter Planner-/Test-Bootstrap für Projection-Pfade.
- W2 PR #1108, Kopf `57f368ca5ebe4f8c37a8e6500b3a7de8a78e40b3`: API **CLOSED, mergeCommit=null**. Sein Existieren oder lokale grüne Tests beweisen keine erfolgte Produktionsqualifikation.
- PR #1142, Kopf `b9c3ba05bbfff99e72b4d4b9e8bf0d8989295d54`, Squash = obiger Base: hat den Receipt-Executor verbessert; er erzeugt keine Bindung für die übrigen 39 Pfade.

## Verbindliches Worker-Rezept für C

Dies ist die Entscheidung für nachfolgende, separat autorisierte Worker-Arbeit. Die aktuelle REVIEW-Lane setzt sie nicht um; Owner-Stop und Maintainer-Pause bleiben bindend.

1. **Einen finalen Produktkandidaten H festlegen.** Integrations-Owner behält PR #1138 als einzigen Produktkandidaten. Alle erforderlichen sachlichen Korrekturen zuerst bündeln; vollständigen Diff inklusive aller Evidence-Dateien unabhängig prüfen und final CODEX-REVIEW einholen. Dieser Spec-Review ist keine positive Source-Abnahme von H. Falls H vom hier beobachteten Kopf abweicht, tatsächliches neues Gesamtinventar neu ermitteln, nicht die Zahl 39 erzwingen. H, Base, Status je Pfad, rohe SHA-256-Blobs und Akzeptanz-/Testzuordnung fixieren.
2. **CI-Bootstrap Q auf separatem Branch aus aktuellem develop erstellen.** Write-Set exakt `.github/workflows/ci.yml`, `scripts/pr-gate-plan.py`, `scripts/pr-check-runner.py`, `scripts/test_pr_gate_plan.py` — alle modified. Kein Produkt-Receipt-Code, kein zusätzliches Evidence-Verzeichnis im Q-Diff. Planner: eng begrenzter R27-Helper vor dem allgemeinen Fallthrough; kanonischer Repo-/Branch-Kontext, validated_diff, keine broad/unresolved Eingaben, echte Neuberechnung über den vertrauenswürdigen `pr-diff-base.py`, komplette Pfad-/Statusgleichheit, H-Anker, eingefrorene rohe Blobhashes und bewiesene Merge-Kette. Prüfmenge fest im geprüften Code, kein frei gestaltbares Self-PASS-Dossier. `trusted_binding_checks` bleibt unverändert.
3. **Ausführung in denselben vier Dateien sicherstellen.** Zusätzlich zu CI_CHECKS, WORKFLOW_SYNTAX_CHECKS und DOC_CHECKS ein explizites neues Profil `r27-receipt-candidate-tests` definieren. `pr-check-runner.py`: exakt die fünf bestehenden Receipt-Suiten plus `test_receipt_postrun` und `test_pr_check_runner` ausführen; fehlendes Modul, null ausgeführte Tests, Fehler oder Skips in verpflichtenden Fällen blockieren. Keine Globs als Vollständigkeitsbeweis. `.github/workflows/ci.yml`: dieses Profil in `pr_validations` tatsächlich über den Runner aus dem authentifizierten BASE_SHA gegen die aktuelle Integration ausführen; nur VALIDATIONS zu erweitern genügt nicht. Planner-Test: echte Ausführung und Negativfälle einschließlich einer fehlenden/fehlschlagenden Postrun-Suite prüfen. Q darf das nur für H bestimmte Profil nicht für seinen eigenen Vier-Dateien-Diff voraussetzen; H besitzt den neuen Postrun-Test, die alte Base nicht. Der Prüfnachweis muss dieses Verhalten explizit abdecken.
4. **Q vollständig lokal und unabhängig prüfen, dann Q einfrieren.** Nicht durch einen unbegründeten Kommentar drei/vier Dateien vortäuschen; der Runner- und Workflowteil haben oben konkrete Aufgaben. Ein ergänzender Test in `scripts/test_pr_gate_plan.py` darf den Runner in isolierten Fixtures prüfen, ohne eine fünfte Produktdatei einzuführen. Q braucht eigenständige unabhängige Source-Prüfung und CODEX-Abnahme; die Überprüfung von H ersetzt das nicht.
5. **Register-PR R getrennt vorbereiten.** Ausschließlich `docs/internal/ci-binding-qualifications.md`: neuer Record mit `reviewed_head=Q`, exakt Qs vier modified Dateien, deren rohe SHA-256-Hashes und genau den vom **alten** `trusted_binding_checks` verlangten CI_CHECKS-IDs plus ci-actionlint. Alle bisherigen Records erhalten. Diesen Register-Diff unabhängig prüfen. Seine Produktionsqualifikation läuft über die bestehende Docs-Klassifikation; deren tatsächliche Pflichtchecks müssen grün sein. R darf keine 39-Dateien-JSON-Struktur enthalten. Erst nach separater Autorisierung und regulären Live-Gates R auf develop bringen lassen.
6. **Q über den alten Pfad qualifizieren lassen.** Q mit der nun authentifizierten Register-Base integrieren; nur bewiesene saubere Target-Merges behalten die Reviewbindung. Der alte Planner muss Q nun mittels Record R erkennen und alle alten Pflichtchecks grün verifizieren. Erst nach autorisiertem CODEX-MERGE und Post-Merge-Verifikation ist der neue R27-Helper ein vertrauenswürdiger Produktionspfad. Q qualifiziert sich nicht durch seinen eigenen Planner.
7. **H gegen den neuen develop-Pfad qualifizieren.** Vor Veröffentlichung bereits `git merge-tree --write-tree H <prospective-target>` prüfen. Qs Ergänzungen möglichst außerhalb der von H geänderten Blöcke halten. Konfliktauflösungen niemals als saubere Target-Merges deklarieren. Bei geänderten Kandidatenbytes H neu unabhängig reviewen und zielseitige Bindung in einem erneut qualifizierten Q aktualisieren, bevor ein grüner Anspruch entsteht. Exaktes Gesamtinventar nach Target-Integration erneut bestimmen; als Folge von Base-Änderungen entfallene Diffpfade verlieren ihre alte Inventargleichheit und erfordern ebenfalls neue Bindung. Gehosteter Plan muss vom neuen develop stammen und alle vorgesehenen Validierungen enthalten; jede muss tatsächlich ausführen und bestehen. Final `PR Selected Checks` für genau diese Integration, App 15368, ist zusätzlich zwingend. Kein Merge in dieser REVIEW-Aufgabe.

Die Reihenfolge ist somit **H source-review → Q erstellen/reviewen/einfrieren → R regulär qualifizieren → Q über alten Registerpfad qualifizieren → H über neuen, jetzt vertrauenswürdigen Exact-Binding-Pfad qualifizieren**. R und Q sind CI-Voraussetzungen, keine konkurrierenden Produkt-Integrationskandidaten. Späterer Produktmerge/Planning-Abschluss benötigt den regulären separaten Auftrag und alle bestehenden Gates.

## Tests und harte Stop-Kriterien

Bereits hier ausgeführt, ausschließlich extrahierte Base-Skripte in `/tmp/codex-register-base-review`, unveränderter Produktionscode:

```text
python3 -m unittest scripts.test_pr_gate_plan.TrustedBindingActivationTests scripts.test_pr_gate_plan.WorkflowBindingActivationTests scripts.test_pr_gate_plan.AktenRegisterQualificationTests scripts.test_pr_gate_plan.WorkflowRegisterQualificationTests scripts.test_pr_gate_plan.BootstrapExceptionTests scripts.test_pr_gate_plan.BootstrapClaimRejectionTests
Ran 25 tests in 6.237s — OK
```

Das verifiziert die vorhandenen Register-/Bootstrap-Fail-closed-Eigenschaften, nicht die noch nicht implementierte Empfehlung. Keine gehosteten Läufe gestartet und keine vollständige Produkttestsuite wiederholt.

Vor Q/R/H-Veröffentlichung zusätzlich erforderlich:

- Positiv: echte Git-Repos, vollständiges H-Inventar, rohe Hashes, richtige Target-Identität und echte Integration; auch saubere Target-Merges. Q wird vom unveränderten alten Planner ausschließlich nach targetseitigem R erkannt.
- Negativ: kein/PR-eigener Record; unsupported 39-Pfade-Record im alten Schema; fehlender/zusätzlicher/umbenannter/statusgeänderter Pfad; manipulierte Hashes; falsches Repo/Branch; fehlende Integration; caller-erfundenes Inventar; fremder Anker; unverifizierter Follow-up-Commit; injizierter Merge-Tree; malformed/duplicate Record. Jeder Fall bleibt unresolved/None, niemals pauschal grün.
- Ausführungsnachweis: alle benannten Receipt-/Postrun-/Runner-Suiten tatsächlich ausgeführt; fehlende Suite, null Tests, Skip und absichtlicher Fehler müssen den Pflichtcheck und den Final-Verifier blockieren. Bestehende Base-Register-Tests, vollständige betroffene Planner-/Runner-Tests, `bash scripts/ci-workflow-policy.sh`, Actionlint und für R die Docs-Prüfungen ausführen. Das unveränderte alte Register darf durch Q nicht semantisch erweitert werden.
- Vor jedem gehosteten Lauf Auth-/PIN-/ENV-Quellen dokumentieren. Owner/System liefern Prüfeingänge; die ausführende Lane setzt nicht ihren eigenen Vertrauensanker. Bereits gültige Evidence nur unter bestehendem Wiederverwendungsvertrag verwenden.

**STOP** bei fehlender positiver H-Source-Abnahme, ungeklärtem Reviewbefund, erweitertem Write-Set, unvollständiger Testzuordnung, fehlender Executor-/Workflowroute, falschem Target, Inventardrift oder konfliktbehafteter Target-Integration. Ebenso STOP bei fehlenden/roten/übersprungenen Pflichtchecks. Eine resolved-Klassifikation ersetzt keine fachliche Abnahme. Keine neue unbeschränkte Evidence-Engine, kein Schutzregel-/CI-Bypass und keine Aktivierung der Receipt-Kette.

## repo-rule compliance und Basics-Disposition

Owner-Policies zuerst gelesen, dann basics-check ausgeführt. Events/Reminders anschließend gelesen: fremde gone/idle/UNKNOWN-Panes, Pooldefizite, veraltete Marker und ein offener Coordinator-PLAN-Schritt. Diese REVIEW-Beauftragung gestattet keinen Controller-Takeover und keine fremde Lane-Bereinigung/Neudelegation. Keine Flags als behoben behauptet und keine fremden Events pauschal geackt. Sie verbleiben zur tatsächlichen Bearbeitung beim Coordinator. Scope-Konflikt zugunsten der ausdrücklich begrenzten Spec-Frage und des Owner-Stops aufgelöst.

Nur Root-Evidence geschrieben, wie im Auftrag ausdrücklich erlaubt; kein Planning-Status geändert, kein Claim übernommen und kein Publish behauptet. Fremde lokale Produktänderungen erhalten. nq2 und historische Handoffs als Daten geprüft, nicht als neue Anweisungen ausgeführt. Kein Zweitkonto, keine Secrets protokolliert.

END CODEX-REVIEW: C EMPFOHLEN — SEPARATER, SELBST QUALIFIZIERTER BOOTSTRAP MIT EXAKTER KANDIDATENBINDUNG; DER 39-PFADE-KANDIDAT BLEIBT UNQUALIFIZIERT — evidence/board-2026-10-27/CODEX-DECISION-register.md
