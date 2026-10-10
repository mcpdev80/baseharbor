# Lokale Control-Plane-Runtime

Die aktuelle BaseHarbor-Control-Plane wird primär lokal betrieben und gehört zum jeweils wirksamen Target. Die Referenzimplementierung von v0.4.21 ist innerhalb eines einzelnen Runtime-Hosts redundant: PostgreSQL und OpenBao verwenden provider-eigene HA. **Ausfallsicherheit gegen den Verlust des gesamten Hosts ist bei Docker/Podman auf einem Einzelhost nicht gegeben.**

## Dienste

`baha up` erstellt die Runtime-Definition und startet:

- PostgreSQL 18 als dreiköpfigen Patroni-/Spilo-Cluster mit etcd-Koordination und stabilem HAProxy-Endpunkt.
- OpenBao 2.7.x mit drei HA-Mitgliedern auf PostgreSQL-HA und einem stabilen HAProxy-Endpunkt für API und Weboberfläche.

Standardmäßig werden nur die stabilen logischen Endpunkte an Loopback gebunden; die Mitgliedsidentitäten bleiben Runtime-intern.

Docker nutzt Docker Compose. Podman übernimmt dasselbe Compose-basierte Modell, erzeugt native Quadlet-Units und verwaltet diese rootless über `systemd --user`. `podman-compose` ist für den Podman-Lifecycle nicht erforderlich.

Die verwalteten Container sind gehärtet: PostgreSQL und OpenBao laufen unter expliziten Nicht-root-Identitäten mit schreibgeschütztem Root-Dateisystem, ohne Linux-Capabilities und mit `no-new-privileges`. Schreibbare Bereiche erhalten eigene Volumes oder tmpfs-Mounts.

OpenBao startet direkt mit seiner endgültigen Storage-Konfiguration: einer eigenen Datenbank `openbao` und einem eingeschränkten Datenbankbenutzer im vorhandenen PostgreSQL-Control-Plane-Provider. Es gibt keinen separaten Raft-Lifecycle und keinen Kompatibilitätspfad für unveröffentlichten `storage.file`-Zustand.

Der erste Bootstrap erfolgt ausschließlich über TLS. BaseHarbor erstellt kurzlebiges Bootstrap-Vertrauen, richtet bei der PostgreSQL-Erstinitialisierung die OpenBao-Datenbank samt Benutzer ein und startet OpenBao mit PostgreSQL `verify-full`. Sobald die OpenBao-PKI bereitsteht, werden beide Control-Plane-Dienste auf verwaltete PKI-Zertifikate rotiert.

OpenBao terminiert HTTPS selbst und verwendet `tls_auto_reload` für Listener-Zertifikate. PostgreSQL-Zertifikate werden separat abgeglichen, weil PostgreSQL den privaten Schlüssel beim Start in einen nur für den Eigentümer lesbaren tmpfs-Pfad kopiert.

Zugangsdaten-Dateien sind ausschließlich für den Eigentümer zugänglich. Erzeugte PostgreSQL-Zugangsdaten, die dedizierten OpenBao-Storage-Zugangsdaten und konfigurierte Ports bleiben bei Folgestarts erhalten.

## Befehle

```bash
baha up
baha status
baha doctor
baha down
```

`baha status` meldet die tatsächliche Betriebsbereitschaft und nicht nur laufende Container.

## OpenBao-Lifecycle

OpenBao verwendet keinen statischen Development-Root-Token.

```bash
baha openbao status
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Beim Bootstrap wird der gemeinsame Shamir-Seal initialisiert, alle HA-Mitglieder werden entsiegelt, der KV-v2-Mount `baseharbor/` und AppRole-Auth aktiviert, die eingeschränkte BaseHarbor-Manager-Identität erstellt und verifiziert und schließlich der initiale Root-Token widerrufen.

Nach einem Neustart reichen normalerweise:

```bash
baha up
baha openbao status
```

`baha up` verwendet den für das Target gespeicherten Recovery-Dateipfad. Das Recovery-Material verbleibt beim Operator außerhalb des normalen BaseHarbor-Zustands. Nach einem Umzug ist `baha up --recovery-file PATH` die explizite Alternative.

Automatisches Unseal über KMS/HSM/Transit ist ein zukünftiges Betriebsprofil. Die derzeitige lokale HA-Implementierung benötigt weiterhin vom Operator aufbewahrtes Recovery-Material, wenn OpenBao nach einem Prozess- oder Mitgliedsneustart versiegelt ist.

## Gültigkeitsbereich

Die aktuelle Control-Plane ermöglicht Prozess- und Mitgliedskontinuität sowie rollierende Wartung innerhalb eines Runtime-Hosts. Sie beansprucht **keine** Ausfallsicherheit bei Host-Verlust, öffentliche Netzwerkerreichbarkeit, Kubernetes-Bereitstellung oder automatisches KMS-/HSM-Unseal. Dafür sind separate Architektur- und Deployment-Nachweise erforderlich.
