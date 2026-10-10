# Vollständiger CLI-Befehlsindex

Die Einträge werden aus dem ausgelieferten Befehlsbaum erzeugt. Kurzbeschreibung und Syntax geben die englischsprachige CLI-Hilfe unverändert wieder; die verlinkten Aufgabenanleitungen erklären den menschlichen Ablauf auf Deutsch.

Der normale Einstieg ist `baha new` oder `baha init`. Explizite `app`-Befehle bleiben für Administration und Automation sichtbar. `baha update` betrifft BaseHarbor/Core; `baha app update` betrifft die Application-Source.

Exakte Optionen stehen in `baha COMMAND --help`. [Globale Optionen](global-options.md) und [JSON/MCP-Zuordnung](../reference/cli-machine-coverage.md) ergänzen jeden Eintrag. Entfernte, fehlende oder geänderte Befehle lassen den Dokumentations-Test scheitern.

| Befehl | CLI-Kurzbeschreibung (verifiziert) | CLI-Syntax (verifiziert) | Aufgabenanleitung |
| --- | --- | --- | --- |
| `baha` | BaseHarbor command-line interface | `baha <command> [options]` | [↗](index.md) |
| `baha agent` | Discover the stable BaseHarbor machine interface | `baha agent describe [-o json\|--output json]` | [↗](automation-agents.md) |
| `baha agent describe` | Describe supported machine operations and contracts | `baha agent describe [-o json\|--output json]` | [↗](automation-agents.md) |
| `baha app` | Manage declarative application backend runtimes | `baha app <command> [options]` | [↗](applications.md) |
| `baha app apply` | Converge and verify an application's backend runtime | `baha app apply [NAME] [--skip-memory-preflight]` | [↗](applications.md) |
| `baha app backup` | Create one encrypted recovery unit for an application | `baha app backup [NAME] [--output FILE] [--password-file FILE] [--include-state CLASS] [--exclude-state CLASS]` | [↗](applications.md) |
| `baha app cache` | Open the managed cache console for the current application | `baha app cache [INSTANCE] [--app NAME]` | [↗](applications.md) |
| `baha app create` | Create an application manifest in BaseHarbor state | `baha app create NAME [--environment ENV] [--sql\|--sql-instance NAME] [--cache\|--cache-instance NAME] [--key-value\|--key-value-instance NAME] [--document-db\|--document-db-instance NAME] [--messaging-queue\|--messaging-queue-instance NAME] [--messaging-pubsub\|--messaging-pubsub-instance NAME] [--messaging-stream\|--messaging-stream-instance NAME] [--s3\|--s3-bucket NAME] [--secrets\|--require-secret NAME]...` | [↗](applications.md) |
| `baha app creds` | Show connection metadata without revealing credentials by default | `baha app creds postgres\|valkey [INSTANCE] [--app NAME] [--reveal]` | [↗](applications.md) |
| `baha app destroy` | Permanently remove BaseHarbor-managed runtime resources and state | `baha app destroy [NAME] [--yes] [--full-reset]` | [↗](applications.md) |
| `baha app doctor` | Diagnose and safely repair an application's runtime | `baha app doctor [NAME] [--fix --yes]` | [↗](applications.md) |
| `baha app down` | Stop an application runtime while preserving persistent data | `baha app down [NAME]` | [↗](applications.md) |
| `baha app env` | Show the standard environment contract for an application | `baha app env [NAME] [--format dotenv\|shell\|json\|yaml] [--reveal] [--path]` | [↗](applications.md) |
| `baha app evidence` | Export secret-safe application lifecycle and verification evidence | `baha app evidence [NAME] [-e ENV\|--environment ENV] [-o json\|--output json]` | [↗](applications.md) |
| `baha app exec` | Execute a command in an application workload service | `baha app exec SERVICE COMMAND [ARG...] [--app NAME]` | [↗](applications.md) |
| `baha app init` | Create the application contract or initialize deployment settings | `baha app init --agents [--json] \| baha app init [--quick] [--json] \| baha app init [--input NAME=VALUE]... [--hostname HOST] [--tls acme\|existing\|local] [--cert-dir DIR] [--yes] \| baha app init [NAME] [-e ENV\|--environment ENV] [--sql\|--sql-instance NAME] [--cache\|--cache-instance NAME] [--key-value\|--key-value-instance NAME] [--document-db\|--document-db-instance NAME] [--messaging-queue\|--messaging-queue-instance NAME] [--messaging-pubsub\|--messaging-pubsub-instance NAME] [--messaging-stream\|--messaging-stream-instance NAME] [--s3\|--s3-bucket NAME] [--secrets\|--require-secret NAME] [--workload-component NAME]... [--workload-source KIND:PATH]` | [↗](applications.md) |
| `baha app inspect` | Inspect a repository without changing it | `baha app inspect [PATH] [--verbose] [-o json\|--output json\|--json]` | [↗](applications.md) |
| `baha app list` | List registered deployments for the effective target | `baha app list [--all-targets]` | [↗](applications.md) |
| `baha app logs` | Show logs for the current application workload | `baha app logs [SERVICE] [--follow] [--app NAME]` | [↗](applications.md) |
| `baha app new` | Create a new ecosystem-native application from a BaseHarbor contract | `baha app new [NAME] [--directory PARENT] [--stack go\|nextjs\|python\|quarkus \| --stack-profile NAME] [--emit-backstage --backstage-owner OWNER [--backstage-lifecycle LIFECYCLE]] [-e ENV\|--environment ENV] [--http] [--sql] [--cache] [--key-value] [--document-db] [--queue] [--pubsub] [--stream] [--s3] [--secrets] [--require-secret NAME]... [--telemetry] [--all] [-o json\|--output json]` | [↗](applications.md) |
| `baha app plan` | Show desired resources without changing anything | `baha app plan [NAME] [-o json\|--output json]` | [↗](applications.md) |
| `baha app preflight` | Validate an application before mutation | `baha app preflight [NAME]` | [↗](applications.md) |
| `baha app restore` | Restore and verify an encrypted application recovery unit | `baha app restore [BACKUP] [NAME] [--password-file FILE]` | [↗](applications.md) |
| `baha app runtime-identity` | Manage the app-scoped credential used for dynamic secret references | `baha app runtime-identity <rotate\|revoke> [NAME] [--yes]` | [↗](applications.md) |
| `baha app runtime-identity revoke` | Immediately revoke the application runtime credential | `baha app runtime-identity revoke [NAME] [--yes]` | [↗](applications.md) |
| `baha app runtime-identity rotate` | Rotate the application runtime credential | `baha app runtime-identity rotate [NAME] [--yes]` | [↗](applications.md) |
| `baha app secret` | Manage application secret values without printing them | `baha app secret <command> [options]` | [↗](applications.md) |
| `baha app secret delete` | Permanently delete one secret and all of its KV versions | `baha app secret delete [NAME] KEY [--yes]` | [↗](applications.md) |
| `baha app secret list` | List secret key names without values | `baha app secret list [NAME]` | [↗](applications.md) |
| `baha app secret set` | Create or replace one secret value from stdin or a file | `baha app secret set [NAME] KEY [--stdin \| --file PATH]` | [↗](applications.md) |
| `baha app secret tls-set` | Validate and store a TLS certificate chain and private key | `baha app secret tls-set [NAME] --cert-file PATH --key-file PATH [--chain-file PATH]` | [↗](applications.md) |
| `baha app shell` | Open a shell in an application workload service | `baha app shell SERVICE [--app NAME]` | [↗](applications.md) |
| `baha app show` | Show a coherent application overview | `baha app show [NAME]` | [↗](applications.md) |
| `baha app sql` | Open the managed SQL console for the current application | `baha app sql [INSTANCE] [--app NAME]` | [↗](applications.md) |
| `baha app status` | Show application runtime and readiness status | `baha app status [NAME] [-o json\|--output json]` | [↗](applications.md) |
| `baha app tls` | Inspect and maintain application TLS certificates | `baha app tls <command>` | [↗](applications.md) |
| `baha app tls update` | Check for or install a newer existing TLS certificate | `baha app tls update [--check]` | [↗](applications.md) |
| `baha app up` | Start an existing application runtime and verify readiness | `baha app up [NAME] [--yes\|-y] [--skip-memory-preflight]` | [↗](applications.md) |
| `baha app update` | Safely inspect or update a Git-backed application | `baha app update [--check] [--backup-password-file FILE \| --no-backup]` | [↗](applications.md) |
| `baha app workspace` | Manage local multi-repository workspace mappings | `baha app workspace [<init\|map\|show\|resolve\|status\|update> [options]]` | [↗](applications.md) |
| `baha app workspace init` | Create versioned source identity metadata for a multi-repository application | `baha app workspace init [--manifest PATH] --source ID=REPOSITORY [--source ...] [--oci ID=IMAGE] [--component COMPONENT=SOURCE[@SUBPATH]]... [-o json]` | [↗](applications.md) |
| `baha app workspace map` | Map one repository source identity to an existing local checkout/worktree | `baha app workspace map SOURCE PATH [--manifest PATH] [-o json]` | [↗](applications.md) |
| `baha app workspace resolve` | Resolve component source identity to local worktrees without using the current working directory | `baha app workspace resolve [--manifest PATH] [-o json\|--output json]` | [↗](applications.md) |
| `baha app workspace show` | Show canonical source identities and local workspace mappings | `baha app workspace show [--manifest PATH] [-o json\|--output json]` | [↗](applications.md) |
| `baha app workspace status` | Inspect Git state for all mapped repository sources | `baha app workspace status [--manifest PATH] [--fetch] [-o json\|--output json]` | [↗](applications.md) |
| `baha app workspace update` | Safely fast-forward mapped Git repositories | `baha app workspace update [--manifest PATH] [--check] [-o json\|--output json]` | [↗](applications.md) |
| `baha backup` | Back up the current application through its managed lifecycle | `baha backup [options]` | [↗](applications.md) |
| `baha completion` | Generate shell completion for Bash, Zsh or Fish | `baha completion bash\|zsh\|fish` | [↗](shell-ux.md) |
| `baha config` | Configure BaseHarbor user preferences | `baha config prompt [options]` | [↗](organization-policy.md) |
| `baha config organization` | Configure versioned organization/platform defaults and references | `baha config organization set\|show\|check\|update [options]` | [↗](organization-policy.md) |
| `baha config organization check` | Resolve the configured source without changing the active organization configuration | `baha config organization check [-o json]` | [↗](organization-policy.md) |
| `baha config organization set` | Resolve and activate an organization configuration source | `baha config organization set --source oci\|git\|local\|system [--location LOCATION] [--requested TAG\|REF] [--environment ENV] [-o json]` | [↗](organization-policy.md) |
| `baha config organization show` | Show the active organization configuration and effective defaults | `baha config organization show [--environment ENV] [--preferences FILE] [-o json]` | [↗](organization-policy.md) |
| `baha config organization update` | Explicitly activate the currently configured source at its newly resolved immutable version | `baha config organization update --yes [--environment ENV] [-o json]` | [↗](organization-policy.md) |
| `baha config prompt` | Configure the optional BaseHarbor shell prompt segment | `baha config prompt [--enable\|--disable] [--preset minimal\|compact\|accessible\|detailed\|none] [--position before-path\|after-path\|right] [--environment never\|critical-only\|always] [--show-application\|--hide-application] [--text-only\|--color-output] [--prod-indicator TEXT] [--label TARGET=LABEL] [--color KEY=VALUE]` | [↗](organization-policy.md) |
| `baha connect` | Allow one explicit application service to reach another | `baha connect SOURCE TARGET` | [↗](security-trust.md) |
| `baha connections` | List explicit cross-application connectivity policy | `baha connections` | [↗](security-trust.md) |
| `baha destroy` | Permanently remove BaseHarbor-managed runtime resources and state | `baha destroy [--yes] \| baha destroy --all [--yes]` | [↗](core.md) |
| `baha dev` | Manage local development conveniences | `baha dev <credentials\|domain>` | [↗](development.md) |
| `baha dev credentials` | Show or rotate the target-scoped development management login | `baha dev credentials [--reset] [--username USER] [--password-file FILE]` | [↗](development.md) |
| `baha dev domain` | Show or configure the target-scoped development domain | `baha dev domain [DOMAIN]` | [↗](development.md) |
| `baha disconnect` | Remove one explicit cross-application connectivity exception | `baha disconnect SOURCE TARGET` | [↗](security-trust.md) |
| `baha doctor` | Diagnose the current application repository, otherwise the control plane | `baha doctor [--fix --yes] [-o json\|--output json]` | [↗](core.md) |
| `baha down` | Stop the current application, or the local control plane outside a repository | `baha down [options]` | [↗](core.md) |
| `baha init` | Initialize or adopt the application in the current repository | `baha init --agents [--json] \| baha init [--quick] [--json] [--input NAME=VALUE]... [--yes]` | [↗](applications.md) |
| `baha inspect` | Inspect the current repository and its application requirements | `baha inspect [options]` | [↗](applications.md) |
| `baha list` | List managed applications on the selected Target | `baha list [options]` | [↗](applications.md) |
| `baha login` | Authenticate the BaseHarbor operator through OIDC | `baha login -e ENV\|--environment ENV` | [↗](security-trust.md) |
| `baha logout` | Remove a local BaseHarbor operator session | `baha logout -e ENV\|--environment ENV` | [↗](security-trust.md) |
| `baha mcp` | Expose BaseHarbor semantic operations over MCP | `baha mcp serve` | [↗](automation-agents.md) |
| `baha mcp serve` | Run the local stdio MCP server | `baha mcp serve` | [↗](automation-agents.md) |
| `baha new` | Create an application, stack, target, provider, or workspace | `baha new [application\|stack\|target\|provider\|workspace] [options]` | [↗](applications.md) |
| `baha node` | Enroll and manage remote Docker or Podman nodes | `baha node [add\|connect\|list\|status\|disconnect]` | [↗](applications.md) |
| `baha node add` | Create a remote Target and one-use enrollment bundle | `baha node add [NODE] [--target NAME] --runtime docker\|podman --tenant-id UUID --core-url https://HOST:PORT --core-address HOST:PORT --ca-file FILE [--environment ENV] [--output FILE] [--json]` | [↗](applications.md) |
| `baha node connect` | Enroll this host and start the rootless connector service | `baha node connect [ENROLLMENT-FILE] [--connector-bin FILE] [--json]` | [↗](applications.md) |
| `baha node disconnect` | Revoke a node identity and remove its empty Target registration | `baha node disconnect TARGET --yes [--json]` | [↗](applications.md) |
| `baha node list` | List configured remote connector nodes | `baha node list [--json]` | [↗](applications.md) |
| `baha node status` | Inspect Core enrollment state for a remote node | `baha node status TARGET [--json]` | [↗](applications.md) |
| `baha open` | Open a verified HTTPS application endpoint | `baha open [--print] [--json]` | [↗](applications.md) |
| `baha openbao` | Bootstrap and operate the BaseHarbor OpenBao trust plane | `baha openbao <command> [options]` | [↗](security-trust.md) |
| `baha openbao bootstrap` | Initialize OpenBao and establish the BaseHarbor manager identity | `baha openbao bootstrap --recovery-file PATH` | [↗](security-trust.md) |
| `baha openbao rotate` | Rotate OpenBao/control-plane credentials and managed service PKI | `baha openbao rotate --recovery-file PATH` | [↗](security-trust.md) |
| `baha openbao status` | Show OpenBao initialization, seal and manager-auth state | `baha openbao status` | [↗](security-trust.md) |
| `baha openbao unseal` | Unseal OpenBao from an operator-held recovery file | `baha openbao unseal --recovery-file PATH` | [↗](security-trust.md) |
| `baha plan` | Show the application plan in the current repository | `baha plan [NAME] [-o json\|--output json]` | [↗](core.md) |
| `baha policy` | Inspect the effective BaseHarbor policy for an application environment | `baha policy <check\|explain> [NAME] [-e ENV\|--environment ENV] [-o json\|--output json]` | [↗](organization-policy.md) |
| `baha policy check` | Evaluate effective policy against the selected application | `baha policy check [NAME] [-e ENV\|--environment ENV] [-o json\|--output json]` | [↗](organization-policy.md) |
| `baha policy explain` | Explain effective policy, defaults and bounded overrides | `baha policy explain [NAME] [-e ENV\|--environment ENV] [-o json\|--output json]` | [↗](organization-policy.md) |
| `baha provider` | Author and validate Capability Provider extensions | `baha provider <command> [options]` | [↗](providers.md) |
| `baha provider add` | Register an externally owned Capability Provider | `baha provider add ID [--descriptor DIR \| --provider-id ID --kind KIND --capability NAME] --endpoint URL [trust options] [-o json]` | [↗](providers.md) |
| `baha provider init` | Create a minimal Capability Provider authoring skeleton | `baha provider init ID [--path DIR] [-o json\|--output json]` | [↗](providers.md) |
| `baha provider inspect` | Inspect one registered external Capability Provider | `baha provider inspect ID [-o json]` | [↗](providers.md) |
| `baha provider list` | List registered external Capability Providers | `baha provider list [-o json]` | [↗](providers.md) |
| `baha provider remove` | Remove an external provider registration without destroying foreign infrastructure | `baha provider remove ID --yes [-o json]` | [↗](providers.md) |
| `baha provider test` | Run provider contract conformance checks | `baha provider test [PATH] [-o json\|--output json]` | [↗](providers.md) |
| `baha provider verify` | Verify endpoint and trust for an external Capability Provider | `baha provider verify ID [-o json]` | [↗](providers.md) |
| `baha restore` | Restore an application through the ownership-safe managed lifecycle | `baha restore [options]` | [↗](applications.md) |
| `baha serve` | Run the TLS-protected BaseHarbor runtime/control-plane API | `baha serve` | [↗](automation-agents.md) |
| `baha shell-init` | Generate BaseHarbor shell integration for Bash, Zsh or Fish | `baha shell-init bash\|zsh\|fish` | [↗](shell-ux.md) |
| `baha stack` | Discover and manage reusable development Stack Profiles | `baha stack list\|show\|create` | [↗](development.md) |
| `baha stack create` | Create a reusable Stack Profile | `baha stack create [NAME --component ID:ROLE:STACK ...] [--extends NAME] [--scope user\|repository] [-o json\|--output json]` | [↗](development.md) |
| `baha stack list` | List built-in, user and repository Stack Profiles | `baha stack list [-o json\|--output json]` | [↗](development.md) |
| `baha stack show` | Show a resolved Stack Profile | `baha stack show NAME [-o json\|--output json]` | [↗](development.md) |
| `baha status` | Show application status in a repository, otherwise control-plane status | `baha status [-o json\|--output json]` | [↗](core.md) |
| `baha target` | Inspect and manage BaseHarbor deployment targets | `baha target [list\|show\|create\|delete\|activate\|deactivate] [-o json\|--output json\|--json]` | [↗](targets.md) |
| `baha target activate` | Persist the active deployment target for this user | `baha target activate NAME` | [↗](targets.md) |
| `baha target create` | Create a deployment target | `baha target create NAME --runtime-provider PROVIDER --access ACCESS --access-provider PROVIDER --reference REFERENCE [--scope SCOPE] [--default] [--docker-endpoint SOCKET \| --docker-context CONTEXT] [--docker-mode rootless\|rootful]` | [↗](targets.md) |
| `baha target deactivate` | Clear persisted active deployment target | `baha target deactivate` | [↗](targets.md) |
| `baha target delete` | Delete an unused target | `baha target delete NAME` | [↗](targets.md) |
| `baha target list` | List configured targets | `baha target list [-o json\|--output json\|--json]` | [↗](targets.md) |
| `baha target show` | Show one target | `baha target show [NAME] [-o json\|--output json\|--json]` | [↗](targets.md) |
| `baha trust` | Inspect, export or explicitly install the managed local BaseHarbor CA | `baha trust <status\|export\|install\|uninstall> [options]` | [↗](security-trust.md) |
| `baha trust export` | Export only the public managed-local CA certificate | `baha trust export --output PATH` | [↗](security-trust.md) |
| `baha trust install` | Explicitly install the managed-local CA into the host trust store | `baha trust install --yes` | [↗](security-trust.md) |
| `baha trust status` | Show whether the managed-local CA is trusted by this host | `baha trust status` | [↗](security-trust.md) |
| `baha trust uninstall` | Remove only BaseHarbor-owned host trust anchors | `baha trust uninstall [--yes] [--json]` | [↗](security-trust.md) |
| `baha tui` | Open the interactive BaseHarbor status dashboard | `baha tui [--json]` | [↗](shell-ux.md) |
| `baha up` | Start BaseHarbor and, inside an application repository, converge the application | `baha up [-e ENV\|--environment ENV] [--yes] [--skip-memory-preflight] [--control-plane-only] [--ha] [--trust-host-ca] [--postgres-port PORT] [--openbao-port PORT] [--recovery-file PATH]` | [↗](core.md) |
| `baha update` | Safely inspect or update BaseHarbor itself | `baha update [--check] [--yes] [--channel stable\|rc \| --version VERSION] [--recover]` | [↗](core.md) |
| `baha use` | Select and persist the active Core or deployment target | `baha use [TARGET]` | [↗](applications.md) |
| `baha version` | Print build version | `baha version [-o json\|--output json\|--json]` | [↗](core.md) |
| `baha whoami` | Show the authenticated BaseHarbor operator identity | `baha whoami -e ENV\|--environment ENV` | [↗](security-trust.md) |
| `baha workspace` | Configure local workspace sources and repository mappings | `baha workspace [<init\|map\|show\|resolve\|status\|update> [options]]` | [↗](development.md) |
| `baha workspace init` | Create versioned source identity metadata for a multi-repository application | `baha workspace init [--manifest PATH] --source ID=REPOSITORY [--source ...] [--oci ID=IMAGE] [--component COMPONENT=SOURCE[@SUBPATH]]... [-o json]` | [↗](development.md) |
| `baha workspace map` | Map one repository source identity to an existing local checkout/worktree | `baha workspace map SOURCE PATH [--manifest PATH] [-o json]` | [↗](development.md) |
| `baha workspace resolve` | Resolve component source identity to local worktrees without using the current working directory | `baha workspace resolve [--manifest PATH] [-o json\|--output json]` | [↗](development.md) |
| `baha workspace show` | Show canonical source identities and local workspace mappings | `baha workspace show [--manifest PATH] [-o json\|--output json]` | [↗](development.md) |
| `baha workspace status` | Inspect Git state for all mapped repository sources | `baha workspace status [--manifest PATH] [--fetch] [-o json\|--output json]` | [↗](development.md) |
| `baha workspace update` | Safely fast-forward mapped Git repositories | `baha workspace update [--manifest PATH] [--check] [-o json\|--output json]` | [↗](development.md) |
