# ADR 0003: Managed Runtime API-Lebenszyklus

## Status

Vorgeschlagen für den nächsten MVP-Operationsabschnitt.

## Kontext

Anwendungen mit BaseHarbor-managed dynamische geheime Referenzen benötigen die `/runtime/v1/...` API zur Laufzeit. Benötigt einen Operator zu halten `baha serve` Das manuelle Laufen würde die Verfügbarkeit von Anwendungen von einem CLI-Prozess im Vordergrund abhängig machen und würde den BaseHarbor-MVP-Vertrag verletzen.

## Entscheidung

Die Laufzeit-API ist ein control-plane-Service von BaseHarbor, der vom gleichen Lebenszyklus wie die gebündelte control-plane-Laufzeit verwaltet wird.

- `baha up` Startet die Laufzeit-API zusammen mit den BaseHarbor Control-Plane Abhängigkeiten.
- Anwendungen werden nie gestartet oder überwacht `baha serve` sich selbst.
- `baha app apply/up` Nur die von BaseHarbor injizierte stabile Laufzeit-API-Endpunkt- und App-scoped-Laufzeitidentität verbrauchen.
- Laufzeit API-Transport bleibt TLS-only.
- Die Laufzeit-only-Operation erfordert keine humane OIDC oder die Operator Control-Plane Datenbank API.
- Aktivierung des Betreibers `/api/...` Die Oberfläche bleibt ein separates explizites Anliegen und erfordert weiterhin OIDC plus die Kontrollebenendatenbank.
- Der Laufzeit-API-Dienst erhält nur den BaseHarbor-Zustand und den Container-Laufzeit-Zugriff, der zum Auflösen von Anwendungs-Scopes erforderlich ist; Anwendungen erhalten niemals Manager-Anmeldeinformationen.
- Keine Anwendung SDK,`baha login`, oder menschliches Token eingeführt wird.

## Laufzeitvertrag

Anwendungen erhalten weiterhin nur Standard-Prozess/Datei-Eingänge:

```text
BASEHARBOR_RUNTIME_API_URL=https://...
BASEHARBOR_RUNTIME_TOKEN_FILE=/run/baseharbor/runtime/token
```

Das Token wird nur in Workload-Dienste eingebunden, die explizit die Laufzeit-Identitätsvariablen verbrauchen.

## Folgen

Der nächste Operationsabschnitt muss ein eigenständiges Service-Artefakt, TLS-Materiallebenszyklus, Gesundheits- und Lesekontrollen, stabile Endpunkt-Entdeckung und `baha up/down/status/doctor` Abdeckung. Eine saubere BaseHarbor-Installation darf nicht von einem Operator abhängen, der einen interaktiven CLI-Prozess am Leben erhält.
