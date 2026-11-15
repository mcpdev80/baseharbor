package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// targetEnvCommand exposes the environment UID rename as a real mutation on the
// production CLI. `baha target env rename OLD NEW` drives RenameEnvironment
// through the actual command path (runWithIO), so the rename is a persisted
// metadata update that keeps the durable UID — not a read-only seam exercised
// only from a unit fixture. This is the finding-2 correction: the rename
// regression must run the RENAME MUTATION through a real CLI entry, not just
// reads.
func targetEnvCommand() *cli.Command {
	return &cli.Command{
		Name:    "env",
		Summary: "Inspect and rename deployment-target environments",
		Usage:   "baha target env [show|rename OLD NEW]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) == 0 {
				return usageError("baha target env requires a subcommand (show|rename)", "Run 'baha target env show' or 'baha target env rename OLD NEW'.")
			}
			switch args[0] {
			case "show":
				return targetEnvShowCommand(ctx, args[1:], out, errOut)
			case "rename":
				return targetEnvRenameCommand(ctx, args[1:], out, errOut)
			case "--help", "-h":
				fmt.Fprintln(out, "Usage: baha target env [show|rename OLD NEW]")
				fmt.Fprintln(out, "  show        Resolve and report the durable UID of the selected environment.")
				fmt.Fprintln(out, "  rename      Persistently rename an environment, keeping its durable UID.")
				return nil
			default:
				return usageError("unknown 'target env' subcommand "+args[0], "Use 'show' or 'rename OLD NEW'.")
			}
		},
	}
}

func targetEnvShowCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, _, err := parseReadOutputArgs(args, "target env show")
	if err != nil {
		return err
	}
	if len(filtered) != 0 {
		return usageError("baha target env show does not accept positional arguments", "Run 'baha target env show'.")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	selection, err := selectedTargetName(strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), cfg)
	if err != nil {
		return err
	}
	target, err := cfg.ResolveTarget(selection, "")
	if err != nil {
		return err
	}
	envName := strings.TrimSpace(os.Getenv("BASEHARBOR_ENVIRONMENT"))
	if envName == "" {
		return machine.NewError(machine.ErrorConflict,
			"no environment selected: set BASEHARBOR_ENVIRONMENT",
			"Run 'baha target env show' with BASEHARBOR_ENVIRONMENT set.", false)
	}
	record, gErr := deployment.GetEnvironment(target.Name, envName)
	if gErr != nil {
		return gErr
	}
	return writeJSON(out, targetEnvShowResult{ContractVersion: machine.ContractVersion, Target: target.Name, EnvironmentID: record.Identity, EnvironmentName: record.Name})
}

func targetEnvRenameCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, _, err := parseReadOutputArgs(args, "target env rename")
	if err != nil {
		return err
	}
	if len(filtered) != 2 {
		return usageError("baha target env rename requires OLD and NEW names", "Run 'baha target env rename OLD NEW'.")
	}
	oldName := strings.TrimSpace(filtered[0])
	newName := strings.TrimSpace(filtered[1])
	if oldName == "" || newName == "" {
		return usageError("baha target env rename requires non-empty OLD and NEW names", "Run 'baha target env rename OLD NEW'.")
	}
	if oldName == newName {
		return usageError("baha target env rename requires a different NEW name", "OLD and NEW must differ.")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	selection, err := selectedTargetName(strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), cfg)
	if err != nil {
		return err
	}
	target, err := cfg.ResolveTarget(selection, "")
	if err != nil {
		return err
	}
	// The RENAME MUTATION runs through the production RenameEnvironment seam.
	// This keeps the durable UID and only rewrites the Name (a metadata update).
	record, rErr := deployment.RenameEnvironment(target.Name, oldName, newName)
	if rErr != nil {
		return rErr
	}
	return writeJSON(out, targetEnvShowResult{ContractVersion: machine.ContractVersion, Target: target.Name, EnvironmentID: record.Identity, EnvironmentName: record.Name})
}

type targetEnvShowResult struct {
	ContractVersion string `json:"contract_version"`
	Target          string `json:"target"`
	EnvironmentID   string `json:"environment_id"`
	EnvironmentName string `json:"environment_name"`
}
