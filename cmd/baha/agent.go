package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type agentDescription struct {
	ContractVersion   string              `json:"contract_version"`
	BaseHarborVersion string              `json:"baseharbor_version"`
	Operations        []machine.Operation `json:"operations"`
	Capabilities      []string            `json:"capability_specifications"`
	MCP               agentMCPDescription `json:"mcp"`
}

type agentMCPDescription struct {
	Transport       string   `json:"transport"`
	ProtocolVersion string   `json:"protocol_version"`
	Tools           []string `json:"tools"`
	Remote          bool     `json:"remote"`
}

func agentCommand() *cli.Command {
	return &cli.Command{
		Name:    "agent",
		Summary: "Discover the stable BaseHarbor machine interface",
		Usage:   "baha agent describe [-o json|--output json]",
		Long:    "Exposes the versioned semantic operations that coding agents and automation may use safely. Discovery describes BaseHarbor operations; it does not expose shell, Docker or Compose execution.",
		Children: []*cli.Command{
			{
				Name:    "describe",
				Summary: "Describe supported machine operations and contracts",
				Usage:   "baha agent describe [-o json|--output json]",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					filtered, format, err := parseReadOutputArgs(args, "agent describe")
					if err != nil {
						return err
					}
					if len(filtered) != 0 {
						return usageError("baha agent describe does not accept positional arguments", "Run 'baha agent describe -o json'.")
					}
					description := currentAgentDescription()
					if format == outputJSON {
						return writeJSON(out, description)
					}
					fmt.Fprintf(out, "BaseHarbor machine contract: %s\n", description.ContractVersion)
					fmt.Fprintf(out, "BaseHarbor version: %s\n", description.BaseHarborVersion)
					fmt.Fprintf(out, "MCP: %s · protocol %s · local only\n", description.MCP.Transport, description.MCP.ProtocolVersion)
					fmt.Fprintln(out, "\nOperations:")
					for _, operation := range description.Operations {
						fmt.Fprintf(out, "  %-8s %-12s %s\n", operation.ID, operation.Safety, operation.Description)
					}
					fmt.Fprintln(out, "\nMCP tools:")
					for _, tool := range description.MCP.Tools {
						fmt.Fprintf(out, "  %s\n", tool)
					}
					return nil
				},
			},
		},
	}
}

func currentAgentDescription() agentDescription {
	return agentDescription{
		ContractVersion:   machine.ContractVersion,
		BaseHarborVersion: version,
		Operations:        machine.Operations(),
		Capabilities:      machine.CapabilitySpecifications(),
		MCP: agentMCPDescription{
			Transport:       "stdio",
			ProtocolVersion: "2026-07-28",
			Tools: []string{
				"baseharbor.inspect",
				"baseharbor.plan",
				"baseharbor.status",
				"baseharbor.doctor",
			},
			Remote: false,
		},
	}
}

func resolveMachineApplication(store application.Store, name string, command string) (resolvedApplication, error) {
	if name == "" {
		return resolveApplication(store, nil, command)
	}
	return resolveApplication(store, []string{name}, command)
}
