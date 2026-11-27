# Core-Bediene-API starten

Die Core-Bediene-API ist die authentifizizierte HTTPS-Oberfläche, die `baha serve`
auf dem Rechner startet, dem ein BaseHarbor-Core gehört. Entwicklungspc und
Console erreichen sie über TLS statt über SSH. Diese Seite sagt, welcher Prozess
welche Routen bedient, welche `BASEHARBOR_*`-Variablen die Bediene-API verlangt
oder kennt, und wie ein Client darauf gezeigt wird.

Die begrifflichen Verträge und die vollständigen Routensemantiken bleiben in
[Laufzeit des Control Planes](../spec/control-plane-runtime.md) und
[Protected Machine HTTP v1](../spec/machine-http-v1.md). Dies ist das Start-Runbook;
es dokumentiert manuell nachgeprüfte Pfade und kennzeichnet den Rest.

## Welcher Prozess die Bediene-API bedient

`baha serve` ist ein Befehl mit drei klar getrennten Modi, ausgewählt über
Umgebungsvariablen. Nur die ersten beiden betreffen die Bediene-API.

- Steuerungs-/Bediene-API (dieses Runbook): `BASEHARBOR_RUNTIME_APP_NAME` leer.
- Anwendungseigener Runtime-Broker: `BASEHARBOR_RUNTIME_APP_NAME` gesetzt. Das
  ist eine andere, anwendungsscopeierte Oberfläche und kann die Bediene-API nie
  bereitstellen.
- Verbindungs-Relay / Runtime-Executor: `BASEHARBOR_CONNECTIVITY_RELAY_MODE`
  oder `BASEHARBOR_RUNTIME_EXECUTOR_MODE` auf `true`.

Die öffentliche HTTP-API ist aktiv, wenn `BASEHARBOR_API_OIDC_ISSUER` gesetzt
ist oder mindestens ein `BASEHARBOR_API_OIDC_AUDIENCES`-Eintrag vorliegt. Ohne
sie startet `baha serve` nur die Runtime-Oberfläche; Anfragen unter `/api/`
werden gar nicht bedient.

## Voraussetzungen

Der Server schließt fehl, bevor er lauscht, wenn eine Anforderung fehlt. Alle
Schritte laufen auf dem Core-Rechner.

1. `baha up` ist fertig und `baha status` meldet einen READY-Core (PostgreSQL,
   OpenBao und Keycloak dieser Installation).
2. Das First-Owner-Onboarding lief: `baha login -e dev --init-tenant` (siehe
   [Authentifizierung](../explanation/authentication.md)). Es legt den
   OIDC-Issuer und -Client fest, gegen den die API später prüft.
3. Die echte Nicht-Loopback-Adresse des Core-Rechners ist bekannt (DNS oder
   Clients) und die verwaltete Core-CA kann an Clients übergeben werden.
4. Ein Schlüssel/Zertifikat-Paar mit SANs für diese Adresse, oder der
   Opt-in-Ausstellungspfad unten. Ausstellung und Erneuerung sind hier nicht
   enthalten.

## Pflichtumgebung

Für eine Bediene-API mit eigenen (BYOC) Zertifikatsdateien:

```text
BASEHARBOR_API_TLS_CERT_FILE
BASEHARBOR_API_TLS_KEY_FILE
BASEHARBOR_API_OIDC_ISSUER
BASEHARBOR_API_OIDC_AUDIENCES
```

Zusätzlich verlangt jede aktivierte Bediene-API `BASEHARBOR_API_DATABASE_URL`.
Der DSN muss die Core-Datenbank erreichen; `baha serve` prüft das Migrationsschema
bei jedem Start gegen die ausgelieferten Migrationen und öffnet den Lauscher nur,
wenn Zustand und Schema zusammenpassen. Die Maschinen-API verlangt ein dauerhaftes
Operationsverzeichnis (`BASEHARBOR_RUNTIME_OPERATIONS_DIR`). In der
Managed-Core-Standardsetzung unten ergeben sich beide Werte aus der Installation.

`BASEHARBOR_API_OIDC_AUDIENCES` ist eine Komma-Liste. Das Audience muss den
Operator-OIDC-Client enthalten, den das Onboarding des Cores eingerichtet hat,
und der Issuer muss zum geprüften Operator-Realm passen; sonst wird der Start
abgewiesen.

## Managed Core: Minimalstart

Für eine von `baha` auf diesem Rechner verwaltete Installation werden die
Standardwerte aus dem READY-Zustand abgeleitet; keine Zertifikatsvariablen und
kein wiederholter Issuer sind nötig:

```bash
BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true baha serve
```

`baha serve` wählt dann das eigene Core-Target als Genehmigungsinstanz, bindet
Issuer und Audience aus dem geprüften Realm, schaltet die
Workstation-Registrierung ein, setzt den Lauscher auf die LAN-Adresse des
Rechners Port `18443` und das Maschinen-Operationsverzeichnis unter dem
Target-Zustandsroot. Ein explizites `BASEHARBOR_API_LISTEN_ADDR` behält seine
Autorität. `BASEHARBOR_MANAGED_CORE_TLS=false` deaktiviert nur diese
Standardsetzung.

Soll die vorhandene OpenBao-Dienst-Ausstellung die serving-Zertifikate statt
BYOC-Dateien erzeugen, alle folgenden Variablen setzen; Loopback- oder
Wildcard-Lauscher, abweichende beworbene Häfen und bereits externe
Zertifikatsdateien schließen fehl:

```bash
BASEHARBOR_MANAGED_CORE_TLS=true \
BASEHARBOR_API_LISTEN_ADDR=<lan-ip>:18443 \
BASEHARBOR_API_PUBLIC_URL=https://<lan-ip>:18443 \
BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true \
baha serve
```

Der Core prüft seinen OIDC-Issuer über dieselbe verwaltete
Vertrauensauflösung wie `baha login`: zuerst
`BASEHARBOR_OPERATOR_OIDC_CA_FILE`, dann die vom Bootstrap geschriebene
verwaltete CA, dann der Systembestand. Die verwaltete CA wird nur ergänzt;
die TLS-Prüfung wird nie abgeschaltet.

## Routen

Der Server bindet TLS 1.2 oder neuer, lauscht standardmäßig auf
`127.0.0.1:8443` (`BASEHARBOR_API_LISTEN_ADDR`) und bedient:

| Routengruppe | Bedingung | Zweck |
| --- | --- | --- |
| `GET /healthz` | immer | Prozess-Liveness ohne Authentifizierung |
| `GET /readyz` | Broker-Modus | Abhängigkeits-Reife ohne Authentifizierung |
| `GET /api/v1/management/installation` | Bediene-API | Installationsidentität und Tenant nach Operator-Authentifizierung |
| `GET /api/v1/machine/discovery` | Bediene-API | Operationen und kanonische `http`-Endpoint-Bindings |
| `POST /api/v1/machine/executions`, `GET /api/v1/machine/executions/{execution_id}` | Bediene-API | Ausführung zulassen (`202` ist nur Zulassung) und ablesen |
| `GET /api/v1/machine/executions/{execution_id}/events` (SSE), `POST /api/v1/machine/streams/logs`, `POST /api/v1/machine/streams/exec` | Bediene-API, capability-gesteuert | Ausführungsereignisse, Log- und Exec-Streams |
| `POST/GET/DELETE /api/v1/machine/terminals…` | Bediene-API | interaktive Terminal-Streams |
| `GET/POST/PUT /api/v1/machine/source-transfers…` | Bediene-API | begrenzter Quell-Transfer-Hochlad |
| `GET /api/v1/apps/…` | Bediene-API | geschützte Application-Secret-API |
| `POST /api/v1/connectors/enroll` | `BASEHARBOR_CONNECTOR_ENROLLMENT_ENABLED=true` | einmaliger Connector-Bootstrap-Tausch, einziger anonyme Endpunkt |
| `POST /api/v1/connectors/authorizations`, `/owner-context` (GET), `/renewal-authorizations`, `/status`, `/disconnect-plan`, `/disconnect` | Connector-Registrierung an | Node-Connector-Gewährung und Lifecycle hinter Operator-Authentifizierung |
| `POST /api/v1/workstations/enroll` | `BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true` | löst eine bereits erzeugte Einladung genau einmal ein |
| `POST /api/v1/workstations/bootstrap`, `POST /api/v1/workstations/renew` | Workstation-Registrierung an | Gewährungserzeugung und mTLS-Gerneuerung |
| `GET /metrics` | Runtime-Executor | OpenMetrics-Scrape-Endpunkt für den Ressourcen-Manager des Executors |

Der Runtime-Executor bietet seine eigene Operator-HTTP-Oberfläche auf
`0.0.0.0:9443` (überschreibbar über `BASEHARBOR_EXECUTOR_LISTEN_ADDR`),
nicht auf dem Operator-Listener `127.0.0.1:8443` von Core. Sein `GET /metrics`
gibt OpenMetrics vom Executor-Observer hinter derselben TLS 1.2-oder-neuer- und
Client-Zertifikat-Authentifizierung wie die `/internal/`- und `/healthz`-Routen
des Executors zurück; ein Scrape muss einen verifizierten Executor-Clientzertifikat
präsentieren. Dies ist ein provider-interner Operator-Endpunkt, getrennt
von der obigen Operator-API-Routentabelle.


Jede `/api`-Anfrage durchläuft TLS, Bearer-OIDC-Prüfung, Mitgliedschaft,
Tenant-Kontext, RBAC und Eigentumsprüfung; Klartext-HTTP erreicht keinen
Handler und es gibt keinen Downgrade. Registrierte Workstations authentisieren
sich mit ihrem mTLS-Blatt statt einem Operator-Token. Die Connector-Ausgeh-
Sitzungen sind ein eigener mTLS-Lauscher (`BASEHARBOR_CONNECTOR_LISTEN_ADDR`
mit `BASEHARBOR_CONNECTOR_TLS_{CERT,KEY,CA}_FILE`, Core-SPIFFE-Blatt nötig);
sie gehören nicht zur Bediene-API.

## CLI darauf zeigen

Auf dem Entwicklungspc eine Einladung einlösen (siehe
[Entfernte Workstation](remote-workstation.md)) oder das Core-eigene Remote-
Target mit Verbindungsprofil wie in
[Target-Befehle](https://mcpdev80.github.io/baseharbor/cli/targets/#remote-core-owned-dockerpodman) anlegen:

```bash
baha target create vm-core --runtime-provider docker \
  --access vm-core-management --access-provider baseharbor-core \
  --reference vm-core --tenant-id <tenant-uuid>
BASEHARBOR_CONNECTION_PROFILE=<profile> baha use vm-core
baha --target vm-core status
```

`BASEHARBOR_CONNECTION_PROFILE` hinterlegt die HTTPS-CA und die
Operator-Sitzung muss vor `baha use` gültig sein. Autorität für Quellprüfung,
Zustimmung und Lifecycle bleibt beim Core.

## Verifikation und Grenzen

Die Routen- und Konfigurationsangaben oben stammen aus dem Code auf `develop`
und wurden zweimal gegen einen echten `baha serve`-Prozess geprüft: einmal mit
PostgreSQL auf dem Host und lokaler HTTPS-OIDC-Attrappe, einmal mit PostgreSQL
und OIDC-Anbieter in Containern auf einem Docker-Netz. Beide Läufe nutzten ein
selbstsigniertes Blatt, den ausgelieferten Migrationsstand und gesetztes
`BASEHARBOR_RUNTIME_OPERATIONS_DIR`.

| Anfrage | Ergebnis |
| --- | --- |
| `GET https://<addr>/healthz` | 200 `{"status":"ok"}` |
| `GET https://<addr>/readyz` | 200 `{"status":"ready"}` |
| `GET https://<addr>/api/v1/machine/discovery` | 401 `{"code":"unauthorized","message":"authentication required"}` |
| `GET https://<addr>/api/v1/management/installation` | 401, gleiche Fehlerhülle |
| `GET https://<addr>/api/v1/<unknown>` | 401, Authentifizierung vor Routing |
| `GET http://<addr>/healthz` | 400; es gibt keinen Klartext-Pfad |

Fehlendes Zertifikat oder Schlüssel, fehlender Datenbank-DSN und ein
unerreichbarer Issuer stoppen den Prozess vor jedem Lauscher. Die Prüfung des
Schemas ist exakt: eine Datenbank mit einem anderen als dem ausgelieferten
Migrationsstand wird mit `database has unexpected migration` oder
`database schema is not ready` abgewiesen; die Meldungen stehen in den vorigen
Abschnitten. Eine unbekannte Issuer-Kette wird über
`BASEHARBOR_OPERATOR_OIDC_CA_FILE` aufgelöst, was beide Läufe genutzt haben.
Der Start auf dem Core-Rechner bleibt:

```bash
BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true baha serve
```

Der Weg `baha up` und `baha onboard` bleibt die dokumentierte
Workstation-Anmeldung; die OIDC-/PostgreSQL-Konfiguration hier ist ihre
Serverseite, kein zusätzlicher Schritt.

In diesem Dokumentationslauf nicht erneut ausgeführt und deshalb nur aus
Quelle und verlinkten Spezifikationen belegt: die Managed-Core-Standardsetzung
auf einem physischen Docker-/Podman-Rechner (LAN-Lauscher `:18443`, verwaltete
CA, OpenBao-Ausstellung), Connector-Ausgeh-Sitzungen auf einem echten
Knoten und der Console-Browserfluss. Ein Rechner mit voller Core-Installation
und einer Workstation-Registrierung muss einmal gegen diese Seite geprüft
werden; die Smoke-Karten 0427-168 und 0427-023 bleiben die Tracker.

## Siehe auch

- [Spezifikation Laufzeit des Control Planes](../spec/control-plane-runtime.md)
- [Protected Machine HTTP v1](../spec/machine-http-v1.md)
- [Authentifizierung und API-Fehler](../reference/authentication-and-api-errors.md)
- [Unterstützte Operationen automatisieren](machine-operations.md)
