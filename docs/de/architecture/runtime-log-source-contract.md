# Laufzeitneutraler Log-Source-Vertrag

Status: v0.4.17

## Ziel

BaseHarbor stellt unabhängig vom ausgewählten Runtime Provider einen log-source-Vertrag frei.

Portable Application Atent und Core-Verifikation dürfen nicht davon abhängen, ob die Laufzeit Protokolle über Docker syslog, Podman Journald, zukünftige CRI-Protokolldateien oder einen anderen standardkompatiblen Mechanismus transportiert.

## Normalisierte Ursprungsidentität

Eine Logquelle wird semantisch identifiziert durch:

- Anwendung;
- Umwelt;
- Herkunftsklasse;
- Anbieter;
- Dienstleistungen.

Die normalisierten Quellklassen sind:

- `application`
- `application-provider`
- `platform-provider`

Transportdetails wie UDP-Ports, Docker-Logging-Treiber-Optionen, Journalpfade, Containernamen und Laufzeit-spezifische Tags sind Realisierungsdetails und sind keine portablen Anwendungsabsichten.

## Laufzeit-Adapter-Grenze

Laufzeitanbieter implementieren die kleinen `runtime.LogSourceAdapter` Vertrag:

```go
type LogSourceAdapter interface {
    LogCollectionMode() LogCollectionMode
    VerifyProjectServiceLogCollection(
        context.Context,
        string,
        string,
        string,
    ) error
}
```

`LogCollectionMode` Derzeit hat zwei first-party Realisierungen:

- Docker:`syslog`
- - Nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein, nein.`journald`

Der Modus ist eine Laufzeitfähigkeit, kein Produktzweig im Core.

## Docker-Verwirklichung

Docker setzt Workload/Provider stdout/stderr mit dem syslog-Logging-Treiber und RFC5424 ein.

BaseHarbor überprüft die effektive Logging-Konfiguration und das erwartete semantische Tag des aktuell laufenden Dienstes.

Stale- oder Stop-Container dürfen die Überprüfung für die aktuelle Service-Instanz nicht erfüllen.

## Podman-Erkennung

Podman verwendet natives Quadlet plus `systemd --user`. Log-Sammlung ist journald-backed.

BaseHarbor prüft, ob die aktuell laufende Projekt/Service-Instanz existiert und löst sich auf den Pfad der Journald Collection. Fehlende oder veraltete Dienste scheitern Überprüfung.

Es gibt keine `podman compose` Rückfall.

## Normalisierung des Kollektors

Legierung Karten Laufzeit-spezifische Quell-Metadaten in die gleichen BaseHarbor-Labels vor Loki Überprüfung:

```text
baseharbor_application
baseharbor_environment
baseharbor_source_class
baseharbor_provider
baseharbor_service
```

Entsprechende Docker- und Podman-Quellen produzieren daher äquivalente semantische Etiketten, obwohl ihre Transporte unterschiedlich sind.

## Lebenszyklusregeln

Quellenregistrierung und Sammlerabstimmung sind deterministisch und idempotent.

Wenn ein Service neu erstellt wird oder seine Laufzeit-Engineering sich ändert:

1. der ausgewählte Runtime Provider die aktuelle Service-Instanz miteinander in Einklang bringt;
2. BaseHarbor prüft die in Anbieterbesitz befindliche Quellenanbindung;
3. der Kollektor normalisiert die Quelletiketten;
4. Loki-Verifikation verwendet die normalisierte semantische Identität.

Alte Laufzeitobjekte oder veraltete Logging-Konfigurationen dürfen nicht als Nachweis für den aktuellen Dienst akzeptiert werden.

## Normen

BaseHarbor definiert kein proprietäres Protokoll.

Aktuelle Implementierungen Wiederverwendung:

- Behälter stdout/stderr;
- RFC5424-Syslog für Docker;
- Systemd Journal Semantik für Podman;
- Loki/Alloy für Abholung und Lagerung.

Zukünftige Kubernetes/OpenShift-Anbieter können CRI/Plattform-Protokollierung in das gleiche normalisierte Quellmodell ohne Änderung der Anwendungsabsicht oder Verifikationssemantik abbilden.

## Sicherheit und Beweismaterial

Laufzeittransportdetails können in der Diagnose erscheinen, aber normal status/devidence berichtet semantische Quellidentität anstatt produktspezifische Transportkonfiguration.

Beglaubigungen und geheime Werte dürfen niemals in Log-Source-Etiketten, Tags oder Registrierungsstatus kodiert werden.

## Vereinbarkeit

Ändern des Laufzeitanbieters darf sich nicht ändern:

- Anforderungen an das Applikationsprotokoll;
- Anbieter/Dienstleistung/Anwendung/Umweltquellenidentität;
- Verifikations-Semantik;
- Loki-Abfrage-Semantik;
- Benutzer-sichtbare Protokollbereitschaft.

Nur der laufzeitspezifische Anbringungs-/Transportmechanismus ändert sich.
