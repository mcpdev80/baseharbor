package main

import (
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

// workspace is the canonical advanced developer namespace. It reuses the
// application workspace operations rather than creating a second domain model
// (card 0428-059): flat leaf commands share the registered Run functions, and
// the source/component maintenance groups resolve through the same canonical
// workspace command tree.
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
