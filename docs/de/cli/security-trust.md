# Security und Trust

## Host-Vertrauen

Nach erfolgreichem lokalem Deployment:

```text
baha trust status
baha trust export --output PATH
baha trust install --yes
```

Exportiert wird die öffentliche managed-local CA, kein privater Schlüssel. Installation ins Host-Vertrauen ist eine ausdrücklich genehmigte Host-Mutation mit `baha trust install --yes`. Externe Firmen-/BYOC-CAs bleiben Operator-Besitz. Danach den Status erneut prüfen.

## OpenBao

```text
baha openbao status
baha openbao bootstrap --recovery-file PATH
baha openbao unseal --recovery-file PATH
baha openbao rotate --recovery-file PATH
```

Rotation ersetzt Manager-AppRole, Control-Plane-Datenbankzugangsdaten und managed Service-PKI. Neue Pfade werden geprüft, bevor altes Material zurückgezogen wird. Recovery-Material bleibt geschützt beim Operator/Core, nicht im Browser-Eingabevertrag.

## Operator und Verbindungen

`baha login`, `baha whoami` und `baha logout` verwalten die Operator-Sitzung. Lokales Development kann trusted-local bleiben; test/prod folgen der konfigurierten OIDC-Grenze. Tokens nicht in Befehlsargumente kopieren.

`baha connect`, `baha disconnect` und `baha connections` verwalten explizite Verbindungen. Zugangsdaten, private Schlüssel und Secret-Werte gehören nicht in Application Intent oder normale Maschinenausgabe.

Weiter: [Sicherheit](../explanation/security.md), [Authentifizierung](../explanation/authentication.md), [exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/security-trust/).


## Workstation-Gerät registrieren

Die Verbindung eines Entwicklungsrechners mit seinem Core ist eine separate
Geräte-Einschreibung, keine OIDC-Anmeldung eines Operators und keine
Node-Connector-Einschreibung. Auf der Core-VM `baha up` und danach
`baha onboard` ausführen. Die private JSON-Einladung auf die Workstation
übertragen und dort mit `baha onboard --import FILE` importieren. Übernimm den
Vorschlag für den lokalen Alias oder gib `--target-name NAME` an; wähle ihn danach
mit `baha use NAME` aus. Der Alias ist vom Core-Target in der Einladung
getrennt. Die Einladung registriert eine Geräteidentität mit minimalen Rechten,
keine Operator-Rolle. Siehe den
[Ablauf Core-zu-Workstation](../spec/zero-friction-workstation-onboarding-v0427.md).

Menschliche Operatoren melden sich weiterhin über die konfigurierte
OIDC-Grenze mit `baha login` an. Die Geräteeinschreibung verändert oder
umgeht diese Authentifizierungsrichtlinie nicht.


Nach dem Einlösen der Einladung und der Prüfung der Core-Identität bietet der
interaktive Import an, der öffentlichen CA auf dem Entwicklungsrechner zu
vertrauen. Er erklärt die Wirkung auf HTTPS und zeigt den SHA-256-Fingerabdruck.
Bei Zustimmung installiert BaseHarbor Systemvertrauen und die CA in unterstützte
vorhandene Linux-NSS-Speicher von Chromium und Firefox, einschließlich erkannter
Firefox-Profile in Snap und Flatpak. Private Schlüssel und Browser-Einstellungen
werden nicht importiert. Browser danach neu starten. Fehler einzelner
Browser-Speicher werden gesondert angezeigt.

Für Skripte bestätigt `baha onboard --import FILE --trust-host-ca` die Installation;
`--skip-host-trust` lässt die Zertifikatsspeicher unverändert. Ohne ausdrückliche
Zustimmung verändert ein nichtinteraktiver Import das Host-Vertrauen nicht.
Die Systeminstallation kann sudo benötigen. Für Browser wird NSS `certutil`
benötigt (Debian/Ubuntu: `libnss3-tools`). Fehlende Werkzeuge und nicht unterstützte
Profile werden mit Hinweisen gemeldet; Browser-Werkzeuge werden nicht ungefragt
installiert.

Scheitert die Vertrauensinstallation, bleibt die Verbindung gespeichert.
Die Einmal-Einladung nicht erneut importieren: Mit
`baha --target NAME trust install --yes` wiederholen. Der Befehl prüft den
Remote-Core und installiert das Vertrauen auf dem Entwicklungsrechner, ohne
einen lokalen Core zu starten. `baha trust uninstall --yes` entfernt nur erfasste,
unveränderte BaseHarbor-Zertifikate aus System und Browsern. Vom Betreiber
installierte Zertifikate bleiben erhalten.

## Weitere Befehlsbeispiele

```text
baha login
baha logout
baha whoami
```

```text
baha connect
baha disconnect
baha connections
```

```bash
baha trust status
baha trust export --output ./baseharbor-dev-ca.pem
```

```bash
baha trust install --yes
```

```bash
baha whoami
baha login
baha whoami
baha logout
```

## Optionale Namensauflösung auf dem Entwicklerrechner

Unter Linux bietet der Einladungsimport separat an, konkrete App- und GUI-Namen
in `/etc/hosts` zu verwalten. Das ist **standardmäßig ausgeschaltet** und von
CA-Vertrauen unabhängig. Vor der Zustimmung erscheinen die Namen und die IP
des verifizierten Cores; sudo kann erforderlich sein. Es wird kein DNS-Dienst
installiert. Wildcards funktionieren in Hosts-Einträgen nicht.
`--configure-hosts` aktiviert die Verwaltung beim Import ausdrücklich;
`--skip-hosts` überspringt das Angebot. Nach der Zustimmung gleichen erfolgreiche
CLI-Deployments die Einträge dieses Targets ab. Ohne Eingabe oder ausdrückliche
Freigabe bleibt die Hosts-Datei unverändert.

Für bestehende Verbindungen:

```text
baha --target NAME dev hosts status
baha --target NAME dev hosts sync
baha --target NAME dev hosts remove --yes
baha --target NAME dev cleanup --yes
```

`sync` zeigt die Namen und fragt beim erstmaligen Aktivieren nach Zustimmung;
`sync --yes` aktiviert die Verwaltung ausdrücklich für Skripte. `remove`
deaktiviert nur die Hosts-Verwaltung. `cleanup` entfernt den unveränderten
eigenen Hosts-Abschnitt dieses Targets sowie dessen registrierte System- und
Browser-CA. Verwendet eine andere aktive Verbindung dieselbe CA, bleibt sie
installiert. Fremde Einträge, Namenskonflikte und manuell veränderte Abschnitte
oder Zertifikate bleiben erhalten und werden gemeldet. Die lokale Bereinigung
benötigt keinen erreichbaren Core und erhält Apps, Daten und Geräteanmeldung.
Auch das Entfernen der lokalen Verbindung im Onboarding-Wizard führt diese
Bereinigung aus, bevor dessen Verbindungsdatensatz entfernt wird. Erneutes
explizites Installieren des CA-Vertrauens aktiviert die CA-Referenz wieder.

Unter Linux unterstützt der Import NSS-Speicher von Chromium und Microsoft Edge,
einschließlich vorhandener Chromium-Snap-Revisionsspeicher und Chromium-/Edge-
Flatpak-Speicher. Firefox verwendet eigene Profil-Datenbanken (nativ, Snap und
Flatpak). BaseHarbor importiert nur in vorhandene unterstützte Speicher und legt
keine Browserprofile oder Browserrichtlinien an. Wird Firefox erst später
installiert oder ein Profil neu angelegt, erneut
`baha --target NAME trust install --yes` ausführen. Betroffene Browser nach einer
Vertrauensänderung vollständig neu starten. Neu installierte Browser-CA-Einträge
werden für dieselbe fingerprintgeprüfte Bereinigung wie System-CA-Einträge erfasst.
