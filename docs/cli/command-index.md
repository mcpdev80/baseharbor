# Complete CLI command index

This index mirrors the current human CLI surface and links it to the categorized documentation.

Command-specific `--help` is authoritative for exact flags and syntax.

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
│   └── unseal
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

## Category map

| Area | Documentation |
| --- | --- |
| Common lifecycle | [Core workflow](core.md) |
| Application authoring/lifecycle/operations | [Applications](applications.md) |
| Deployment destination | [Targets](targets.md) |
| Provider authoring and provider model | [Providers](providers.md) |
| Stack Profiles, workspaces, developer access | [Development](development.md) |
| Organization defaults and policy | [Organization and policy](organization-policy.md) |
| Authentication, trust and OpenBao operator commands | [Security and trust](security-trust.md) |
| JSON, MCP, agents and service interfaces | [Automation and agents](automation-agents.md) |
| Completion, shell integration, prompt and TUI | [Shell and UX](shell-ux.md) |
| Cross-command switches | [Global options](global-options.md) |
