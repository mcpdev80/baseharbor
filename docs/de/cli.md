# `baha` CLI

`baha` ist die zentrale Operator- und Entwickler-Schnittstelle von BaseHarbor. Mutationen erfolgen fail-closed: prüfen, ändern, verifizieren.

## Wichtige Befehle

```text
baha
├── up / down / status / doctor
├── serve
├── app
│   ├── init / create / list / show
│   ├── plan / preflight / apply
│   ├── env / status / doctor
│   ├── backup / restore
│   ├── down / up / destroy
│   ├── runtime-identity rotate|revoke
│   └── secret set|list|delete|tls-set
├── openbao status|bootstrap|unseal
└── version
```

Die exakte Syntax liefert immer die ausführbare Hilfe:

```bash
baha --help
baha app --help
baha openbao --help
```

## Control Plane

```bash
baha up
baha up --yes
baha up --postgres-port 15432 --openbao-port 18200
```

Beim ersten Start werden Ports geprüft. Belegte Standardports werden nicht blind verwendet.

## Anwendung

```bash
baha app init mailflow --postgres --redis --require-secret SECRET_KEY
baha app plan
baha app preflight
baha app apply
baha app status
baha app doctor
```

## Environment und Bindings

```bash
baha app env
baha app env --format json
baha app env --path
```

Credential-haltige Werte sind standardmäßig maskiert.

## Secrets

```bash
printf '%s' "$API_TOKEN" | baha app secret set API_TOKEN --stdin
baha app secret list
baha app secret delete API_TOKEN --yes
```

## Backup und Restore

```bash
baha app backup --password-file ./backup-password.txt
baha app restore ./backup.bhbackup --password-file ./backup-password.txt
```

## Version

```bash
baha version
```

Offizielle Releases enthalten Semantic Version, Commit und Build-Zeit. Development-Builds sind als solche erkennbar.