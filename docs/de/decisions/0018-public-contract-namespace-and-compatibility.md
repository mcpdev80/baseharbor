# ADR 0018: Namespace für öffentliche Aufträge und Kompatibilität

Status: Akzeptiert für die Konvergenz vor dem Einfrieren; das Einfrieren von Verträgen bleibt aus.

## Kontext

Verschiffte JSON-Schema-IDs, die zuvor benannt wurden `schemas.baseharbor.dev`, und Stack
Verwendete Profile `baseharbor.dev/v1`. Historische Beispiele auch genannt ` baseharbor.io`.
DNS- oder HTTP-Erreichbarkeit ist kein Beweis für Projekteigentum. Dieses Projektarchiv
enthält keinen verifizierten Domain-Kontroll-Record für diese Namen.

## Entscheidung

Verwenden Sie den projektgesteuerten GitHub-Repository-URI-Namensraum:

```text
https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/
```

Jede ausgelieferte JSON Schema ID ist das Präfix gefolgt von seiner genauen verpackt
Pfad, einschließlich des Contract-Version-Verzeichnisses. StackProfile `apiVersion` is:

```text
https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/development/v1
```

Diese URIs identifizieren Verträge; sie sind kein Schema-Hosting-Dienst oder ein
unveränderliche Download-Referenz. Verbraucher lösen JSON Schemas durch die
packed offline registry. Ein externes fetch muss eine verifizierte unveränderliche verwenden
Repository Commit und der Pfad der Registry, nie schweigend folgen `HEAD`.
Der Schemadialekt bleibt der Standard JSON Schema 2020-12 URI.

Die eingebettete Registry lehnt nicht-kanonische/duplizierende IDs ab, nicht registriert verschachtelt
IDs und ungelöste oder ausländische Referenzen. Es werden keine Domänen-Aliasen beibehalten.
Logische Protokoll-Identifikatoren wie `baseharbor.machine/v1` bleiben versioniert
Protokollnamen; sie behaupten kein DNS-Eigentum.

## Kompatibilitätsstatus

[COMPATIBILITY.md](https://github.com/mcpdev80/baseharbor/blob/HEAD/COMPATIBILITY.md)
besitzt aktuelle Kompatibilitätspolitik. Versionierte v0.4 Verträge bleiben Entwürfe.
Diese Entscheidung ersetzt nur Vermächtnis-/Migrationserhaltungsversprechen vor dem Einfrieren
in den ADR 0005, 0006, 0008 und 0013; ihre ursprüngliche architektonische Begründung bleibt erhalten
historische Beweise. Es ersetzt nicht die Trennung von tragbaren Absichten,
Anbieter, Lebenszyklus oder Genehmigung.

## Normen und Folgen

Annahme von JSON Schema 2020-12, URI Referenzen und JSON Pointer. Die BaseHarbor
Erweiterung ist das verpackte Identifier-zu-Artefakt-Register und versioniert
StackProfile semantics. Namespace-Änderungen brechen Voreinfrieren-Änderungen:
Hersteller, Verbraucher und Tests müssen gemeinsam vorankommen.
erfordert eine überprüfte Verwaltungskontrolle und eine ausdrückliche neue Entscheidung.
