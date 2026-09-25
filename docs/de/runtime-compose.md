# Lokale Control Plane

Die aktuelle BaseHarbor-Control-Plane ist bewusst single-node. Auf Docker-/Podman-Targets ist sie local-first und gehoert dem effektiven Target. `baha up` startet PostgreSQL 18 und OpenBao 2.6.x, standardmaessig nur auf Loopback.

Docker fuehrt die generierte Runtime ueber Docker Compose aus. Podman uebersetzt dasselbe Compose-basierte Runtime-Modell in native Quadlet-Units und verwaltet sie rootless ueber `systemd --user`. `podman-compose` wird fuer den BaseHarbor-Podman-Lifecycle nicht benoetigt.

Die verwalteten Control-Plane-Container laufen als gehaertete Runtime-Komponenten und nicht als privilegierte Bootstrap-Helfer. PostgreSQL und OpenBao verwenden explizite Non-Root-Identitaeten, ein read-only Root-Filesystem, droppen alle Linux-Capabilities und setzen `no-new-privileges`. Schreibbar bleiben nur explizit benoetigte Volumes bzw. tmpfs-Pfade.

OpenBao schreibt seine generierte lokale Konfiguration in ein fluechtiges, schreibbares `/openbao/config`-tmpfs; persistente Provider-Daten bleiben auf dem dedizierten `/openbao/file`-Volume. BaseHarbor setzt den vom Image vorgesehenen `SKIP_CHOWN`-Modus, weil der Container bereits direkt als Non-Root-User `openbao` startet. Ein Root-Start oder eine `CAP_CHOWN`-Ausnahme ist nicht erforderlich.

## Port-Auswahl beim ersten Start

Standardports sind PostgreSQL `5432` und OpenBao `8200`. Vor der Initialisierung werden sie geprüft.

```bash
baha up
```

Bei belegten Ports schlägt `baha` freie Alternativen vor. Für Automation:

```bash
baha up --yes
```

Explizite Ports:

```bash
baha up --postgres-port 15432 --openbao-port 18200
```

Belegte explizite Ports führen zu einem Fehler statt zu einer stillen Umkonfiguration.

## Target-scoped Runtime-State

Control Plane, Provider und Deployments gehoeren dem effektiven Target. Der Standardpfad ist:

```text
$XDG_DATA_HOME/baseharbor/targets/<target>/
```

oder ohne `XDG_DATA_HOME`:

```text
~/.local/share/baseharbor/targets/<target>/
```

Darunter liegen unter anderem `runtime/`, `providers/` und `deployments/`. Mehrere Docker-/Podman-Targets koennen dadurch parallel existieren, ohne BaseHarbor-eigenen Zustand zu teilen.

`BASEHARBOR_STATE_DIR` und repository-lokaler `.baseharbor/runtime`-State sind nur noch Legacy-Kompatibilitaet fuer vor-v0.4.15 erzeugten Runtime-State; neuer Target-owned State verwendet die XDG-Target-Grenze.

## OpenBao

```bash
baha openbao status
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Bootstrap erzeugt den eingeschränkten BaseHarbor-Manager, verifiziert ihn und widerruft danach den initialen Root-Token. Recovery-Material wird ausschließlich in die explizit angegebene Datei geschrieben.

Nach einem Neustart:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
```

HA, öffentliche Exposition, Kubernetes und automatisches KMS/HSM-Unseal sind noch separate zukünftige Deployment-Profile.