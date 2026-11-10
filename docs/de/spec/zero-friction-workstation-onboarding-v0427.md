# Core-zu-Workstation-Onboarding ohne manuelle Konfiguration (v0.4.27)

## Bedienvertrag

Auf einer frischen VM wird BaseHarbor installiert und der lokale Core gestartet:

```text
baha up
baha onboard
```

Der Core stellt eine zeitlich begrenzte, einmalig verwendbare Einladungsdatei
mit Dateirechten `0600` bereit. Diese Datei wird privat auf den
Entwicklungsrechner übertragen. Dort genügt:

```text
baha onboard --import FILE
```

Der Client erzeugt seinen privaten Geräteschlüssel lokal, prüft die
Core-Zertifizierungsstelle und die Installationsidentität, registriert
ein gerätegebundenes mTLS-Zertifikat und speichert das Remote-Target.
Ein Node-Connector ist bei Core und Docker/Podman auf derselben VM nicht erforderlich.
Ein Browser-Login ist für diese Gerätekopplung nicht vorgesehen.

## Zuständigkeiten und Sicherheit

Der lokale Core muss dem ausgewählten Target gehören und im Zustand
`READY` sein. Weder ein fremder Core noch ein Remote-Target darf
Einladungen über die lokale Bootstrap-Berechtigung ausgeben.
Die bestehende OpenBao-CA bleibt alleiniger Zertifikatsaussteller.
Einladungen enthalten nur öffentliches CA-Material, die genaue HTTPS-Adresse,
eine Installations-ID, den Workstation-Namen sowie kurzlebige
Einmal-Zugangsdaten. Private Schlüssel bleiben stets auf dem Client.

Der Import verwirft ungültige, abgelaufene oder bereits verbrauchte Einladungen,
fremde Installationen und CA-Bindungen, unsichere Dateien sowie lokale
Profilkonflikte. Die Geräteidentität wird bei Remote-Anfragen gegen die
persistente Registry und Widerrufsinformationen geprüft. Geräte erhalten
keine Operator- oder Administratorrolle. Das vorhandene OIDC-Modell bleibt
für die getrennte Browser-/Operatorverwaltung bestehen.

## Automatischer LAN-HTTPS-Endpunkt

Bei einem lokalen, startbereiten Core richtet `baha onboard` bei Bedarf
einen systemd-User-Service für die Core-API ein. Die Software ermittelt eine
konkrete private IPv4-Adresse, verwendet standardmäßig Port `18443` und
stellt das dazu passende Serverzertifikat über die bereits vorhandene
OpenBao-CA aus. Die geprüfte HTTPS-Adresse wird installationsgebunden
und geschützt gespeichert.

Dazu sind keine manuellen Angaben von
`BASEHARBOR_MANAGED_CORE_TLS`, `BASEHARBOR_API_LISTEN_ADDR` oder
`BASEHARBOR_API_PUBLIC_URL` erforderlich. Ein explizit konfigurierter
BYOC-Endpunkt beziehungsweise Node-Connector wird nicht automatisch verändert.
Wenn keine eindeutige sichere LAN-Adresse gefunden wird, wird die
Konfiguration abgelehnt, anstatt eine Wildcard- oder Loopback-Adresse zu
veröffentlichen.

Der User-Service wird mit systemd beaufsichtigt und startet zur
Zertifikatserneuerung mindestens einmal täglich neu. Damit er nach
Abmeldung weiterläuft, wird systemd-Linger geprüft beziehungsweise
aktiviert. Schlägt dies mangels Berechtigung fehl, wird die Inbetriebnahme
nicht fälschlich als dauerhaft verfügbar bestätigt.

## Abnahme und Nachweis

Der Draft-PR prüft den Go-Code in den vorhandenen Shards `cli-0`,
`cli-1` und `cli-2` sowie die betroffenen übrigen Pakete.
Der Wechsel zu `Ready for review` startet die vollständige PR-CI.

Für diese Änderung sind echte Docker- und Podman-E2E-Nachweise
verpflichtend: Core `READY`, Einladung, Import in eine isolierte
Workstation, Remote-App-Plan/Apply/Status/Update sowie Destroy.
Übersprungene, fehlende und fehlgeschlagene Untertests gelten nicht
als Erfolg. Nur eigene Testressourcen dürfen bereinigt werden.

Bis diese Nachweise für den endgültigen PR-Commit erfolgreich sind,
ist die Änderung nicht mergefähig. Besonders die
Zertifikatserneuerung sowie Recovery nach teilweise abgeschlossenem
Import sind mit realen Integrationsfällen zu bestätigen.

## Technische Referenzen und unveraenderliche Grenzen

Die lokale Ausstellung eines einmaligen Grants verwendet `workstationenrollment.Authority.Create`. Der versionierte Datenaustausch liegt unter `internal/onboardingbundle`. Der produktive HTTP-Baustein `ProductionHandler` bleibt an die bestehende Authentifizierungs- beziehungsweise Grant-Grenze gebunden. Ein kuenstlich erzeugtes `identity.Principal{IssuerVerified:true}` ist ausdruecklich verboten; die lokale Bootstrap-Berechtigung muss aus der nachweislich eigenen Core-Installation folgen.
