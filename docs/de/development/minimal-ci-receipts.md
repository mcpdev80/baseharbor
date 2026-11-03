# Minimale CI-Receipts

Die normale CI, Pre-Release und Release führen ihre erforderlichen Tests direkt aus. Fehlgeschlagene, abgebrochene oder fehlende Pflichtjobs blockieren den Workflow über normale Abhängigkeiten und die abschließende Ergebnisprüfung. Die CI lädt keine Gate-Manifeste herunter, bewertet abgeschlossene Actions-Läufe nicht erneut und verwendet keine zusätzliche Evidence- oder Receipt-Freigabekette.

## Commit-Vermerk

Nach einem autorisierten Merge nach `develop` schreibt `receipt-post-run.yml` genau eine kanonische JSON-Datei nach `baseharbor-plan`:

```json
{"commit":"<full merge commit SHA>","pull_request":123}
```

Der Pfad lautet `evidence/receipts/commits/<merge-commit-sha>.json`. Die beiden Felder bilden den vollständigen Vermerk: keine Run-IDs, Versuche, Joblisten, PASS-Daten, Manifeste, Freigaben oder kopierten Testausgaben. Der historische Verzeichnisname führt keine Evidence-Abhängigkeit ein. Eine Receipt protokolliert den Merge; sie ist keine zusätzliche Prüfung oder Release-Genehmigung.

## Veröffentlichung

Nur vertrauenswürdiger Code des Default-Branches erhält das Planning-Schreibtoken. Der Writer authentifiziert den gemergten PR und dessen tatsächlichen Commit in `develop` und veröffentlicht mit einem normalen Fast-Forward-Push. Er fragt niemals CI-Läufe, Jobs oder Gate-Artefakte ab. Zugangsdaten dürfen nicht in Ausgaben erscheinen.

Identische Wiederholungen sind unschädlich. Ein widersprüchlicher Vermerk oder fehlgeschlagener Push lässt die Veröffentlichung fehlschlagen; parallele Planning-Änderungen bleiben erhalten. Ein verpasstes Merge-Ereignis lässt sich über die bestehende Workflow-Eingabe `pr_number` auf `main` nachholen. Dabei läuft derselbe Writer ohne erneute CI-Prüfung.

## Test- und Release-Grenzen

Normale Prüfungen bleiben im Entwicklungsprozess verpflichtend. Das abschließende CI-Ergebnis verwendet die Jobergebnisse des aktuellen Workflows; fehlgeschlagene oder fehlende Pflichtarbeit muss blockieren und ein übersprungener Pflichtcheck darf nicht als Erfolg gelten. Quellidentität, unveränderliche Consumer-Pins, Runtime-Isolation, tatsächliche Testausführung und Bereinigung bleiben erforderlich.

Während der CI-Korrektur bleiben die Rulesets deaktiviert. Eine spätere Aktivierung ist ein eigener Vorgang. Pre-Release-Ausführung, Promotion, Tagging und Veröffentlichung benötigen weiterhin den autorisierten Release-Auftrag; ein Commit-Vermerk genehmigt keinen dieser Schritte.

Ältere Pilot-Receipts bleiben historische Daten. Der frühere Live-/Approve-/Retained-Verifier, Shadow-Consumer, automatische Evidence-Verknüpfungen und die Wiederverwendung von Gate-Manifesten sind abgelöst.
