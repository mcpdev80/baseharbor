# Core-zu-Workstation-Onboarding ohne manuelle Konfiguration (v0.4.27-Kandidat)

## Ablauf

Der Ablauf umfasst zwei Rechner. Core-Einrichtung und Einladungserstellung
finden auf der Core-VM statt. Nach der Übertragung werden alle
Anwendungsbefehle vom Entwicklungsrechner über das importierte Core-Target
ausgeführt.

### Auf der Core-VM

Führe die aktuelle `baha`-CLI außerhalb eines Anwendungs-Repositories aus:

```text
baha up
baha onboard
```

`baha up` richtet die Core-Provider PostgreSQL, OpenBao und Keycloak ein oder
startet sie. Vor Änderungen am Provider-Zustand prüft Baha, ob ORAS vorhanden
ist. ORAS löst OCI-Referenzen zu unveränderlichen Digests auf und verifiziert
Runtime-Artefakte; es ersetzt Docker oder Podman nicht. Fehlt ORAS, erklärt
Baha den Grund und fragt nach Zustimmung. Bei Zustimmung installiert es das
offizielle, per Prüfsumme verifizierte Release unter
`~/.local/bin` des aktuellen Benutzers. Es verwendet weder `sudo` noch eine
systemweite Paketinstallation. Bei Ablehnung beginnt die Core-Einrichtung
nicht. `--yes` ist keine Zustimmung zur Installation; installiere ORAS in
nicht-interaktiven Abläufen separat und starte den Befehl erneut.

`baha onboard` wartet, bis der lokale Core `READY` ist. Danach erstellt es eine
einmal verwendbare Einladung und gibt den Pfad zur geschützten JSON-Datei aus.
Es richtet außerdem den verwalteten LAN-HTTPS-Endpunkt ein. Ein separater
Befehl `baha serve` und eine Browser-Anmeldung gehören nicht zu diesem Ablauf.

Entwicklungs-Builds mit eingebettetem Commit erstellen aus dem installierten
Linux-Core-Binary ein gecachtes Runtime-Image mit dem kanonischen Binary-Rezept.
Broker und Executor verwenden dadurch denselben Build; ein Quellcode-Checkout
oder Go-Compiler auf dem Core-Rechner ist nicht nötig. Explizite Image-Overrides
behalten ihre Auswahl.

Übertrage die JSON-Datei sicher auf den Entwicklungsrechner. Sie enthält einen
kurzlebigen, einmal verwendbaren Registrierungsschlüssel. Gib ihren Inhalt
nicht aus, committe sie nicht und lege sie nicht in einem gemeinsam genutzten
Verzeichnis ab.

### Auf dem Entwicklungsrechner

Verwende die aktuelle Baha-CLI und importiere die übertragene Datei:

```text
baha onboard --import /path/to/invitation.json
baha use dev-docker-1
```

Der Import fragt nach einem Target-Namen für die Workstation und schlägt
`<environment>-<runtime>-<number>` vor, zum Beispiel `dev-docker-1`. Du kannst den
Vorschlag übernehmen oder einen gültigen, noch freien Namen eingeben. Für
Automatisierung lässt sich der Name direkt angeben:
`baha onboard --import FILE --target-name NAME`. Dieser Name ist nur ein lokaler
Alias; das Core-Target und die Installationsidentität aus der Einladung bleiben
im geschützten Verbindungsprofil unverändert gebunden. Der Import prüft Core
und CA, erstellt den privaten Geräteschlüssel lokal, löst die Einmaleinladung
ein und speichert die Remote-Verbindung. `baha use NAME` wählt den gewünschten
Core ausdrücklich aus. Ein eigener lokaler Core-Target kann daneben bestehen
und weiter verwendet werden. Anwendungsbefehle laufen auf dem
Entwicklungsrechner über das ausgewählte Target, niemals direkt auf der VM.

Prüfe im Anwendungs-Repository zuerst den Quellstand und erstelle oder
aktualisiere den dort verwalteten BaseHarbor-Vertrag. Starte die Anwendung
anschließend beim ausgewählten Core:

```text
baha app inspect .
baha app init
baha up
baha status
baha doctor
```

`baha app init` schreibt `baseharbor.yaml`, registriert den vollständigen Vertrag
beim ausgewählten Core und speichert die Deployment-Einstellungen. Es startet
keine Container. `baha up` übernimmt App-Namen und Umgebung aus dem Manifest;
ein separates `app create` und ein wiederholtes `--environment dev` entfallen.
Der Target-Name ist ein frei wählbarer Alias und legt keine Umgebung fest.
Die Registrierung verwendet nur die bereits von diesem Repository bestätigte
Core-Identität erneut; eine fremde App mit gleichem Namen wird nicht übernommen. Dabei
gelten dieselben Plan-, Kandidatenbestätigungs-, Provider-Freigabe- und
Prüfschranken wie bei `baha app apply`; `--yes` darf diese Zustimmungen nicht
ersetzen. Interaktive Remote-Aufrufe zeigen den konkreten Provider-Plan,
unterscheiden verbleibende Installation, Start, Updates und Prüfungen von
vorhandenen Ressourcen und fragen einmal nach der
Zustimmung zu diesen Folgen und dem exakten Quellkandidaten. Bei destruktiven
Plänen benennt diese Frage ausdrücklich die dauerhafte Datenlöschung. Ohne Eingabemöglichkeit wird die strukturierte
Bestätigungsanforderung zurückgegeben, ohne automatisch erneut auszuführen.
Führe `status` und `doctor` nach dem Start im selben Repository und
mit demselben ausgewählten Target aus. Ein manueller Schritt `baha serve` ist
nicht erforderlich.

## Verbindliche Sicherheits- und Zuständigkeitsanforderungen

1. `baha up` auf dem Core-Rechner verantwortet den Core-Provider- und
   Bootstrap-Lebenszyklus. Einladungen dürfen nur vom lokal besitzenden Core
   im Zustand `READY` ausgestellt werden. Remote- oder fremde Installationen
   müssen geschlossen abgelehnt werden.
2. `baha onboard` bezieht den initialen Mandanten über die geschützte lokale
   Bootstrap-Berechtigung. Es darf weder
   `identity.Principal{IssuerVerified:true}` künstlich erzeugen noch die
   Authentifizierung der produktiven Operator-API abschwächen.
3. Die Einladung bindet Core-Installation, exakten HTTPS-Endpunkt, öffentliche
   CA, Workstation-Kennung, Einmalberechtigung und begrenzte Laufzeit. Die
   sensible JSON-Datei erhält ausschließlich Benutzerrechte. Das
   Workstation-Zertifikat läuft standardmäßig fünf Jahre; kürzere Laufzeiten
   bleiben einstellbar.
4. Der Import lehnt Symlinks und zu offene Dateirechte, unbekannte Vertrags-
   felder, falsche CAs, abgelaufene oder bereits eingelöste Einladungen,
   fremde Installationen und veränderte Endpunkte ab. Der private Schlüssel
   wird auf der Workstation erstellt und bleibt dort.
5. Die Registrierung erteilt eine Berechtigung mit minimalen Rechten, keine
   Operator- oder Administratorrolle. Core-Daten und Eigentümerschaft bleiben
   erhalten.
6. Vor dem Speichern prüft der Import das ausgestellte Zertifikat anhand der
   eingeladenen CA, Installations-ID und SPIFFE-Workstation-Identität. Ein
   Fehler darf weder verwendbare Teilzugangsdaten noch ein nutzbares Target
   hinterlassen.
7. Alle späteren Anwendungsoperationen starten auf dem Entwicklungsrechner
   und verwenden das ausgewählte Remote-Target. Ein lokaler Runtime-Fallback
   darf Remote-Fehler nicht verdecken.

## Abnahme

- Frische Core-VM: `baha up` schließt Einrichtung und Abhängigkeitsprüfung ab;
  `baha onboard` erzeugt eine geschützte JSON-Einladung.
- Frische Workstation: Der Import prüft und registriert das Gerät. Die
  ausdrückliche Target-Auswahl verbindet mit dem vorgesehenen Core.
- Negativtests decken Replay, Ablauf, parallelen Import, nicht autorisierte
  Geräte, falsche CA, fremden Core, manipulierten Endpunkt, abgelehnten Reset,
  Symlinks und offene Dateirechte, Rollback bei Teilfehlern sowie den Schutz
  privater Schlüssel ab.
- Docker- und Podman-E2E-Pfade prüfen Core `READY`, Einladung, Import auf der
  Workstation sowie Remote-App-Plan/Apply/Status/Änderung/Destroy. Es dürfen
  nur eigene Testressourcen entfernt werden.
- CI gilt nur für den geprüften Quellstand und ersetzt keinen Live-Nachweis
  von Workstation zu Core.

## Implementierungsgrenzen

Der versionierte Einladungsvertrag liegt unter
`internal/onboardingbundle`. `workstationenrollment.Authority.Create` erzeugt
die Einmalberechtigung und speichert nur Token- und Nonce-Digests. Die
produktiven Machine-APIs bleiben authentifiziert; nur der eng begrenzte
Einladungsendpunkt nimmt die Einmalberechtigung an. Browser-OIDC bleibt eine
separate Oberfläche für menschliche Betreiber.
