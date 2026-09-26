# Targets und Deployment-Ziele

BaseHarbor trennt **wo** eine Anwendung betrieben wird von **was** sie benoetigt und **welches Environment** ausgewaehlt ist.

Eine konkrete Deployment-Identitaet ist:

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
├── Runtime Access Reference
└── Target Scope
```

Die Begriffe bleiben getrennt:

```text
Target
  wohin BaseHarbor arbeitet

Runtime Provider
  welche Runtime-Implementierung Workloads realisiert

Access
  wie BaseHarbor die Runtime erreicht/authentifiziert

Scope
  welcher logische Bereich innerhalb der Runtime ausgewaehlt ist
```

Ein Target-Name traegt keine Runtime-Semantik. Target bedeutet weder automatisch lokal/remote noch Docker/Podman/Kubernetes/OpenShift.

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

Mehrere Targets duerfen dieselbe Access-Definition verwenden und unterschiedliche Scopes waehlen.

## Repository und installierter BaseHarbor-State

Das Repository bleibt Source of Truth fuer portablen Application Intent:

```text
baseharbor.yaml
envs/<environment>/baseharbor.yaml
```

Das aktuelle Verzeichnis darf Application und Environment erkennen helfen, aber niemals festlegen, welche Deployments die installierte BaseHarbor-Instanz kennt.

## Config versus Target-State

Benutzerkonfiguration ist global:

```text
$XDG_CONFIG_HOME/baseharbor/config.yaml
~/.config/baseharbor/config.yaml
```

Sie enthaelt Target-Definitionen, Access-Definitionen, Defaults und Prompt-Praeferenzen. Auch Target-eigene Provider-Lifecycle-Referenzen koennen hier liegen; fuer managed OpenBao speichert der optionale Eintrag `target.openbao.recovery-file` nur den absoluten Pfad zur operator-gehaltenen Recovery-Datei, niemals das Recovery-Material selbst.

Veraenderlicher Runtime-/Deployment-State ist Target-scoped:

```text
$XDG_DATA_HOME/baseharbor/targets/<target>/
~/.local/share/baseharbor/targets/<target>/
```

Damit koennen Docker-, Podman- und lokale Kubernetes/K3s-Targets unabhaengig parallel existieren, ohne versehentlich denselben BaseHarbor-State zu teilen. Shared Provider gehoeren zum Target-/Provider-Lifecycle und nicht zu einer einzelnen Application. Insbesondere duerfen Application-Destroy oder -Rename weder die Target-weite OpenBao-Recovery-Referenz noch die shared OpenBao Provider-Instanz entfernen.

Ein lokales K3s/Kubernetes-Target ist kein Sonderfall: es ist ein Kubernetes-Target, dessen Access-Definition einen lokalen Cluster erreicht.

## Target-Auswahl

Der effektive Target wird so aufgeloest:

```text
explizites --target
        ↓
aktiviertes BASEHARBOR_TARGET
        ↓
konfigurierter Default-Target
        ↓
First-Run/Local-Target-Auswahl
```

Application und Environment bleiben unabhaengige Achsen.

## Shell-Aktivierung und Prompt

Targets koennen shell-lokal wie ein Python-vEnv aktiviert werden. Verschiedene Terminals koennen dadurch gleichzeitig unterschiedliche Targets verwenden.

Die optionale Prompt-Anzeige macht das Target sichtbar, bevor ein baha-Befehl eingegeben wird:

```text
[homelab] ~/projects/demo $
[prod-ocp PROD] ~/projects/mailflow $
```

Darstellung, Position, Farben und Accessibility sind konfigurierbar. Production darf niemals nur ueber Farbe erkennbar sein.

## Lifecycle und Sicherheit

Ein Wechsel des aktiven/default Targets verschiebt oder benennt bestehende Deployments niemals um.

Ein Target mit registrierten Deployments oder BaseHarbor-eigenen Runtime-Ressourcen darf nicht implizit geloescht werden.

Mutierende/destruktive Operationen zeigen die volle effektive Identitaet:

```text
target + application + environment
```

`baha app list` liest Target-/Global-State statt repository-lokalen State. `baha app list --all-targets` liefert die installationsweite Sicht.

CLI, JSON und MCP verwenden dieselbe Target-/Deployment-Identitaet.

Diese v0.4.15-Foundation wird in Issue #408 umgesetzt und bildet die Grundlage fuer #396 sowie spaetere Kubernetes-/OpenShift-Runtimes.


## Frei definierbares Prompt-Label

Die Prompt-Anzeige ist reine Darstellung und darf vom eigentlichen Target-Namen abweichen.

Beispiel:

```text
Target: laptop-docker-12
Prompt-Label: ld12
Farbe: weiches Gruen

[ld12] ~/projects/demo $
```

Das Label ist nur ein visueller Alias. Es veraendert weder Target-Identitaet noch Deployment-Schluessel, Ownership oder Target-Aufloesung.

Der Wizard erlaubt deshalb neben Presets auch:

- freies Prompt-Label;
- frei waehlbare Farbe;
- Position vor/hinter dem Pfad bzw. Right Prompt, wenn unterstuetzt;
- Live Preview der exakten Darstellung.

Production bleibt standardmaessig auch ohne Farberkennung eindeutig sichtbar.
