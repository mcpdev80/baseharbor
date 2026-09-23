# Einstieg

Dieser Einstieg bringt eine bestehende Anwendung unter BaseHarbor zum Laufen, ohne dass du zuerst Provider-Interna verstehen musst.

## 1. Repository prüfen

```bash
baha app inspect .
```

Die Prüfung ist read-only.

## 2. Application Contract anlegen

```bash
baha app init
```

Bei eindeutigem Repository:

```bash
baha app init --quick
```

## 3. Plan prüfen

```bash
baha plan
```

## 4. Anwendung starten

```bash
baha up -e dev
```

Compose ist aktuell die vollständige Runtime. Kubernetes und OpenShift kommen später.

## 5. Ergebnis prüfen

```bash
baha status
baha doctor
```

BaseHarbor meldet Erfolg erst, wenn die relevante Capability verifiziert wurde.

Weitere Themen:

- [Architektur](../explanation/architecture.md)
- [Application Contract](../explanation/application-contract.md)
- [Provider](../explanation/providers.md)
- [Security](../explanation/security.md)
