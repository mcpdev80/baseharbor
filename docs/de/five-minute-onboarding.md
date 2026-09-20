# Einstieg in fuenf Minuten

BaseHarbor kann ein bestehendes Repository uebernehmen, ohne dass Entwickler zuerst Provider- oder Runtime-Topologie verstehen muessen.

## 1. Repository analysieren

```bash
baha app inspect .
```

Alternativ kann eine Remote-Git-URL mit der normalen Git-Authentifizierung analysiert werden:

```bash
baha app inspect https://github.com/example/app.git
```

Die Analyse ist read-only. Evidenz wird als **Detected**, **Suggested** oder **Possible** klassifiziert. Unsichere Evidenz wird nicht stillschweigend uebernommen.

Strukturierte Ausgabe:

```bash
baha app inspect . -o json
```

## 2. Anwendung uebernehmen

```bash
baha app init
```

Bei eindeutiger Erkennung non-interaktiv:

```bash
baha app init --quick
```

Optional kann BaseHarbor eine klar begrenzte, idempotente Sektion fuer Coding Agents in `AGENTS.md` pflegen:

```bash
baha app init --agents
```

Andere Projektanweisungen werden nicht veraendert.

## 3. Plan pruefen

```bash
baha plan
baha plan -o json
```

Der Plan ist read-only und liegt vor jeder Mutation.

## 4. Lokales Playground starten

```bash
baha up
```

Das lokale Playground ist die echte Compose-Runtime von BaseHarbor und kein separater Toy-Stack. Es verwendet denselben Capability-/Provider-Lifecycle, Security-Preflight, Bindings, Readiness und Ownership.

Ein Deployment-Environment kann ohne Aenderung des portablen Contracts gewaehlt werden:

```bash
baha up -e dev
```

`--environment dev` ist gleichwertig. `baseharbor.yaml` wird dadurch nicht umgeschrieben.

## 5. Zustand pruefen

```bash
baha status
baha doctor
```

Strukturierte read-only Ausgabe:

```bash
baha status -o json
baha doctor -o json
```

Secret-Werte werden nie ausgegeben. Bei Required Secrets erscheinen nur Readiness-Metadaten.

## Sichere Defaults

- Inspection mutiert weder Repository noch Runtime;
- unklare Evidenz wird nicht automatisch uebernommen;
- Providerdetails bleiben ausserhalb des portablen Application Intent;
- vorhandene Infrastruktur wird explizit angebunden und nicht still ersetzt;
- Mutationen laufen ueber Plan/Preflight/Reconcile/Verify;
- `doctor --fix` bleibt ein expliziter Human-Mutationspfad und ist nicht mit JSON kombinierbar;
- Credentials in Remote-Git-URLs werden abgelehnt; Git Credential Helper oder SSH Agent verwenden.
