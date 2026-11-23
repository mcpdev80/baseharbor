package main

import (
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type commandCoverage struct {
	Command        string `json:"command"`
	Classification string `json:"classification"`
	Operation      string `json:"operation,omitempty"`
	MCPTool        string `json:"mcp_tool,omitempty"`
	Reason         string `json:"reason,omitempty"`
	JSONSurface    string `json:"json_surface,omitempty"`
}

// Keep unresolved product operations explicit instead of calling them exclusions.
func currentCommandCoverage() []commandCoverage {
	var rows []commandCoverage
	seen := map[string]bool{}
	var walk func(*cli.Command, string)
	walk = func(command *cli.Command, parent string) {
		if command.Hidden {
			return
		}
		path := strings.TrimSpace(parent + " " + command.Name)
		// The router uses the first matching name; later duplicate registrations
		// do not create another supported command path.
		if seen[path] {
			return
		}
		seen[path] = true
		row := classifyCommandCoverage(path, command)
		rows = append(rows, row)
		for _, alias := range command.Aliases {
			aliasRow := row
			aliasRow.Command = strings.TrimSpace(parent + " " + alias)
			if seen[aliasRow.Command] {
				continue
			}
			seen[aliasRow.Command] = true
			aliasRow.Classification = "alias"
			aliasRow.Reason = "Canonical command: " + path + "; shares its coverage classification."
			rows = append(rows, aliasRow)
		}
		for _, child := range command.Children {
			walk(child, path)
		}
	}
	walk(rootCommand(), "")
	sort.Slice(rows, func(i, j int) bool { return rows[i].Command < rows[j].Command })
	return rows
}

func classifyCommandCoverage(path string, command *cli.Command) commandCoverage {
	row := commandCoverage{Command: path, Classification: "gap", Reason: "Supported product operation pending semantic machine coverage in #787."}
	if command.Run == nil {
		row.Classification = "presentation"
		row.Reason = "Command group; invoke its supported subcommands."
		return row
	}
	// Workspace namespace clones (workspaceNamespaceCommand) classify through
	// their canonical app-workspace twins; group rows above already matched.
	if isWorkspaceNamespaceClone(path) {
		twin := classifyCommandCoverage(canonicalCoveragePath(path), command)
		twin.Command = path
		return twin
	}
	if operationID, ok := cliMachineOperations[path]; ok {
		if operation, exists := machine.OperationByID(operationID); exists {
			row.Classification = "semantic"
			row.Operation = operation.ID
			row.MCPTool = operation.MCPTool
			row.Reason = cliSemanticModes[path]
			row.JSONSurface = "CLI -o json / typed MCP result"
			switch path {
			case "baha app edit":
				row.JSONSurface = "Interactive CLI / typed two-phase MCP consent"
			case "baha app env":
				row.JSONSurface = "CLI --format json (masked) / typed MCP result"
			case "baha trust export":
				row.JSONSurface = "CLI --json (--output is CA destination) / typed MCP result"
			}
		}
		return row
	}
	if reason, ok := cliExcludedCommands[path]; ok {
		row.Classification = "excluded"
		row.Reason = reason
		return row
	}
	if reason, ok := cliPresentationCommands[path]; ok {
		row.Classification = "presentation"
		row.Reason = reason
		return row
	}
	return row
}

// isWorkspaceNamespaceClone reports whether a path belongs to the root
// workspace namespace, including its nested group commands.
func isWorkspaceNamespaceClone(path string) bool {
	return strings.HasPrefix(path, "baha workspace") && !strings.HasPrefix(path, "baha app workspace")
}

// canonicalCoveragePath maps a root-namespace workspace command onto its
// canonical app-workspace twin. Both namespaces execute identical shared
// implementations (workspaceNamespaceCommand reuses the app workspace tree),
// so one classification table serves both surfaces; the generated
// machine-coverage reference still lists every command path explicitly.
func canonicalCoveragePath(path string) string {
	if !isWorkspaceNamespaceClone(path) {
		return path
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(path, "baha workspace"))
	if remainder == "" {
		return "baha app workspace"
	}
	return "baha app workspace " + remainder
}

// isWorkspaceNamespaceClone reports whether a path belongs to the root

var cliMachineOperations = map[string]string{
	"baha app edit": "app.edit", "baha app footprint show": "app.footprint.show",
	"baha update": "release.check",
	"baha up":     "control-plane.up", "baha down": "control-plane.stop", "baha destroy": "control-plane.destroy", "baha status": "control-plane.status", "baha doctor": "control-plane.doctor",
	"baha openbao status":    "openbao.status",
	"baha openbao bootstrap": "openbao.bootstrap",
	"baha openbao unseal":    "openbao.unseal",
	"baha openbao rotate":    "openbao.rotate",
	"baha dev domain":        "dev.domain",
	"baha dev credentials":   "dev.credentials",
	"baha app env":           "app.environment", "baha app creds": "app.connection",
	"baha connect": "connectivity.connect", "baha disconnect": "connectivity.disconnect", "baha connections": "connectivity.list",
	"baha whoami": "operator.identity", "baha provider init": "provider.init", "baha provider test": "provider.test", "baha app workspace show": "workspace.show", "baha workspace show": "workspace.show", "baha app show": "app.show",
	"baha app create": "app.create", "baha app init": "app.adopt", "baha init": "app.adopt",
	"baha app runtime-identity rotate": "runtime-identity.rotate", "baha app runtime-identity revoke": "runtime-identity.revoke", "baha app tls update": "tls.update",
	"baha trust status": "trust.status", "baha trust export": "trust.export", "baha trust install": "trust.install", "baha trust uninstall": "trust.uninstall",
	"baha app down": "app.stop", "baha app stop": "app.stop", "baha app prune": "app.prune", "baha app preflight": "app.preflight",
	"baha app perses enable": "app.perses.enable", "baha app perses disable": "app.perses.disable", "baha app perses status": "app.perses.status", "baha app perses remove": "app.perses.remove",
	"baha app secret list": "secret.list", "baha app secret set": "secret.set", "baha app secret delete": "secret.delete", "baha app secret tls-set": "secret.tls-set",
	"baha node add": "node.add", "baha node connect": "node.connect", "baha node list": "node.list", "baha node status": "node.status", "baha node disconnect": "node.disconnect",
	"baha target create": "target.create", "baha target delete": "target.delete",
	"baha stack list": "stack.list", "baha stack show": "stack.show", "baha stack create": "stack.create",
	"baha target": "target", "baha target show": "target", "baha target list": "target.list",
	"baha target policy show": "target",
	"baha list":               "app.list", "baha app list": "app.list", "baha app inspect": "inspect", "baha app new": "app.new", "baha inspect": "inspect", "baha backup": "backup", "baha restore": "restore",
	"baha plan": "plan", "baha app plan": "plan", "baha app apply": "apply", "baha app up": "apply",
	"baha app status": "status", "baha app doctor": "doctor",
	"baha app versions": "app.versions",
	"baha app update":   "update", "baha app evidence": "evidence", "baha app backup": "backup", "baha app restore": "restore", "baha app destroy": "destroy",
	"baha app workspace resolve": "workspace.resolve", "baha workspace resolve": "workspace.resolve",
	"baha app workspace status": "workspace.status", "baha workspace status": "workspace.status", "baha app workspace update": "workspace.update", "baha workspace update": "workspace.update",
	"baha app workspace init": "workspace.init", "baha workspace init": "workspace.init", "baha app workspace map": "workspace.map", "baha workspace map": "workspace.map",
	"baha app workspace path": "workspace.map",

	"baha provider list": "provider.list", "baha provider inspect": "provider.inspect", "baha provider verify": "provider.verify", "baha provider add": "provider.add", "baha provider remove": "provider.remove",
	"baha policy check": "policy.check", "baha policy explain": "policy.explain",
	"baha config organization show": "organization.inspect", "baha config organization check": "organization.check", "baha config organization set": "organization.set", "baha config organization update": "organization.update",
}

var cliPresentationCommands = map[string]string{
	"baha config prompt":                  "Client-local shell prompt display preferences; explicit target arguments and target inspection provide semantic context.",
	"baha app workspace":                  "Interactive presentation combining workspace.init and workspace.map; clients invoke those typed operations explicitly.",
	"baha app workspace component":        "Source-model component maintenance group; presentation over the portable source model that workspace.init/map/show/resolve read and write.",
	"baha app workspace component add":    "Edits the versioned source model through the mutating workspace.init contract.",
	"baha app workspace component list":   "Reads the versioned source model through the workspace.show contract.",
	"baha app workspace component remove": "Edits the versioned source model through the mutating workspace.init contract.",
	"baha app workspace source":           "Source-model maintenance group; presentation over the portable source model that workspace.init reads and writes.",
	"baha app workspace source add":       "Edits the versioned source model through the mutating workspace.init contract.",
	"baha app workspace source remove":    "Detaches source, mapping and component entries through the mutating workspace.init contract; no local file deletion.",
	"baha app workspace path":             "Sets or reads a local checkout path in the versioned source model; writes require workspace.map authorization and never create or delete files.",
	"baha workspace component":            "Alias for the source-model component maintenance group; writes require workspace.init and reads require workspace.show.",
	"baha workspace component add":        "Alias for component add; edits the source model through workspace.init.",
	"baha workspace component list":       "Alias for component list; reads the source model through workspace.show.",
	"baha workspace component remove":     "Alias for component remove; edits the source model through workspace.init.",
	"baha workspace source":               "Alias for the source-model maintenance group; writes require workspace.init.",
	"baha workspace source add":           "Alias for source add; edits the source model through workspace.init.",
	"baha workspace source remove":        "Alias for source remove; detaches model references without deleting local files.",
	"baha workspace path":                 "Alias for setting or reading a local checkout path; writes require workspace.map and never create or delete files.",
	"baha app secure-profile export":      "Operator-local, password-locked file export of opt-in local credentials; no runtime mutation or machine tool. Ciphertext opens only with the export password.",
	"baha app export":                     "Operator-local, password-locked file export of explicitly selected local credentials; no runtime mutation or machine tool, and fails closed on an empty selection. Ciphertext opens only with the export password; managed-secret sources require the human-authorization reveal seam.",
	"baha workspace":                      "Canonical interactive presentation of the same workspace.init/map operations.",
	"baha target activate":                "Persists local user selection; machine clients supply explicit target arguments on each semantic operation.",
	"baha target policy set":              "Owner-local protected Target configuration edit with CAS; no remote operator policy mutation is exposed yet. Remote targets refuse this action; machine clients must not bypass the owning Core authority.",
	"baha use":                            "Persists local user selection; machine clients supply explicit target arguments on each semantic operation.",
	"baha target deactivate":              "Clears persisted local user selection; machine clients omit explicit target to resolve configured defaults.",
	"baha":                                "Root help and routing; use semantic operations for product actions.",
	"baha agent describe":                 "Machine registry/discovery itself, rather than a product mutation.",
	"baha mcp serve":                      "Starts the local MCP transport; clients launch it before discovery.",
	"baha completion":                     "Shell completion emits client-local shell text; semantic discovery is agent describe.",
	"baha shell-init":                     "Shell-local helpers; explicit Target selection remains product functionality.",
	"baha context":                        "Command group for the per-terminal context surface; invoke its use/clear/show subcommands.",
	"baha context use":                    "Emits the shell snippet that sets BASEHARBOR_TARGET for the current shell process only; no on-disk state. The per-terminal value lives in the shell-process environment (carried via the completion hook); machine clients resolve the same precedence through explicit --target, the BASEHARBOR_TARGET env, or the configured default target.",
	"baha context clear":                  "Process-level unset of the per-terminal BASEHARBOR_TARGET plus a hint; nothing is persisted, so machine clients simply omit --target to resolve configured defaults.",
	"baha context show":                   "Repository-bound presentation of the effective per-terminal selection and the winning precedence level (explicit flag, shell-env, or configured default); machine clients read the same decision through their own explicit-argument resolution.",
	"baha prompt":                         "Displays shell-local context; target inspection is the semantic equivalent.",
	"baha version":                        "Executable build identity is provided by agent describe and MCP initialization.",
	"baha open":                           "Host browser presentation over validated application/TLS state; typed clients inspect app status and TLS instead.",
	"baha new":                            "Human creation chooser delegating to existing app.new, stack.create, target.create, provider.init and workspace operations.",
	"baha onboard":                        "Guided workstation onboarding presentation; the underlying installation, target and enrollment operations retain their own authenticated semantic contracts.",
	"baha user":                           "Human directory and lifecycle commands for Core-managed people.",
	"baha user list":                      "Core-local human directory view; authentication state remains authoritative in the owned Keycloak realm.",
	"baha workstation":                    "Human inventory and lifecycle commands for Core-managed workstation certificates.",
	"baha workstation list":               "Core-local inventory of workstation certificate ownership, expiry and revocation state.",
	"baha tui":                            "Human interactive presentation; its product actions require individual semantic coverage.",
	"baha app version":                    "Repository-bound presentation of the app.versions read model for the current deployment; machine clients invoke app.versions with explicit identity.",
	"baha env":                            "Command group routing to env.show and env.destroy; invoke the supported subcommands explicitly.",
	"baha env destroy":                    "Environment-scoped identity removal with a CLI JSON impact plan; no machine tool yet (pending semantic MCP coverage). The --json surface exposes the plan.",
	// The optional Perses lifecycle surface (0428-084/#1604) is guided human
	// presentation: enable and disable fold into the manifest services.observability
	// selection and the apply/preflight flow, status is a repository-bound read of
	// the same three-state contract, and remove is a manifest convergence plus an
	// ownership-safe plan (runtime retirement lands in the release window). None
	// exposes a separate typed MCP operation.
	"baha app perses":         "Command group for the optional Perses management surface; invoke its guided lifecycle subcommands.",
	"baha app perses enable":  "Guided human enablement of the optional Perses surface; it folds into the manifest services.observability selection and the apply/preflight flow, which carry their own typed coverage, so no separate MCP operation is exposed.",
	"baha app perses disable": "Guided human opt-out of the optional Perses surface; it folds into the manifest services.observability selection and the apply/preflight flow, which carry their own typed coverage, so no separate MCP operation is exposed.",
	"baha app perses status":  "Repository-bound presentation of the app.perses.status read model for the current deployment; machine clients inspect app status instead.",
	"baha app perses remove":  "Guided human retirement of the owned Perses surface; manifest convergence plus an ownership-safe removal plan whose runtime retirement lands in the release window; no separate MCP operation is exposed.",
}

// These modes are outside the semantic tool boundary by design. Business
// metadata and lifecycle remain available through the named typed alternatives.
var cliExcludedCommands = map[string]string{
	"baha dev hosts":          "Workstation-local hosts-file opt-in and ownership cleanup; cannot mutate the MCP server host on behalf of a remote client. Names come from authenticated control-plane.status metadata.",
	"baha dev cleanup":        "Workstation-local removal of owned hosts and CA trust, preserving other connections; explicit host operator approval required. No Core or application mutation.",
	"baha app exec":           "Arbitrary process execution is intentionally excluded: #787 forbids generic exec passthrough; use typed lifecycle and observation tools. The CLI path is guided (component from the contract, --cmd one command, exit status preserved) but never becomes a machine tool.",
	"baha app shell":          "Interactive container terminal is host/TTY dependent and would grant arbitrary exec; use typed lifecycle and app.environment instead. It is the shell default of the guided app.exec path.",
	"baha app sql":            "Interactive database terminal and raw SQL passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client.",
	"baha app cache":          "Interactive database terminal and arbitrary command passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client.",
	"baha app logs":           "Raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation. The CLI read path is guided over the same read view (deployment log policy plus contract components); JSON output carries only bounded results, never a followed stream.",
	"baha logs":               "Top-level shorthand for the excluded baha app logs: raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation.",
	"baha app urls":           "Lists only the verified URLs and bound-service GUI surfaces of the status read model; typed clients inspect app.status and TLS state instead. Opening a browser remains the excluded human seam 'baha open'.",
	"baha login":              "Authorization Code/PKCE browser handoff establishes the operator's local protected session before semantic requests; operator.identity verifies that boundary without returning tokens.",
	"baha logout":             "Removes a client-local protected login session; performed by the authentication client, outside server product actions. operator.identity reports the resulting boundary.",
	"baha serve":              "Starts a long-lived TLS API/broker transport using host-held credentials; transport supervision belongs to the host, typed lifecycle tools operate managed resources.",
	"baha update":             "Self-replacement of the executing host binary and recovery copy belongs to the host operator; release.check provides the same bounded release inspection.",
	"baha access assign":      "Guided, human interactive profile assignment folds every decision into the single production policy authority (policy.Aggregate/policy.ResolveFinding); no typed MCP operation, so excluded from the machine tool boundary by design.",
	"baha access offboard":    "Guided, human interactive offboarding re-checks the current configuration at submit time and folds every decision into the single production policy authority; no typed MCP operation, so excluded from the machine tool boundary by design.",
	"baha user offboard":      "Human-confirmed operator directory action disables the owned Keycloak user and revokes its workstation leaves; no typed MCP operation, so excluded from the machine tool boundary by design.",
	"baha workstation new":    "Human invitation workflow for a new device; private one-use invitation transfer is an operator action and has no machine equivalent.",
	"baha workstation revoke": "Human-confirmed device revocation; no typed MCP operation, so excluded from the machine tool boundary by design.",
}

var cliSemanticModes = map[string]string{
	"baha app perses enable":  "Sets services.perses in the repository manifest through the shared lifecycle seam; runtime converges with the apply operation.",
	"baha app perses disable": "Clears services.perses in the repository manifest through the shared lifecycle seam; the backend observability owner is preserved.",
	"baha app perses status":  "Reads the public three-state contract for the optional surface from manifest truth; never a health probe.",
	"baha app perses remove":  "Retires only BaseHarbor-owned Perses runtime resources and disables the surface; foreign backends are never removed.",
	"baha update":             "--check maps to release.check. Installing replaces the execution-host binary and is explicitly excluded from MCP; use the host operator CLI with --yes.",
	"baha up":                 "Control-plane-only mode maps here. Repository application mode additionally uses app.configure and apply; host trust installation uses trust.install with approval.",
	"baha down":               "Control-plane mode maps here; repository application mode uses app.stop.",
	"baha destroy":            "Always targets complete installation destruction regardless of repository; use baha app destroy for one application and baha down to stop Core services. Authorizes every discovered deployment. All destruction retains ownership checks and explicit approval.",
	"baha status":             "Control-plane mode maps here; repository application mode uses status.",
	"baha doctor":             "Control-plane mode maps here; --fix uses control-plane.repair and rechecks readiness. Repository mode uses doctor or repair.",
	"baha app init":           "New repository intent uses app.adopt; an existing repository uses app.configure. Interactive suggestions are presentation only.",
	"baha app env":            "MCP exposes masked values only. --reveal and loading the protected local dotenv file are execution-host credential access and are explicitly excluded modes.",
	"baha app creds":          "MCP exposes connection metadata without passwords or credential-bearing URIs; CLI credential reveal is operator-local and excluded from MCP.",
	"baha app prune":          "Registry-only removal of a dangling deployment record whose source repository is gone; never loads source or runtime, and refuses when source is present or multiple records match.",
	"baha dev credentials":    "Ensure/reset/configuration use the shared authority. MCP returns username and protected file reference; CLI password display is an excluded operator-local mode.",
}
