# ADR 0002: Anwendungseigene Konfiguration und von BaseHarbor verwaltete Geheimnisse

- Status: angenommen
- Geburtsdatum: 2026-09-08

## Kontext

Die ersten Referenzanwendungen von BaseHarbor sind Agent Workflow Coordinator (AWC), MailFlow und AI-Coding-System (ACS). Alle drei nutzen AI/LLM-Provider, aber Anbieterauswahl und -konfiguration gehören zu den Anwendungen selbst:

- AWC erhält derzeit seine Agent Gateway Konfiguration in erster Linie über Laufzeitkonfiguration/Umgebungsvariablen.
- Mit MailFlow können Benutzer KI-Anbieter und -Endpunkte zur Anwendungslaufzeit konfigurieren.
- ACS ist so konzipiert, dass die Providerkonfiguration über ein eigenes Anwendungs-/Anbietermodell und eine eigene Benutzeroberfläche freigestellt wird.

Wenn BaseHarbor auch der maßgebliche Store für Provider-Auswahl, Endpoint, Modell oder ähnliche Application-Domain-Konfiguration wurde, würde dieselbe Konfiguration an zwei Orten existieren und BaseHarbor würde Eigentümer der Application-Business-Logik werden.

Gleichzeitig sollten Provider-Anmeldeinformationen wie API-Schlüssel nicht als Klartext in normalen Anwendungskonfigurationen oder Datenbanken gespeichert werden müssen, wenn BaseHarbor verfügbar ist.

BaseHarbor muss auch seine No-Lock-In-Eigenschaft bewahren: Die gleiche Anwendung muss ohne BaseHarbor nutzbar bleiben.

## Entscheidung

### 1. Anwendungskonfiguration bleibt anwendungseigener

Konfiguration, die zu der Domäne einer Anwendung gehört, bleibt in dieser Anwendung maßgeblich.

Beispiele hierfür sind:

- Auswahl des Anbieters
- Anbieter-Endpunkt/Basis-URL
- Modellauswahl
- Routing-Regeln
- Anbieterspezifische Optionen

BaseHarbor dupliziert oder überschreibt diese Konfiguration nicht, es sei denn, eine zukünftige Fähigkeit ist explizit als optionale Integration konzipiert.

### 2. Empfindliche Anmeldeinformationen können BaseHarbor-managed sein

Anwendungen können sensible Anmeldeinformationen über BaseHarbors verwaltetes geheimes Flugzeug speichern, das von OpenBao unterstützt wird.

Die Anwendung speichert nur eine stabile logische Geheimreferenz/Handle mit eigener Konfiguration, zum Beispiel konzeptuell:

```text
provider_id = agentgateway-main
endpoint = http://192.168.6.132:4000
model = qwen
credential_ref = baseharbor://secrets/providers/agentgateway-main/api-key
```

Der reale Credential-Wert wird in OpenBao gespeichert und wird in der normalen Konfigurationsdatenbank der Anwendung nie benötigt.

Die genaue URI/Handle Syntax wird durch diesen ADR nicht festgelegt. Der öffentliche Auftrag muss klein, stabil und für die Verwendung durch unabhängige Anwendungen geeignet bleiben.

### 3. Erforderliche Startup-Geheimnisse bleiben unterstützt

Statische/Runtime-Anmeldeinformationen, die zur Bereitstellungszeit bekannt sind, verwenden weiterhin den bestehenden erforderlichen geheimen Vertrag, z. B.:

```yaml
secrets:
  required:
    - name: AGENT_GATEWAY_API_KEY
```

BaseHarbor löst und injiziert diese Werte mit dem normalen Laufzeitvertrag in die Laufzeit der Anwendung.

### 4. Dynamische Anwendungsgeheimnisse sind eine MVP-Fähigkeit

Das MVP muss anwendungsgeschaffene Geheimnisse unterstützen, deren Namen/Identitäten nicht bekannt sind, wenn `baseharbor.yaml` ist geschrieben.

Dies ist für Anwendungen wie MailFlow und ACS erforderlich, bei denen Benutzer später über die Applikationsoberfläche Provider-Anmeldeinformationen hinzufügen können.

Ein dynamisches Geheimnis muss:

- genau auf eine BaseHarbor-Anwendung/Umgebung ausgerichtet sein
- werden in OpenBao oder dem ausgewählten BaseHarbor Secret Provider gespeichert
- nur durch die eigene Anmeldung/Laufzeit-Identifikation und autorisierte Betreiber abrufbar sein
- Unterstützung Erstellen, Lesen/Verwenden, Aktualisieren/Rotieren und Löschen des Lebenszyklus
- nie durchsickern `status`, ` doctor`, Protokolle, API-Fehler oder normale CLI-Ausgabe
- eine stabile Referenz haben, dass die Anwendung neben ihrer eigenen Konfiguration bestehen kann
- Fehler geschlossen, wenn der referenzierte Wert fehlt oder nicht lesbar ist

### 5. Standalone-Anwendungen bleiben erstklassige

BaseHarbor-gemanagte Geheimnisse sind optionale Infrastruktur-Bequemlichkeit, keine obligatorische Laufzeitabhängigkeit.

Eine Anwendung muss weiterhin ohne BaseHarbor funktionieren können, indem sie einen eigenen unterstützten geheimen Mechanismus, Umgebungsvariablen, verschlüsselten Datenbankspeicher oder einen anderen Credential-Anbieter verwendet.

Anträge dürfen nicht Folgendes erfordern:

- `baha login`
- ein BaseHarbor Laufzeit-Token ausschließlich zur Nutzung ihrer normalen Anwendungsfunktionalität
- ein obligatorisches BaseHarbor SDK
- ein proprietäres BaseHarbor-Protokoll für den PostgreSQL-, Redis/Valkey-, HTTP-, OIDC- oder KI-Anbieterverkehr

### 6. BaseHarbor wird kein KI-Anbieter-Register für den MVP

Die Verwendung von KI/LLM allein schafft keine `services.ai` Anforderung in `baseharbor.yaml`.

Für den MVP ist BaseHarbor zuständig für:

- sicheres Zertifikatsspeichern auf Anfrage
- Laufzeit geheime Lieferung
- Konnektivität/Egress des Anwendungsnetzwerks
- Gesundheits-/Diagnostikberichte für BaseHarbor-eigene Infrastruktur

Die Anwendung bleibt für die Entscheidung verantwortlich, welche KI-Anbieter, Endpunkt und Modell sie verwendet.

## Abbildung der Referenzanwendung

### AWÜ

AWC kann bekannte Laufzeit-Anmeldeinformationen wie z.B.`AGENT_GATEWAY_API_KEY` nach Bedarf BaseHarbor Geheimnisse. Endpoint/Provider Verhalten bleibt AWC-Konfiguration.

### MailFlow

MailFlow besitzt Provider-, Endpunkt- und Modellkonfiguration. Provider-API-Schlüssel können als dynamisch erstellte BaseHarbor-gemanagte Geheimnisse gespeichert und aus den Konfigurationsaufzeichnungen von MailFlow referenziert werden.

### KI-Kodierungssystem

ACS besitzt sein Provider-Modell und zukünftige Anbieter-UI. Provider-Anmeldeinformationen können den gleichen dynamischen BaseHarbor-gemanagten geheimen Vertrag verwenden, während ACS nur die entsprechende geheime Referenz speichert.

## Folgen

### Positiv

- Anwendung und BaseHarbor Eigentumsgrenzen bleiben klar
- keine doppelte Wahrheitsquelle für die Provider-Konfiguration
- sensible Anmeldeinformationen können aus normalen Anwendungsdatenbanken entfernt werden
- die gleiche Anwendung kann mit oder ohne BaseHarbor laufen
- das Modell funktioniert für AI-Anbieter und für andere dynamisch konfigurierte Anmeldeinformationen wie SMTP, Webhooks oder externe APIs

### Handelshemmnisse

- Anwendungen, die verwaltete dynamische Geheimnisse wollen, benötigen eine kleine Integrationsgrenze für geheime Referenzen
- BaseHarbor muss einen sicheren Runtime/API-Mechanismus für dynamische geheime Operationen bereitstellen
- secret-reference Kompatibilität und Migration Semantik muss vor v1.0 definiert werden

## Auswirkungen der MVP-Zulassung

Der BaseHarbor MVP ist nicht vollständig, bis mindestens eine Referenzanwendung beide geheimen Pfade unter Beweis stellt:

1. ein erforderliches Geheimnis für die Bereitstellungszeit erfolgreich injiziert wird und
2. ein von der Anwendung zur Laufzeit erstelltes Credential kann in BaseHarbor/OpenBao gespeichert, von der Anwendung referenziert, gedreht und gelöscht werden, ohne den Klartextwert anzugeben.

MailFlow ist die bevorzugte Referenzanwendung für den zweiten Pfad, da die Provider-Konfiguration bereits zur Laufzeit benutzergeführt wird.
