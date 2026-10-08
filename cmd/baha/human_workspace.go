package main

import (
	"github.com/mcpdev80/baseharbor/internal/cli"
	"strings"
)

// workspace is the canonical advanced developer namespace. It reuses the
// application workspace operations rather than creating a second domain model.
func workspaceNamespaceCommand() *cli.Command {
	command := appWorkspaceCommand()
	command.Name = "workspace"
	command.Summary = "Configure local workspace sources and repository mappings"
	normalizeWorkspaceHelp(command)
	return command
}

func normalizeWorkspaceHelp(command *cli.Command) {
	command.Usage = strings.ReplaceAll(command.Usage, "baha app workspace", "baha workspace")
	for _, child := range command.Children {
		normalizeWorkspaceHelp(child)
	}
}
