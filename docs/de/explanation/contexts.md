# Kontexte und Deployment-Ziele

BaseHarbor trennt **wo** eine Anwendung betrieben wird von **was** die Anwendung benoetigt und **welches Environment** ausgewaehlt ist.

Eine konkrete Deployment-Identitaet ist:

```text
context + application + environment
```

Beispiele:

```text
local    / demo     / dev
k3s      / demo     / dev
k8s-prod / mailflow / prod
```

## Context-Modell

Ein BaseHarbor Context ist Deployment-/Operator-State:

```text
Context
├── Runtime Provider
├── Runtime Access Reference
└── Target Scope
```

Beispiele:

```text
local
  runtime: docker
  access: local
  scope: default

k3s
  runtime: kubernetes
  access: homelab-k3s
  scope: default

k8s-prod
  runtime: kubernetes
  access: corp-prod
  scope: team-a-prod
```

K3s wird dabei als Kubernetes-Target behandelt und nicht als eigener portabler Runtime-Typ. Kubernetes-/OpenShift-spezifische Details bleiben hinter dem jeweiligen Runtime Provider.

Context, Application und Environment sind unabhaengige Achsen. Ein Context bedeutet nicht automatisch `dev`, `test` oder `prod`, und ein Environment waehlt keine Runtime aus.

## Repository und installierter BaseHarbor-State

Das Repository bleibt die Source of Truth fuer portablen Application Intent:

```text
baseharbor.yaml
envs/<environment>/baseharbor.yaml
```

Das aktuelle Verzeichnis darf dabei helfen, Application und Environment zu erkennen. Es darf aber niemals festlegen, welche Deployments die installierte BaseHarbor-Instanz kennt.

```text
cwd darf beantworten: "Welche Application meine ich?"
cwd darf nicht beantworten: "Welche Applications kennt BaseHarbor?"
```

Context-Definitionen sind Benutzerkonfiguration. Deployment-/Runtime-State ist user-globaler BaseHarbor-State.

## Context-Auswahl

Der effektive Context wird deterministisch aufgeloest:

```text
explizites --context
        ↓
aktiviertes BASEHARBOR_CONTEXT
        ↓
konfigurierter Default-Context
        ↓
local
```

Application und Environment werden davon getrennt aufgeloest.

## Shell-lokale Aktivierung

Contexts sollen wie ein Python-vEnv shell-lokal aktiviert werden koennen.

Damit koennen verschiedene Terminals gleichzeitig unterschiedliche Targets verwenden:

```text
Terminal A -> local
Terminal B -> k3s
Terminal C -> k8s-prod
```

## Sichtbarer Prompt

Der aktive Context soll sichtbar sein, **bevor ein baha-Befehl eingegeben wird**.

Die Prompt-Integration ist optional und konfigurierbar. Der Default bleibt kompakt:

```text
[k3s] ~/projects/demo $
~/projects/demo [k3s] $
```

Environment-Farben koennen als zusaetzliches Signal dienen:

- dev: weiches Gruen;
- test/stage: warmes Amber/Orange;
- prod: gedecktes Rot.

Farbe ist nie das einzige Production-Signal. Fuer Barrierefreiheit kann zusaetzlich `TEST` oder `PROD` angezeigt werden.

Der Prompt-Wizard soll Presets, Live Preview, Text-only/Accessibility und die Position vor dem Pfad, hinter dem Pfad oder als Right Prompt unterstuetzen, wenn die Shell das verlaesslich kann.

## Deployment-Uebersicht

`baha app list` wird Context-/Global-State-basiert und nicht CWD-basiert.

Damit koennen mehrere Applications und Environments in einem Context sowie dieselbe Application in mehreren Contexts gleichzeitig existieren.

CLI, JSON und MCP muessen dieselbe effektive Context-/Deployment-Identitaet verwenden.

Die v0.4.15-Foundation wird in Issue #408 umgesetzt und ist die Grundlage fuer die spaetere Runtime-Provider-Arbeit in #396 sowie Kubernetes/OpenShift.
