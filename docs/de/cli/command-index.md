# Vollständiger CLI-Befehlsindex

Dieser Index spiegelt die aktuelle menschliche CLI-Oberfläche wider und verknüpft sie mit der kategorisierten Dokumentation.

Befehlsspezifisch `--help` ist maßgeblich für exakte Flags und Syntax.

```text
baha
├── init
├── up
├── down
├── destroy
├── plan
├── status
├── doctor
├── update
├── version
├── login
├── logout
├── whoami
├── target
│   ├── list
│   ├── show
│   ├── create
│   ├── delete
│   ├── activate
│   └── deactivate
├── provider
│   ├── init
│   └── test
├── stack
│   ├── list
│   ├── show
│   └── create
├── dev
│   ├── domain
│   └── credentials
├── config
│   └── prompt
├── policy
│   ├── check
│   └── explain
├── trust
│   ├── status
│   ├── export
│   └── install
├── agent
│   └── describe
├── mcp
│   └── serve
├── serve
├── connect
├── disconnect
├── connections
├── openbao
│   ├── status
│   ├── bootstrap
│   ├── unseal
│   └── rotate
├── shell-init bash|zsh|fish
├── prompt
├── completion bash|zsh|fish
├── tui
└── app
    ├── new
    ├── workspace
    │   ├── init
    │   ├── map
    │   ├── show
    │   └── resolve
    ├── init
    ├── inspect
    ├── create
    ├── list
    ├── show
    ├── plan
    ├── preflight
    ├── apply
    ├── env
    ├── status
    ├── doctor
    ├── evidence
    ├── backup
    ├── restore
    ├── update
    ├── psql
    ├── redis
    ├── valkey
    ├── creds
    ├── logs
    ├── shell
    ├── exec
    ├── tls
    │   └── update
    ├── down
    ├── up
    ├── destroy
    ├── runtime-identity
    │   ├── rotate
    │   └── revoke
    └── secret
        ├── set
        ├── list
        ├── delete
        └── tls-set
```

## Karte der Kategorie

Bereich Dokumentation
| --- | --- |
Allgemeiner Lebenszyklus[Core workflow](core.md) |
Anwendung Autoren/Lebenszyklus/Betriebe[Applications](applications.md) |
Einsatzziel[Targets](targets.md) |
Provider-Autoren- und Provider-Modell[Providers](providers.md) |
Stack Profile, Workspaces, Entwicklerzugriff[Development](development.md) |
Organisations-Defaults und Richtlinien[Organization and policy](organization-policy.md) |
Authentisierung, Vertrauen und OpenBao-Betreiberbefehle[Security and trust](security-trust.md) |
JSON, MCP, Agenten und Service-Schnittstellen[Automation and agents](automation-agents.md) |
Abschluss, Shell-Integration, prompt und TUI-[Shell and UX](shell-ux.md) |
Befehlsübergangsschalter[Global options](global-options.md) |
