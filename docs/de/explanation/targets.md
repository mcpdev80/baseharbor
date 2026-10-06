# Targets und Bereitstellungsziele

BaseHarbor trennt **wo** eine Anwendung betrieben wird von **was** sie benötigt und **welche Umgebung** ausgewählt ist.

Eine konkrete Bereitstellungsidentität ist:

```text
target + application + environment
```

Beispiele:

```text
docker-dev / demo     / dev
podman-dev / demo     / dev
laptop-k3s / demo     / dev
prod-ocp   / mailflow / prod
```

## Das Target-Modell

Ein BaseHarbor Target ist ein benanntes Deployment-Ziel.

```text
Target
├── Runtime Provider
├── Runtime-Zugriffsreferenz
└── Target-Bereich
```

Die Begriffe bleiben getrennt:

```text
Target
  wohin BaseHarbor arbeitet

Runtime-Provider
  welche Runtime-Implementierung Workloads realisiert

Zugriff
  wie BaseHarbor die Runtime erreicht und sich authentifiziert

Bereich
  welcher logische Bereich innerhalb der Runtime ausgewählt ist
```

Ein Target-Name trägt keine Runtime-Semantik. Ein Target bedeutet weder automatisch lokal/entfernt noch Docker/Podman/Kubernetes/OpenShift.

K3s, k3d, kind, minikube, MicroK8s und aehnliche Distributionen sind Kubernetes-Targets und keine eigenen BaseHarbor Runtime Provider.

Beispiele:

```text
Target              Provider      Access          Scope
------------------------------------------------------------
docker-dev          docker        local-docker    default
podman-dev          podman        local-podman    default
laptop-k3s          kubernetes    laptop-k3s      dev
homelab-k3s         kubernetes    homelab         dev
homelab-k3s-test    kubernetes    homelab         test
customer-prod       openshift     customer-a      project-x
```

Mehrere Targets dürfen dieselbe Zugriffsdefinition verwenden und unterschiedliche Bereiche wählen.

## Repository und installierter BaseHarbor-Zustand

Das Repository bleibt die maßgebliche Quelle für die portable Anwendungsanforderung:

```text
baseharbor.yaml
envs/<environment>/baseharbor.yaml
```

Das aktuelle Verzeichnis darf beim Erkennen von Anwendung und Umgebung helfen, aber niemals festlegen, welche Bereitstellungen die installierte BaseHarbor-Instanz kennt.

## Konfiguration und Target-Zustand

Die Benutzerkonfiguration ist global:

```text
$XDG_CONFIG_HOME/baseharbor/config.yaml
~/.config/baseharbor/config.yaml
```

Sie enthaelt Target-Definitionen, Zugriffsdefinitionen, Vorgaben und Prompt-Präferenzen. Auch Target-eigene Provider-Lifecycle-Referenzen koennen hier liegen; fuer managed OpenBao speichert der optionale Eintrag `target.openbao.recovery-file` nur den absoluten Pfad zur vom Betreiber verwahrten Wiederherstellungsdatei, niemals das Recovery-Material selbst.

Veränderlicher Runtime-/Bereitstellungszustand ist Target-bezogen:

```text
$XDG_DATA_HOME/baseharbor/targets/<target>/
~/.local/share/baseharbor/targets/<target>/
```

Damit koennen Docker-, Podman- und lokale Kubernetes/K3s-Targets unabhaengig parallel existieren, ohne versehentlich denselben BaseHarbor-State zu teilen. Gemeinsam genutzte Provider gehören zum Target-/Provider-Lebenszyklus und nicht zu einer einzelnen Anwendung. Insbesondere duerfen Löschen oder Umbenennen einer Anwendung weder die Target-weite OpenBao-Recovery-Referenz noch die gemeinsam genutzte OpenBao-Provider-Instanz entfernen.

Ein lokales K3s/Kubernetes-Target ist kein Sonderfall: es ist ein Kubernetes-Target, dessen Access-Definition einen lokalen Cluster erreicht.

## Target-Auswahl

Das effektive Target wird so aufgelöst:

```text
explizites --target
        ↓
aktiviertes `BASEHARBOR_TARGET`
        ↓
konfiguriertes Standard-Target
        ↓
Auswahl beim ersten Lauf bzw. für ein lokales Target
```

Anwendung und Umgebung bleiben unabhängige Achsen.

## Shell-Aktivierung und Prompt-Anzeige

Targets können nur für die aktuelle Shell ähnlich wie eine Python-Umgebung aktiviert werden. Verschiedene Terminals koennen dadurch gleichzeitig unterschiedliche Targets verwenden.

Die optionale Prompt-Anzeige macht das Target sichtbar, bevor ein baha-Befehl eingegeben wird:

```text
[homelab] ~/projects/demo $
[prod-ocp PROD] ~/projects/mailflow $
```

Darstellung, Position, Farben und Barrierefreiheit sind konfigurierbar. Produktion darf niemals nur über Farbe erkennbar sein.

## Lifecycle und Sicherheit

Ein Wechsel des aktiven/Standard-Targets verschiebt oder benennt bestehende Deployments niemals um.

Ein Target mit registrierten Deployments oder BaseHarbor-eigenen Runtime-Ressourcen darf nicht implizit geloescht werden.

Mutierende/destruktive Operationen zeigen die volle effektive Identitaet:

```text
target + application + environment
```

`baha app list` liest Target-/Global-State statt repository-lokalen Zustand. `baha app list --all-targets` liefert die installationsweite Sicht.

CLI, JSON und MCP verwenden dieselbe Target-/Bereitstellungsidentität.

Diese v0.4.15-Foundation wird in Issue #408 umgesetzt und bildet die Grundlage fuer #396 sowie spaetere Kubernetes-/OpenShift-Runtimes.


## Frei definierbare Prompt-Beschriftung

Die Prompt-Anzeige ist reine Darstellung und darf vom eigentlichen Target-Namen abweichen.

Beispiel:

```text
Target: laptop-docker-12
Prompt-Beschriftung: ld12
Farbe: weiches Gruen

[ld12] ~/projects/demo $
```

Die Beschriftung ist nur ein visueller Alias. Sie verändert weder Target-Identität noch Bereitstellungsschlüssel, Besitz noch Target-Auflösung.

Der Wizard erlaubt deshalb neben Presets auch:

- freie Prompt-Beschriftung;
- frei waehlbare Farbe;
- Position vor oder hinter dem Pfad beziehungsweise als rechter Prompt, wenn unterstützt;
- Live-Vorschau der exakten Darstellung.

Produktion bleibt standardmäßig auch ohne Farberkennung eindeutig sichtbar.

## Tenant-Zuordnung für Connector-Einschreibung

Die vertrauenswürdige Core-Konfiguration eines Connector-Targets benötigt
`tenant-id` als kanonische Tenant-UUID. Die Access-Definition verwendet
`provider: baseharbor-node-connector` und als `reference` die stabile Node-ID.
Nur eine aufgelöste Editor-Mitgliedschaft desselben Tenants darf eine
Einschreibungsberechtigung erstellen; Runtime und Zuordnung stammen aus Core.
Ein nicht zugeordnetes oder fremdes Target wird abgewiesen.

Die CA wird an ein ausdrücklich ausgewähltes lokales Core-Target gebunden.
HTTPS-Einschreibung allein belegt noch keine ausführbare Remote-Verbindung.
Live-Zulassung, Erneuerung, Widerruf und Docker-/Podman-Qualifizierung bleiben
offen. Details stehen im [Target-Access-Vertrag](../../spec/target-access-v1.md).
