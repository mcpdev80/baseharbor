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
			row.Reason = ""
		}
		return row
	}
	if reason, ok := cliPresentationCommands[path]; ok {
		row.Classification = "presentation"
		row.Reason = reason
	}
	return row
}

var cliMachineOperations = map[string]string{
	"baha target": "target", "baha target show": "target", "baha target list": "target.list",
	"baha app list": "app.list", "baha app inspect": "inspect", "baha app new": "app.new",
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
	"baha":                "Root help and routing; use semantic operations for product actions.",
	"baha init":           "Explains initialization paths; app init and target create remain explicit product coverage gaps.",
	"baha agent describe": "Machine registry/discovery itself, rather than a product mutation.",
	"baha mcp serve":      "Starts the local MCP transport; clients launch it before discovery.",
	"baha completion":     "Shell completion emits client-local shell text; semantic discovery is agent describe.",
	"baha shell-init":     "Shell-local helpers; explicit Target selection remains product functionality.",
	"baha prompt":         "Displays shell-local context; target inspection is the semantic equivalent.",
	"baha version":        "Executable build identity is provided by agent describe and MCP initialization.",
	"baha tui":            "Human interactive presentation; its product actions require individual semantic coverage.",
}
