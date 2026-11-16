# Zugriff auf Management-Oberflächen v1

## Gültigkeitsbereich

Diese Spezifikation beschreibt Authentifizierung, Autorisierungszuordnung und Umgebungsrichtlinien für von BaseHarbor verwaltete Infrastruktur- und Management-Oberflächen. Anwendungsbezogene Business-Benutzer und Application-RBAC gehören nicht dazu.

## Authentifizierungsintegration

Jede Oberfläche deklariert genau eine wirksame Klasse:

```text
native-oidc
standards-auth-adapter
native-credential
unsupported
```

Die Priorität lautet: natives OIDC/OAuth2, vorhandener standardbasierter Auth-Adapter/Proxy, providerspezifische Zugangsdaten, ansonsten `unsupported`.

BaseHarbor führt keinen proprietären Authentifizierungsproxy ein, nur um eine Oberfläche einzubinden. Unsichere Integration wird ausdrücklich als nicht unterstützt gemeldet; sie darf keine unabhängige Benutzerverwaltung erzeugen.

## Optionale Perses-Management-Oberfläche

Perses ist eine OPTIONALE, provider-eigene Management- und Visualisierungs-Oberfläche. Es ist nie eine Application-Fähigkeit und nie ein verpflichtendes BaseHarbor-Core-Bündel; es ist eine Realisierung der vorhandenen optionalen Observability-Oberfläche `management-ui`, deren Daten-Eigentümer im Backend (Prometheus sowie Loki/Tempo, sofern die angepinnte Perses-Verfassung deren Plugins ausliefert und unterstützt) sie bereits deklarieren.

- Die Perses-Verfassung (Produktversion plus Produktions-Image/Digest) wird als Realisierungs-Metadaten angepinnt. Eine Perses-Oberfläche, die nicht ausgewählt ist (oder mit einem ungültigen oder nicht verfügbaren Pin deklariert wird) MUSS `not_configured` melden (fail-closed); sie ist nie `configured_unverified` oder `verified`.
- Authentifizierung ist opt-in; wenn aktiviert, ist die von upstream verpflichtende Verschlüsselungsschlüssel-Anforderung einzuhalten. Externe Benachrichtigungen und Routing bleiben standardmäßig deaktiviert.
- Metrics-, Logs- und Traces-Fähigkeiten dürfen nie falsch deklariert werden und die Prometheus-Management-Oberfläche darf nie stillschweigend ersetzt werden.

Der öffentliche Status (`baha status --json`, `baha app status --json`) meldet die Perses-Management-Oberfläche über das maschinenlesbare Feld `perses` mit genau einem von drei unterscheidbaren Zuständen:

- `not_configured`: das Manifest wählt keine funktionierende Perses-Oberfläche — entweder gar keine `perses`-Auswahl oder eine Auswahl mit ungültigem oder nicht verfügbarem Pin. Das ist der Fail-Closed-Standardfall, nie „configured“.
- `configured_unverified`: die Oberfläche ist ausgewählt und ihre angepinnte Verfassung gegen den einkompilierten Standard-Pin geprüft; dieses Core definiert nur die Nahtstelle und liefert keine Perses-Prüfung, daher bleibt die Erreichbarkeit ungeprüft. Das ist der einzige erreichbare Zustand einer ausgewählten Oberfläche: eine Auswahl mit gültigem Pin ohne echte Beobachtung des Gesundheitspfads wird immer hier gemeldet.
- `verified`: nur wenn eine echte Perses-Gesundheitsprüfung die Oberfläche beobachtet. Die reine Pin-Prüfung DARF diesen Zustand nicht erzeugen.

## Management-Rollen

Das providerneutrale Profil lautet:

```text
native OIDC/OAuth2
    -> existing standards-based auth adapter/proxy
    -> provider-native credential
    -> unsupported
```

Die Rollen gelten nur für BaseHarbor-verwaltete Infrastruktur und Management-Oberflächen. Provider-Adapter ordnen sie nach Möglichkeit nativen Rollen und Richtlinien zu. Jede Zuordnung ist `exact`, `limited` oder `unsupported`. Es darf keine feinere Berechtigung zugesichert werden, als der Provider tatsächlich erzwingt. Application-Benutzer, Gruppen, Rollen und Rechte bleiben bei Application beziehungsweise Identity-Provider.

## Umgebungsrichtlinien

### Entwicklung

- Natives OIDC/OAuth2 wird bevorzugt.
- Fehlt OIDC, ist eine gemeinsame Target-gebundene Entwickleridentität der Klasse A für kompatible Oberflächen der Standard, soweit technisch sicher.
- Interne Maschinenzugangsdaten bleiben getrennt.

### Test und Produktion

- Zentral verwaltete Identity/OIDC wird bevorzugt.
- Ohne OIDC gelten isolierte Zugangsdaten je Oberfläche als Standard.
- Gemeinsame Zugangsdaten sind nur mit ausdrücklicher Auswahl und korrekten Autorisierungs- und Isolationsgarantien zulässig.
- Interne Maschinenzugangsdaten bleiben getrennt.

## Abnahme

Jede ausgelieferte Management-Oberfläche dokumentiert und überprüft HTTPS/Trust, Authentifizierungsklasse, Identity-Anbindung oder Fallback, Rollenzuordnung, Herkunft und Eigentümerschaft von Zugangsdaten, Rotation/Widerruf, secret-sichere Diagnose und gegebenenfalls HA-Kontinuität.

Maschinenlesbare Ergebnisse müssen die gleiche wirksame Authentifizierungsklasse und Rollenzuordnung wie CLI und Statusanzeigen liefern.


## Weitere technische Beispiele

```text
admin
edit
view
audit
```
