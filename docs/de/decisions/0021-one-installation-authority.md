# ADR 0021: Eine Installation-Autorität für lokale und Remote-Ausführung

- Status: vorgeschlagen für v0.4.27 (Teil von Task `t-20261018-r27-authority-contract`).

## Kontext

BaseHarbor führt lokale Direktausführung (die `baha`-CLI und in-Prozess-Steuerflächen)
und Remote-Clients (JSON, MCP, HTTP) aus. Beide müssen sich über **welches Objekt
maßgeblichen Zustand besitzt** einigen. Ohne eine ausdrückliche Autorität akkumulierte
das System parallele Besitzer: die lokale Installation-Wurzel, die Remote-
Managerverbindung, das Ziel und die gecachte Kopie der Entwickler-Workstation. Das
v0.4.27-Ziel ist eine ausgewählte Installation, die maßgeblichen Deployments-,
Anbieter-, Identitäts-, Consent-Revision- und Mutationszustand für seine admitted
Targets verwaltet, wobei die Managerverbindung und das Ziel als separate Auswahlen
genannt werden.

Dieses ADR verfeinert die bestehende `target-access-v1.md`-Grenze (Runtime Provider
vs. Target Access Provider) um die **Zuständigkeit**, nicht um den Transport. Es
erfindet keinen neuen generischen State-Rahmen, keine neue `coreinstallation`-State-
Datenbank und keine zweite Installation-Axis. Es wiederverwendet den bereits maßgeb-
lichen Core-Lifecycle in `internal/coreinstallation` und die Deployment-/State-
Besitzer in `internal/deployment`.

## Entscheidung

### 1. Installation-Identität ist maßgeblich; Managerverbindung und Ziel sind getrennt

Drei unabhängige Auswahlen, nie verwechselt:

- `installation` ist eine pro Maschinenwurzel, opaque UUID.
- `management-connection` ist, wie ein Remote-Client das ausgewählte Core erreicht.
- `target` ist das admitted Environment, das Core bedient.

- **Installation** ist die dauerhafte Autorität. Sie wird von
  `internal/coreinstallation` verwaltet (`installation_id`, `owner=baseharbor`,
  `placement=shared`, Version `baseharbor.core-installation/v1`, Fähigkeiten
  `{sql,secrets,identity}`). Ihre Auswahl `{target, runtime, machineRole, ha}` ist
  nach der Persistierung unveränderlich; eine widersprüchliche Auswahl wird
  abgelehnt, nicht ersetzt.
- **Managerverbindung** ist, wie ein Remote-Client das ausgewählte Core adressiert.
  Sie ist ein `Target Access Provider` (z. B. `baseharbor-node-connector`,
  `native-api`) hinter dem canonical Target Access Vertrag. Sie ist **nicht** die
  Installation und **nicht** das Ziel.
- **Ziel** ist das admitted Environment. Es ist eine Projektion
  `{runtime_provider, access_provider, access_reference, scope}` und trägt keine
  Autorität über die Installationidentität oder das Consent.

Ein Remote-Client **adressiert** die ausgewählte Installation durch seine
Managerverbindung; er wird **nie** zur Autorität der Installation.

### 2. Eine maßgebliche State-/Lock-Autorität pro Managed Deployment

- Lokale DirekAusführung possesses Deployment-State direkt über die Core. Es gibt
  keinen zweiten lokalen Besitzer und keine Reconciliation zwischen einem
  „lokalen Cache" und einer „lokalen Wahrheit".
- Deployment-State ist keyed durch
  `targets/<target>/deployments/<deployment_id>/`
  (`internal/deployment/deployment_store.go`). Ein Deployment-Record pro
  `{target, deployment_id}`; mehrere Records, die dieselben
  `{application_id, environment}` auf einem Target beanspruchen, schließen fehl.
- Target-Konfigurationsmutation nimmt einen einzigen Besitzer über
  `.installation.lock` + `coreinstallation.AcquireStateFileLock`
  (`internal/deployment/config_mutation.go`). Kein concurrenter Mutationsbesitzer.
- Die Installation-Identität, der Deployment-Besitzer und die Mutations-Sperre sind
  derselbe semantischen Kern, nicht drei konkurrierende Autoritäten.

### 3. Lokale DirekAusführung wiederverwendet den semantischen Kern ohne
   Login/Server/SQL/Secrets/Identity-Bundle

Die lokale Ausführung einer bereits ausgewählten Operation verwendet denselben
Core-Typen und verpflichtet kein unbedingtes Stack:

- **Keine Login-Pflicht für vertrautes lokale `dev`.** `dev`-lokale Arbeit muss
  keinen Operator-OIDC-Boundary erzwingen, um installation-lokale Operationen
  durchzuführen (siehe `security-invariants.md`). Login, Server und eine
  OIDC-Operator-Boundary sind Anforderungen der **Remote**-Control-Plane-Surface,
  nicht der lokalen Ausführung.
- **Kein gebundener SQL/Secrets/Identity-Stack.** Lokale Arbeit konsumiert nur die
  Fähigkeiten, die die ausgewählte Installation tatsächlich braucht.
  `installation.json` zeigt Bereitschaft pro Fähigkeit; eine Fähigkeit, die die
  Operation nicht verwendet, wird weder bereitgestellt oder verlangt. Eine gecachte
  Fähigkeit stellt keine Live-Bereitschaft her.
- **Keine Server-Pflicht.** Der Control-Plane-Server (`baha serve`) ist eine
  Remote-Grenze. Die lokale Ausführung ist unabhängig davon, ob der Server präsent
  oder erreichbar ist.

### 4. Remote-Clients besitzen niemals maßgeblichen Zustand und fallen niemals zurück

- Remote-Clients adressieren die ausgewählte Installation durch die
  Managerverbindung. Sie schreiben nie maßgeblichen Installation-, Deployment-,
  Consent- oder Mutationszustand.
- **Kein maßgeblicher lokaler Cache.** Kein maßgeblicher Remote-Zustand wird auf
  Entwickler-Workstations, NFS-Mounts, Home-Directory-Synchronisierung,
  Baha-to-Baha-Föderation oder sonstiger lokaler Kopie gespeichert. Eine gecachte
  last-known-Beobachtung ist eine veraltete Beobachtung, kein maßgeblicher Zustand.
- **Observed-at-Frische.** Eine gecachte Beobachtung trägt ihren `observed_at`-
  Timestamp. Wenn die Live-Beobachtung fehlschlägt, meldet der Remote-Client die
  Beobachtung als veraltet/abgelehnt und ihre Frische; er downgrade niemals eine
  fehlgeschlagene Live-Prüfung zu „fertig aus Cache". Gecachter Zustand darf
  niemals aktuelle `READY` geltend machen, wenn die Beobachtung sie nicht
  bestätigen konnte.
- **Kein lokaler Rückgriff.** Eine fehlende, fremde oder abgelaufene
  Managerverbindung schließt fehl. Es gibt keinen Umschalten auf die Ausführung
  gegen eine lokale Installation oder Laufzeit. „Eine fehlende oder fremde Session
  wählt niemals eine lokale Laufzeit" (bereits in `target-access-v1.md` gesagt;
  dieses ADR verallgemeinert es auf das Autoritätsmodell).

### 5. Consent/Revision-Akzeptanz, observed-at-Frische und unabhängige
   Sicherheitsgrenzen

- **Consent/Revision-Akzeptanz.** Eine veraltete Plan/Consent, eine inkompatible
  Revision, eine fremde Installation-Auswahl, eine falsche Installation, eine
  verlorene Antwort, ein Prozess-Neustart, ein nicht erreichbares Ziel oder ein
  revoziertes Gerät sind alles **negative** Fälle, das fehl schließt. Ein Consent,
  der einmal akzeptiert wurde, bleibt nicht als stehende Remote-Autorität
  bestehen; jede maßgebliche Entscheidung validiert gegen den live Installation-
  zustand neu.
- **Observed-at-Frische** ist eine first-class, erzwungene Eigenschaft (Punkt 4).
- **Unabhängige Sicherheitsgrenzen.** Remote-Zugriffssurfaces, der lokale
  Ausführungspfad und die Installation-State-Wurzel bleiben unabhängig besessen und
  verifiziert. Ein grüner Gate auf einer Grenzen autorisiert nicht eine andere; die
  stärkste Grenze wird nie geschwächt, um eine schwächere zu tragen. Fremde
  Revision oder Consent auf jeder Grenzen wird ohne Übernahme abgelehnt.

## Folgen

- Downstream-Besitzer (state-owner, connection-client, delivery-boundary,
  execution-journal, node-service, client-commands) konsumieren die minimal
  aufrufbare Selection/Authority-Typen, die dieses ADR fordert: eine Installation,
  ein State-Besitzer, remote-only Autorität mit Frische — kein neuer Rahmen.
- Der canonical Target Access Vertrag (`target-access-v1.md`, canonical wire
  records) bleibt die Transportgrenze; dieses ADR legt die Autoritätsebene oben
  drauf, ohne es zu ersetzen.
- Keine zweite Installation-Axis, keine neue `coreinstallation`-State-Datenbank,
  kein generischer State-Rahmen sind impliziert.

## Begrenztes Produktionssichtfeld (siehe Task `t-20261018-r27-authority-contract`)

- `internal/coreinstallation`: minimal, aufrufbare Selection/Authority-Typen, die
  *Installation-Identität + ein State-/Lock-Besitzer + remote-only Autorität mit
  Frische* ausdrücken; begrenzt, kein Rahmen.
- Positive und negative Regressionstests abdeckend:
  - Managerverbindung ≠ Target ≠ Installation (Auswahlen nie vertauscht);
  - kein lokaler Rückgriff für eine fehlende/fremde/abgelaufene Remote-Autorität;
  - veralteter `observed_at` ⇒ abgelehnt, niemals geltend gemacht `READY`;
  - fremde Revision/Consent ⇒ abgelehnt.

## Kompatibilität

Dieses ADR formalisiert Invariante, die bereits in `target-access-v1.md`,
`security-invariants.md`, ADR 0009 (shared core) und ADR 0017 (stable identity)
vorhanden sind. Es ändert kein portable Application Intent oder die bestehende
Core-Lifecycle-Typen; es dokumentiert den Autoritätsvertrag für Downstream-
Besitzer.
