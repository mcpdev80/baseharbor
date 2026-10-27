# Minimale CI-Receipts

Diese Referenz definiert den Pilot mit genau einem Gate. Produktive Workflows und Pflichtprüfungen bleiben unverändert; lokale Fixtures qualifizieren den Walking Skeleton nicht.

## Receipt und Plan

Das einzige Pilot-Gate ist `public-contracts`, zugeordnet zu `PR · Public Contracts` in `.github/workflows/ci.yml`. Eine kanonische ASCII-JSON-Datei einschließlich Zeilenumbruch muss strikt unter 500 Bytes bleiben. Die sechs Felder sind festgelegt. Gespeichert werden keine Logs, Ausgaben, Signierschlüssel, Attestierungs-Bundles oder Archivdienste.

```json
{"approved":null,"gate":"public-contracts","plan":"<full Planning commit SHA>","run":{"attempt":1,"id":123},"sha":"<full tested integration SHA>","status":"pass"}
```

Der Pfad in baseharbor-plan lautet `evidence/receipts/public-contracts/<run>-<attempt>.json`. Vollständige SHAs mit 40 Zeichen sind erforderlich. `sha` bezeichnet den getesteten PR-Integrationscommit, nicht lediglich den PR-Head. `plan` bezeichnet den unveränderlichen Planning-Commit mit `evidence/receipt-plans/public-contracts.json`:

```json
{
  "gate": "public-contracts",
  "repository": "mcpdev80/baseharbor",
  "workflow_id": 123,
  "head_sha": "<full PR head SHA>",
  "candidate_sha": "<full tested integration SHA>",
  "base_sha": "<full PR base SHA>",
  "task": "tasks/<task-id>.md"
}
```

Der vertrauenswürdige Integrationsverantwortliche erzeugt diesen kleinen Plan in PR 2 aus dem bestehenden vertrauenswürdigen Planner. Niemand kopiert Run-IDs oder Quellhashes zur manuellen Receipt-Erstellung. Die Platzhalter oben sind Dokumentation und keine gültigen Nachweise. Unbekannte Gates und Actions-Reruns werden im ersten Pilot bewusst nicht unterstützt: Einen neuen Run starten. Den aktiven Producer nicht verändern, um den Pilot grün zu bekommen.

## Zwei Skripte

| Einstieg | Eingaben | Ergebnis |
| --- | --- | --- |
| `scripts/receipt_writer.py` | `--planning`, `--plan`, `--run`, `--attempt` | Live-geprüfte Receipt ohne Genehmigung und lokaler Erstellungscommit |
| `scripts/receipt_verifier.py live` | `--planning`, `--revision`, `--receipt`, `--candidate`, `--plan` | Exakte committete Receipt gegen den lebenden GitHub-Nachweis geprüft |
| `scripts/receipt_verifier.py approve` | Gleiche Argumente, authentifizierter Owner-Workflow-Kontext | Neue Live-Prüfung, anschließend einzige Genehmigungsänderung und lokaler Commit |
| `scripts/receipt_verifier.py retained` | Gleiche Argumente, vertrauenswürdiger geschützter Planning-Stand | Offline-Prüfung der genehmigten Historie und des erforderlichen Kandidaten/Plans |

Die Skripte verwenden Python-Standardbibliotheken, Git und GitHub CLI. Der REST-Header lautet `X-GitHub-Api-Version: 2026-03-10`. Git-, API-, Schema- und Identitätsfehler liefern einen Fehlercode. Ausgaben enthalten nur Referenzen. Schreibzugriffe erfordern einen sauberen Planning-Checkout und konfigurierten Git-Autor.

Der aufrufende Workflow muss jeden lokalen Commit per normalem Fast-Forward-Push veröffentlichen und dessen Ergebnis prüfen, bevor er Veröffentlichung meldet oder die nächste Stufe startet. Ein abgelehnter Push ist kein Erfolg: Aktuelle Planning-Historie holen und den begrenzten Vorgang aus einem sauberen Checkout wiederholen; parallele Änderungen erhalten. Identische Writer-Wiederholungen liefern den ursprünglichen Erstellungscommit; widersprüchlicher Inhalt blockiert. PR 1 installiert keinen Publisher-Workflow.

## Live-Nachweis und Owner-Genehmigung

Der Live-Verifier verlangt erwartete numerische Repository-Identität, Workflow, Event, aktuellen Versuch 1, erfolgreichen Gesamt-Run und eindeutig erfolgreiche Gate-/Plan-Jobs. API `head_sha` allein beweist den getesteten Integrationscommit nicht. Deshalb liest der Verifier nur während der Live-Prüfung das bestehende eindeutig benannte Artefakt `trusted-pr-plan`: begrenzte ZIP-Datei, genau eine `pr-plan.json`, kein Entpacken ins Dateisystem, exakte Base-/Head-/Integrationsbindung und ausgewähltes Pilot-Gate. Artefakt und Inhalt werden nicht aufbewahrt. Fehlende, abgelaufene oder mehrdeutige Nachweise verhindern die Genehmigung. Dieser kleine Live-Lesezugriff schließt eine Quellidentitätslücke, die kopierte API-Metadaten und Git-Historie allein nicht schließen.

Die Genehmigung ist eine ausdrückliche Owner-Entscheidung per `workflow_dispatch` auf geschütztem `main`. Vertrauenswürdiger Code prüft `GITHUB_ACTOR` und `GITHUB_TRIGGERING_ACTOR` und wiederholt danach die Live-Prüfung. Die einzige erlaubte Änderung ersetzt `approved:null` durch `by`, UTC `at` und `receipt_commit`, den exakten Erstellungscommit. Ein Workflow darf vom Aufrufer gesetzte Identitätsvariablen nicht als Autorisierung akzeptieren. Fork-/Contributor-Jobs erhalten kein Planning-Schreibtoken; der privilegierte Job führt ausschließlich geschützten Quellcode auf einem sauberen Runner aus.

Nach Genehmigung ruft der Retained-Verifier GitHub nie auf. Der Aufrufer muss `--revision` vom geschützten Planning-Branch beziehen, niemals von einem PR-Branch oder beliebiger lokaler Historie. Er verlangt die unveränderte ursprüngliche Receipt und genau eine Genehmigungsänderung; spätere Änderungen einschließlich zurückgenommener Manipulationen blockieren. Geschützte Planning-Schreiber gelten als vertrauenswürdig: Git-Historie liefert keinen unabhängigen kryptografischen Beweis ihrer Ehrlichkeit; vollständiger Repository-Verlust ist nicht abgesichert.

## Bootstrap und Abnahme

PR 1 enthält nur diese Skripte, gezielte Tests und EN/DE-Dokumentation. Er qualifiziert sich über den bestehenden Legacy-Pfad. Die zielseitige exakte Pfadzuordnung und Test-Erkennung liegt jetzt in `scripts/pr-gate-plan.py`: Die drei Skriptpfade oben wählen den bestehenden Python-Testlauf des Planers aus; die Receipt-Suiten laufen zusätzlich über das `receipt-execution-tests`-Gate, einen vertrauenswürdiger Runner-Profile, der die echten Receipt-Suiten (`test_receipt_writer.py`, `test_receipt_verifier.py`, `test_receipt_chain.py`, `test_receipt_tasklink.py`, `test_ci_workflow_policy_receipts.py`) unter `PYTHONPATH=scripts` ausführt und fehlschlägt, wenn eine Suite nicht tatsächlich läuft. Kein neues Gate, keine neue Matrix und kein ausführbarer Pfad verlässt diese Laufzeiten; Löschungen und Umbenennungen dieser Pfade bleiben ungeklärt, und ungelöste Coverage wird nicht durch eine Ausnahme ersetzt.

PR 2 beweist einen normalen Contributor-PR vollständig: Producer, Writer, veröffentlichte Receipt, Live-Verifier, Owner-Genehmigung, automatischer Task-Link und konsumierende Schattenprüfung des Final-Checks. Der Producer-Workflow kann nicht auf den Post-Run-Abschluss warten, den erst sein Ende auslöst. PR 3 schaltet ausschließlich den akzeptierten Konsumpfad um; danach dürfen weitere Gates folgen.

Die Abnahme verlangt einen echten erfolgreichen GitHub-Pfad und drei abgelehnte Fälle: falsche SHA, tote Run-ID, bearbeitete Receipt. Lokale Tests nutzen echte temporäre Git-Historien und simulierte GitHub-Antworten; sie sind keine Hosted-Qualifikation. Der positive Retained-Fall läuft außerdem bei verbotenem API-Zugriff. Eine vollständige Runtime-Matrix ist für diesen Code nicht erforderlich.

Die Entwicklerübersicht umfasst höchstens drei Zeilen: geprüft, gültig wiederverwendet, nicht erforderlich mit Begründung. Nicht-erforderlich ist eine Planentscheidung und niemals eine PASS-Receipt. Wiederverwendungsauswahl und Workflow-Integration der Übersicht folgen später; dieser PR implementiert keine zweite Evidence-Autorität.
