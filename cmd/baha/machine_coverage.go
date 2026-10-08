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
	if operationID, ok := cliMachineOperations[path]; ok {
		if operation, exists := machine.OperationByID(operationID); exists {
			row.Classification = "semantic"
			row.Operation = operation.ID
			row.MCPTool = operation.MCPTool
			row.Reason = cliSemanticModes[path]
			row.JSONSurface = "CLI -o json / typed MCP result"
			switch path {
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
	}
	return row
}

var cliMachineOperations = map[string]string{
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
	"baha whoami": "operator.identity", "baha provider init": "provider.init", "baha provider test": "provider.test", "baha app workspace show": "workspace.show", "baha app show": "app.show",
	"baha app create": "app.create", "baha app init": "app.adopt", "baha init": "app.adopt",
	"baha app runtime-identity rotate": "runtime-identity.rotate", "baha app runtime-identity revoke": "runtime-identity.revoke", "baha app tls update": "tls.update",
	"baha trust status": "trust.status", "baha trust export": "trust.export", "baha trust install": "trust.install",
	"baha app down": "app.stop", "baha app preflight": "app.preflight",
	"baha app secret list": "secret.list", "baha app secret set": "secret.set", "baha app secret delete": "secret.delete", "baha app secret tls-set": "secret.tls-set",
	"baha target create": "target.create", "baha target delete": "target.delete",
	"baha stack list": "stack.list", "baha stack show": "stack.show", "baha stack create": "stack.create",
	"baha target": "target", "baha target show": "target", "baha target list": "target.list",
	"baha list": "app.list", "baha app list": "app.list", "baha app inspect": "inspect", "baha app new": "app.new", "baha inspect": "inspect", "baha backup": "backup", "baha restore": "restore",
	"baha plan": "plan", "baha app plan": "plan", "baha app apply": "apply", "baha app up": "apply",
	"baha app status": "status", "baha app doctor": "doctor",
	"baha app update": "update", "baha app evidence": "evidence", "baha app backup": "backup", "baha app restore": "restore", "baha app destroy": "destroy",
	"baha app workspace resolve": "workspace.resolve",
	"baha app workspace status":  "workspace.status", "baha app workspace update": "workspace.update",
	"baha app workspace init": "workspace.init", "baha app workspace map": "workspace.map",
	"baha provider list": "provider.list", "baha provider inspect": "provider.inspect", "baha provider verify": "provider.verify", "baha provider add": "provider.add", "baha provider remove": "provider.remove",
	"baha policy check": "policy.check", "baha policy explain": "policy.explain",
	"baha config organization show": "organization.inspect", "baha config organization check": "organization.check", "baha config organization set": "organization.set", "baha config organization update": "organization.update",
}

var cliPresentationCommands = map[string]string{
	"baha config prompt":     "Client-local shell prompt display preferences; explicit target arguments and target inspection provide semantic context.",
	"baha app workspace":     "Interactive presentation combining workspace.init and workspace.map; clients invoke those typed operations explicitly.",
	"baha target activate":   "Persists local user selection; machine clients supply explicit target arguments on each semantic operation.",
	"baha target deactivate": "Clears persisted local user selection; machine clients omit explicit target to resolve configured defaults.",
	"baha":                   "Root help and routing; use semantic operations for product actions.",
	"baha agent describe":    "Machine registry/discovery itself, rather than a product mutation.",
	"baha mcp serve":         "Starts the local MCP transport; clients launch it before discovery.",
	"baha completion":        "Shell completion emits client-local shell text; semantic discovery is agent describe.",
	"baha shell-init":        "Shell-local helpers; explicit Target selection remains product functionality.",
	"baha prompt":            "Displays shell-local context; target inspection is the semantic equivalent.",
	"baha version":           "Executable build identity is provided by agent describe and MCP initialization.",
	"baha open":             "Host browser presentation over validated application/TLS state; typed clients inspect app status and TLS instead.",
	"baha new":              "Human creation chooser delegating to existing app.new, stack.create, target.create, provider.init and workspace operations.",
	"baha tui":               "Human interactive presentation; its product actions require individual semantic coverage.",
}

// These modes are outside the semantic tool boundary by design. Business
// metadata and lifecycle remain available through the named typed alternatives.
var cliExcludedCommands = map[string]string{
	"baha app exec":  "Arbitrary process execution is intentionally excluded: #787 forbids generic exec passthrough; use typed lifecycle and observation tools.",
	"baha app shell": "Interactive container terminal is host/TTY dependent and would grant arbitrary exec; use typed lifecycle and app.environment instead.",
	"baha app sql":  "Interactive database terminal and raw SQL passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client.",
	"baha app cache": "Interactive database terminal and arbitrary command passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client.",
	"baha app logs":  "Raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation.",
	"baha login":     "Authorization Code/PKCE browser handoff establishes the operator's local protected session before semantic requests; operator.identity verifies that boundary without returning tokens.",
	"baha logout":    "Removes a client-local protected login session; performed by the authentication client, outside server product actions. operator.identity reports the resulting boundary.",
	"baha serve":     "Starts a long-lived TLS API/broker transport using host-held credentials; transport supervision belongs to the host, typed lifecycle tools operate managed resources.",
	"baha update":    "Self-replacement of the executing host binary and recovery copy belongs to the host operator; release.check provides the same bounded release inspection.",
}

var cliSemanticModes = map[string]string{
	"baha update":          "--check maps to release.check. Installing replaces the execution-host binary and is explicitly excluded from MCP; use the host operator CLI with --yes.",
	"baha up":              "Control-plane-only mode maps here. Repository application mode additionally uses app.configure and apply; host trust installation uses trust.install with approval.",
	"baha down":            "Control-plane mode maps here; repository application mode uses app.stop.",
	"baha destroy":         "Control-plane mode maps here; repository application mode uses destroy. --all maps to installation.destroy after authorization of every discovered deployment. All destruction retains ownership checks and explicit approval.",
	"baha status":          "Control-plane mode maps here; repository application mode uses status.",
	"baha doctor":          "Control-plane mode maps here; --fix uses control-plane.repair and rechecks readiness. Repository mode uses doctor or repair.",
	"baha app init":        "New repository intent uses app.adopt; an existing repository uses app.configure. Interactive suggestions are presentation only.",
	"baha app env":         "MCP exposes masked values only. --reveal and loading the protected local dotenv file are execution-host credential access and are explicitly excluded modes.",
	"baha app creds":       "MCP exposes connection metadata without passwords or credential-bearing URIs; CLI credential reveal is operator-local and excluded from MCP.",
	"baha dev credentials": "Ensure/reset/configuration use the shared authority. MCP returns username and protected file reference; CLI password display is an excluded operator-local mode.",
}
