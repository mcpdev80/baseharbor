# Dokumentationsstil

Die BaseHarbor Dokumentation folgt vor allem einer Regel:

> So wenig wie möglich, so viel wie nötig.

## Eine Seite, ein Zweck

Jede Seite ist in erster Linie eine von:

- Tutorial — lernen, indem sie tun.
- Wie man eine Aufgabe löst.
- Erklärung — ein Konzept verstehen.
- Referenz — genaues Verhalten nachschlagen.
- Spec — normativer Vertrag.
- ADR — Entscheidung und Begründung.
- Freigabe — gelieferte Geschichte.

Mischen Sie diese nicht beiläufig.

## Human Dok.: A5 0031/2003 Verfahren: Konsultation, * Aussprache: 18.11.2003

Verwenden Sie kurze Abschnitte, aktive Stimme und konkrete Beispiele.

Beginnen Sie mit dem, was der Leser tun kann oder wissen muss. Beginnen Sie nicht mit der Implementierungsgeschichte.

Vermeiden Sie:

- vage Architektursprache;
- wiederholte Grundsätze;
- Einzelheiten des Fahrplans;
- Geschichte der Freigabe nach Freigaben;
- Provider Internale, die der Aufgabe nicht helfen.

## Sachgebietsnummer

Bevorzugen Sie Tabellen, Schemas und prägnante Definitionen über Prosa.

Duplizieren Sie die Syntax nicht manuell, die aus Code oder Schema generiert werden kann.

## Technische Daten

Verwenden Sie MUSS/SCHULD/MAY sparsam und nur für echte Kompatibilitäts-, Interoperabilitäts- oder Sicherheitsanforderungen.

Bevorzugen Sie maschinenlesbare Verträge, wenn praktisch.

## Quelle der Wahrheit

- JSON Schema 2020-12 -> Aufbau und Validierung von tragbaren Service-/Anbieterkonfigurationen und -validierung;
- Schema/Typen -> interne Umsetzungsstruktur;
- OpenAPI -> REST;
- Protobuf->-Provider-Prozessprotokoll;
- code/generated help -> CLI-Syntax;
- Spezifik -> normative Semantik;
- ADR -> Begründung;
- GitHub Issues -> Zukunftsplanung;
- Veröffentlichungen -> Geschichte.

Eine detaillierte Tatsache sollte eine maßgebliche Heimat haben.


## Repository-Struktur

Die öffentliche Dokumentation verwendet einen kanonischen Standort pro Zweck:

- `tutorials/`— lernen, indem sie tun;
- `how-to/`— eine Aufgabe zu erfüllen;
- `explanation/`— ein Konzept zu verstehen;
- `reference/`— genaues aktuelles Verhalten;
- `spec/`— normative BaseHarbor Semantik;
- `decisions/`— Architekturentscheidungen und Begründungen;
- `releases/`— Geschichte der menschenlesbaren Freisetzung;
- `internal/`— Wartungsaudits und operative Nachweise.

Nicht hinzufügen Kompatibilität Umleitung Seiten auf der `docs/` root. Aktualisieren Sie stattdessen Links auf die kanonische Seite.

Deutsche Dokumentation hält Mensch-Betrachtung `tutorials/`, ` explanation/`, ` cli/`und ` how-to/`Anleitung mit der gleichen Aufgabe/Domain-Navigation wie Englisch. Unübersetzte Referenz-, Specs-, ADR- und Release-Engineering-Seiten verlinken explizit zu ihrer kanonischen englischen Quelle und sind markiert ` EN`. Befehlsnamen, Flaggen und Maschinenverträge bleiben unverändert. Englisch bleibt maßgeblich für normatives Verhalten.

Freigabeprüfungen sind interne Nachweise und gehören zu `docs/internal/release-audits/`.
