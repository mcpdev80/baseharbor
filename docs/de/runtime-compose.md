# Lokale Control Plane

Die aktuelle BaseHarbor-Control-Plane ist bewusst single-node und local-first. `baha up` startet PostgreSQL 18 und OpenBao 2.6.x, standardmäßig nur auf Loopback.

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

## Globaler Runtime-State

Die Control Plane ist benutzer-/maschinenbezogen. Der Standardpfad ist:

```text
$XDG_DATA_HOME/baseharbor/runtime/
```

oder ohne `XDG_DATA_HOME`:

```text
~/.local/share/baseharbor/runtime/
```

`BASEHARBOR_STATE_DIR` bleibt ein expliziter Override. Ein alter repository-lokaler `.baseharbor/runtime`-Stand wird nur zur Kompatibilität wiederverwendet, wenn noch kein globaler Zustand existiert.

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