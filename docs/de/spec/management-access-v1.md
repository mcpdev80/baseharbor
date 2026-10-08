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

## Management-Rollen

Das providerneutrale Profil lautet:

```text
admin
edit
view
audit
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
