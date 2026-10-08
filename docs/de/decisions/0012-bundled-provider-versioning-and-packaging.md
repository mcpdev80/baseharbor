# ADR 0012: Bundled Provider bleiben im Monorepo hinter einer versionierten Provider-Grenze

Status: angenommen

Geburtsdatum: 2026-09-23

## Kontext

BaseHarbor trennt bereits portable Fähigkeiten von konkreten Produkten und definiert die sprachneutrale `baseharbor.provider/v1` Protokoll. Die aktuellen Referenzanbieter werden noch im BaseHarbor-Repository implementiert, da sich ihr Lebenszykluscode aus der ursprünglichen Implementierung Compose-first entwickelte. Kompose ist nun Workload-Source-Kompatibilität, nicht Runtime Provider-Identität.

Ein Repository pro Anbieter zu erstellen, bevor der Provider-Vertrag eingefroren wird, würde die Freigabe, die CI und die Kompatibilität multiplizieren, während sich die Grenze noch ändert.

## Entscheidung

Erstanbieter bleiben bis zum Einfrieren des Anbietervertrags im BaseHarbor monorepo.

Sie werden als unabhängig versionierte Provider-Implementierungen behandelt:

```text
BaseHarbor version
Provider implementation version
Capability specification version
Concrete product version
```

sind separate Versionsachsen.

Bundled Anbieter Entdeckung wird durch exponiert `internal/provider/builtin`. Kerncode sollte gebündelte Anbieter durch diese Grenze lösen, anstatt von konkreten Produktpaketen für Auswahl-Metadaten abhängig zu sein.

Jeder Provider-Integration hat:

- eine stabile Namespaced Provider-ID, zum Beispiel `baseharbor/postgresql`;
- eine Umsetzungsversion;
- eine Version des Anbieterprotokolls;
- explizite Spezifikationen für die versionierte Leistungsfähigkeit;
- unterstützte Platzierung und Lebenszyklus-Semantik.

Die geschützte Laufzeit-Anbieter-Registrierung hält diese Distributions-Identität getrennt von der logischen Fähigkeitsbindung fort.

Konkreter Provider-Lebenszykluscode kann in bestehenden Paketen verbleiben, während er inkrementell migriert wird. Code MUSS NICHT nur verschoben werden, um den Verzeichnisbaum komplett aussehen zu lassen, wenn dadurch ein zweiter Lebenszykluspfad oder eine spekulative Abstraktion entsteht.

Nach dem Einfrieren des Anbietervertrages kann ein Anbieter in sein eigenes Repository verlegt und Zug freigeben, ohne den Vertrag über die Anwendungsfähigkeit zu ändern.

## Folgen

- die derzeitige Entwicklung bleibt ein Repository und als ein Produkt testbar;
- Anbieterversionen können sich unabhängig von BaseHarbor-Releases entwickeln;
- künftige externe Anbieter können die gleichen Spezifikationen für Protokolle und Fähigkeiten umsetzen;
- Die spätere Entnahme von Repositorys ist eher eine Verpackungsentscheidung als eine Migration von Anwendungsaufträgen;
- Anbieter/Produkt-Details bleiben in der Diagnostik sichtbar, werden aber nie zu tragbaren Anwendungsabsichten.
