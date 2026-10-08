# ADR 0019: Organisationspräferenzen und unabhängige Politik

Status: Akzeptiert für die Resolver/Target-Integration; Volle Lebenszyklusakzeptanz anhängig.

## Kontext

Die Distributionsstiftung pins schon Organisation Konfiguration und Aktien
seine Ausgabe zwischen CLI/JSON/MCP/HTTP. Behandlung der Politik als eine weitere Präferenzliste
erlaubt eine spätere Umgebungsliste zu löschen vererbte verbindliche Richtlinie.
Explizite Target-Auswahl kehrte auch vor der Beratung der aktiven Organisation zurück.

## Entscheidung

Erweitern Sie den bestehenden Resolver, ohne eine parallele Konfiguration
System. Deterministische eingebaute < Organisation < Team < Benutzer < Repository <
invocation preference order. Aufrechterhaltung der verbindlichen Politik unabhängig und bewerten
Organisation/Team Einschränkungen gegen die endgültigen Verweise. Emit nicht-geheim
Gewinnen/Überwinden von Quellprovenienz und typisierte umsetzbare Denials.

Das[resolution specification](../spec/organization-resolution-v1.md)definiert
Bereich Zusammenführung, Abwesenheit, Vertrauen, kanonische Verdauungen und Verbrauchergrenzen.
Anfragen können keine verwalteten Geltungsbereiche oder Richtlinien liefern. Firmennamen, Produkt
Auswahlmöglichkeiten, CA-Pfade und Verteilungs-Backends werden von diesem Metamodell nicht eingefroren.

## Normen und Folgen

Bestehende JSON/YAML-Typen, SHA-256-Inhaltsidentität und unveränderliche Quelle verwenden
Auflösung. BaseHarbor definiert nur die minimale oberflächenübergreifende Reichweite/Beweis
und Einschränkungen der Semantik. Tragbare Anforderungen bleiben getrennt.
Es wird kein IAM-Verzeichnis, Legacy-Modus, Migrationsschicht oder breiteres Onboarding hinzugefügt.
Die Prüfungen erstrecken sich auf Präventivgrenzen, unabhängige gegensätzliche Einschränkungen,
obligatorische Politikerhaltung, quellengeheime Ablehnung und Oberflächenparität.
