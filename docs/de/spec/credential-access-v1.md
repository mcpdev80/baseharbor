# Beglaubigungs- und Zugangsrecht v1

## Anwendungsbereich

Diese Spezifikation definiert die normative BaseHarbor Credential Taxonomie und Eigentumsgrenze.

Es klassifiziert das von BaseHarbor verwaltete Material und Identität. Es erstellt kein neues IAM-, RBAC-, Secret-Store- oder Provider-Protokoll.

Die Taxonomie gilt konsequent für Anbieterimplementierungen, Managementflächen, sichere Bindungen, CLI, Maschine JSON, MCP, Status, Arzt und Nachweis.

## Beglaubigungsklassen

### A. Identität des Menschen / des Managements

Klasse A stellt nur den Zugang zum Menschen dar, der auf die Infrastruktur ausgerichtet ist.

Beispiele sind Managed Identity Login, Provider/Management GUIs und BaseHarbor-managed Infrastrukturrollen.

Das Rollenprofil des portablen Infrastrukturmanagements ist:

```text
admin
edit
view
audit
```

Diese Rollen MÜSSEN NICHT zu Anwendungsgeschäftsrollen werden.

### B. Anmeldeinformationen für den Bewerbungsservice

Klasse B stellt technische Referenzen dar, die von einer Anwendung verwendet werden, um eine Fähigkeit zu verbrauchen.

Beispiele dafür sind PostgreSQL, Valkey/cache, RabbitMQ/messaging, MongoDB/documente database, object storage/S3 und technische Anwendungs-Client-Geheimnisse.

Klasse-B-Anmeldeinformationen bleiben für die Anwendung, Umgebung und logische Ressource, wie durch den Fähigkeitsvertrag erforderlich. Sie MÜSSEN NICHT als menschliche Identität behandelt werden.

### C. BaseHarbor-interne Maschinenidentität

Klasse C stellt interne Maschinenidentitäts- und Kontrollanmeldeinformationen dar.

Beispiele sind mTLS private Schlüssel, SPIFFE/Workload-Identitäten, Laufzeit-Broker-Token, OpenBao AppRolles, Provider-Control/Provider-Admin-Anmeldeinformationen und interne Service-to-Service-Anmeldeinformationen.

Klassen-C-Anmeldeinformationen MÜSSEN isoliert bleiben und MUSS NICHT gemeinsam genutzte Human-/Entwickler-Anmeldeinformationen oder Anmeldeinformationen für den Application-Service erben oder wiederverwenden.

## Die geschäftliche Identität der Bewerbung ist nicht anwendbar.

Anwender, Gruppen, Rollen und Berechtigungen von Anwendungen sind Eigentum der Anwendung und ihres ausgewählten Identitätsanbieters.

BaseHarbor MUSS keine parallele Anwendungs-User-Datenbank oder ein proprietäres Business-RBAC-Modell unterhalten.

Anwendungen können normale OIDC, OAuth2, Ansprüche, Gruppen, Kundenrollen oder ein anderes ausgewähltes externes Identitätssystem verwenden.

## Umweltpolitik

### Entwicklung

- Klasse A MAY verwenden eine Ziel-Scope-Entwickler-zugewandte Credential auf kompatiblen BaseHarbor-managed nicht-OIDC-Oberflächen, wo technisch sicher.
- Native OIDC/OAuth2 bleibt bevorzugt, wenn eine Oberfläche sie unterstützt.
- Klasse B MAI verwenden einfache oder gemeinsame Voreinstellungen nur, wenn Anwendung/Umwelt/Ressourcen-Scope und erforderliche Isolation korrekt bleiben.
- Klasse C MUSS isoliert bleiben.

### Prüfung und Produktion

- Klasse A SHOULD verwendet zentral verwaltete Identität/OIDC, wo unterstützt wird.
- Klasse A MUSS Standard für isolierte pro-Oberflächen-Anmeldeinformationen sein, wenn OIDC nicht verfügbar ist.
- Klasse B MUSS Standardeinstellungen für isolierte, am wenigsten privilegierte Anmeldeinformationen sein.
- Explizites unterstütztes Teilen MAY nur dann ausgewählt werden, wenn der Anbieter die erforderliche Autorisierung, Eigentümerschaft und Isolation Semantik bewahren kann.
- Klasse C MUSS isoliert bleiben.

## Eigentumsregeln und Regeln für die gemeinsame Nutzung

Beglaubigungsfreigabe ist eine politische Entscheidung nur innerhalb der gleichen Beglaubigungsklasse.

```text
A Human / management
    != B Application-service
    != C BaseHarbor-internal machine
```

Teilen MUSS KEINE Klassengrenzen kollabieren.

Ein gemeinsames Entwicklungs-Credential MÜSSEN Provider-Kontrolle, Workload-Identität, Laufzeit-Broker, AppRolle oder andere Maschinenanmeldeinformationen NICHT ersetzen.

Gemeinsame Provider-Infrastruktur MÜSSEN KEINE geteilten Klassen-B-Anmeldeinformationen zwischen Anwendungen implizieren.

## Anforderungen des Anbieters

Provider-Implementierungen MUSS diese Taxonomie konsumieren, anstatt Provider-lokale Anmeldeklassen zu erfinden.

Das Verhalten des Anbieters MUSS die Anmeldeklasse, den Eigentümer, den Anwendungsbereich/die Umwelt/den Ressourcenumfang, die Isolationsanforderungen, die maßgebliche Secret-Source-Semantik, die Projektions-/Liefer-Semantik und die Rotations-/Revolutionssemantik beibehalten.

Ein Anbieter MUSS ein nicht unterstütztes oder eingeschränktes Mapping explizit melden, anstatt die angeforderte Sicherheitssemantik stillschweigend zu schwächen.

## Grenzfläche des Managements

Menschliche Authentifizierung für von BaseHarbor verwaltete Managementflächen gehört zur Klasse A.

Provider-Administration-Anmeldeinformationen, die intern von BaseHarbor verwendet werden, gehören zur Klasse C.

Ein provider-natives Username/Passwort, das direkt von einem Menschen als effektives Management-Login verwendet wird, gehört zur Klasse A, auch wenn der Anbieter es in seinem nativen Anmeldesystem speichert.

## Sichere Bindungsgrenze

`secure-binding/v1` Nach wie vor der Arbeitsaufwand/Kapazitätssicherheit und der Vertrag über die Lebenszyklusbindung.

Es enthält anbieterneutrale Referenzen und Sicherheitsmetadaten für Material der Klassen B und Class C, wenn dieses Material an Workload/Capability-Bindung teilnimmt.

Es wird nicht zu einem Humanlogin- oder Business-RBAC-Vertrag.

Plaintext geheime Werte bleiben in portabler Absicht, Anbieter-Register, Plan, Status, Arzt, Beweise und normale Protokolle verboten.

## Maschinenschnittstellenparität

CLI, Machine JSON und MCP MUSS die gleiche Anerkennungs-Klassifikation aufdecken und KEINE Klassen zusammenfügen.

Status, Arzt und Beweismittel MÜSSEN KEINE stärkeren Eigentums-, Isolations- oder Authentifizierungsgarantien geltend machen, als die Implementierung bestätigt hat.

## Versionierung

Dieses Dokument definiert credential/access ownership semantics Version 1.

Additive Evolution MAY hinzufügen Metadaten oder zusätzliche explizite Klassifikationen, ohne bestehende Klassengrenzen zu schwächen.

Eine künftige inkompatible Neuzuteilung von Eigentumssemantik erfordert eine explizite neue Vertragsversion.
