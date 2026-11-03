# Kandidaten-Vertrag v1

## Umfang

Diese Spezifikation definiert die unveränderliche Grenze der Quell- und
Bild-Einreichung eines BaseHarbor-Kandidaten. Ein Kandidat ist eine **exakte
Einreichung**: eine feste Quellrevision, Bildreferenz oder native Compose/
Quadlet-Einheit, die an ein Target angewandt wird. Er ist kein proprietäres
Pflichtbündel, -katalog, -Archivprotokoll, -Stage-DSL, neues Lebenszyklus-
dienst oder allgemeine Migrations-Engine.

Lese [Lieferung-Provider-Vertrag v1]
(delivery-provider-contract-v1.md) für den Modus/Besitzer/Auswahl/Platzierung-
Vertrag und [Anwendungs-Vertrag v1] (application-contract-v1.md) für die
portable Anwendungsabsicht.

## Unverändliche Einreichung

Die Kandidaten-Einreichung ist exakt und wird nie abgeleitet:

- Eine Quell-Revision ist eine feste `repository` + `revision` (Branch, Tag oder
  Commit-SHA), oder eine abwesende Quelle, wenn nur ein Bild eingereicht wird.
- Eine Bild-Einreichung ist eine feste OCI-Digest (nie ein alleiniger,
  beweglicher Tag) mit einem optionalen image-layout-Pfad.
- Native Compose- und Quadlet-Einheiten werden wie authored als Eingang
  konsumiert; sie werden von BaseHarbor beim Konsum nicht neu kompiliert.
- Eine Git-Revision kann unbekannt oder abwesend sein; OCI-Annotationen allein
  beweisen keine Herkunft und werden nicht an ihre Stelle gesetzt.

Menschliche Release- oder Komponentenversionen mappen auf tatsächliche
Artefaktreferenzen. Eine Versionszeichenkette wird nie als unverändliche
Identität verwendet.

## Anwendungs-/Target-/Consent-Bindung

Der Kandidat bindet eine Anwendung und ein Target durch ein ausdrückliches
Consent:

- `app.*`-Felder binden die Einreichung an eine bestimmte Anwendung.
- `target.*`-Felder binden sie an ein bestimmtes Target.
- `consent.*`-Felder erfassen die Operator-/Automatisierungs-Anerkennung, dass
  eine feste Einreichung angewandt wird.

Die Bindung ist nur deklarative Absicht. Sie kompiliert, befördert oder migriert
alleine nichts.

## Lokale Build- vs. Freigabe-Konsumierung

Lokale Quell-Kompilierung und der Konsum einer CI-lieferten Freigabe sind
getrennt:

- Ein **lokales Build** kompiliert Quelle in Laufzeit-Realisierung. Sie konsumiert
  keine Freigabe.
- Ein **Freigabe-Konsum** wendet ein bestehendes CI-Bild/Revision an. Es
  kompiliert nicht.

Die Freigabe-Konsumierung und -Beförderung kompiliert niemals. Die zwei Pfade sind
ausdrücklich und werden nie zu einer einzelnen mehrdeutigen Operation zusammengefügt.

## Migrationsgrenze

Die Anwendung oder ihr bestehendes Migrationswerkzeug besitzt Schemaänderungen und
Kompatibilitätsnachweise. BaseHarbor prüft nur die deklarierten Voraussetzungen,
die es tatsächlich verifizieren kann.

- Eine Sicherung beweisen kein Schema-Kompatibilität.
- Ein Bild-Rollback impliziert keine Datenbank-Rollback.

BaseHarbor leidet keine Kompatibilität automatisch ab, führt keine Migrationen aus
und liefert keine allgemeine Migrations-Engine. Nicht unterstützte Migrationen
fehlen vor der Mutation.

## Stage-Versionen

Ein Bereitstellungs-Freigabe/Config-Datensatz kann die bestehende Umweltachse um
ein optionales, nicht-normatives Stage-Label für Beobachtung erweitern.
Stage-Informationen wiederverwendet die bestehenden Quell/angewandt/beobachtet
Datensatzfelder; sie führen keine neue normative Stage-Achse oder DSL ein.

## Standards-Grenze

Wiederverwendet bestehende Werkzeuge und Eigentümer. Kein proprietäres Release-,
Registry-, Build-, Pipeline- oder Controller-Framework. Keine Standard- CI- oder
GitOps-Voraussetzung für lokale Arbeit. Runtime-spezifisches nicht unterstütztes
Verhalten bleibt ausdrücklich. Dieser Schnitt ist durch
[Native Lieferung und Stages]
(../decisions/2026-10-18-r27-native-delivery-and-stages.md) beschränkt.

## Fixtures

Ein-/Negative-Fixtures, die malformed-, digest-, Pfad- und secret-reference-
Einreichungen prüfen, liegen unter `contracts/candidate/v1`. Sie erzwingen, dass
eine Kandidaten-Einreichung exakt ist und schlagen bei fehlender oder ungültiger
Identität ohne Downgrade fehl; sie beweisen nie Herkunft, Laufzeit-Gesundheit oder
Freigabe-Nachweis.

<!-- generated:card -->
