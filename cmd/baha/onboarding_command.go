package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/onboardingwizard"
)

// onboardCommand implements the guided flow. The command is public and registered in the CLI tree.

func onboardCommand() *cli.Command {
	return &cli.Command{
		Name:    "onboard",
		Summary: "Guided onboarding: attach a workstation to a BaseHarbor installation",
		Usage:   "baha onboard [--target NAME] [--json]",
		Long:    "Guided path from a fresh workstation to a functioning installation with minimal questions. Bootstrap and attach are distinct intents; joining an existing installation never re-installs services and never repeats credential entry while a valid reusable connection exists. Cancel and EOF change nothing.",
		Run:     runOnboardCommand,
	}
}

func runOnboardCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "onboard")
	if err != nil {
		return err
	}
	target := ""
	forced := false
	for i := 0; i < len(filtered); i++ {
		if filtered[i] == "--target" {
			if i+1 >= len(filtered) {
				return usageError("--target requires a value", "Run baha onboard --target NAME.")
			}
			target = strings.TrimSpace(filtered[i+1])
			forced = true
			i++
			continue
		}
		if strings.HasPrefix(filtered[i], "-") {
			return unknownOptionUsage("baha onboard", filtered[i], "--target", "--json")
		}
		return usageError("baha onboard accepts at most one --target", "Run 'baha onboard --help'.")
	}
	if noInput(ctx) {
		if target == "" {
			return usageError("onboard requires --target in non-interactive mode", "Run baha onboard --target NAME, or run it in a terminal for the guided path.")
		}
		if trustedLocalConnection(ctx, target) {
			// A repeat operation with valid reusable access continues
			// silently and persists nothing again.
			return finishOnboardOutput(out, format, onboardingwizard.Decision{Mode: onboardingwizard.ModeAttach, Target: target})
		}
		if err := registerOnboardingConnection(ctx, target); err != nil {
			return err
		}
		return finishOnboardOutput(out, format, onboardingwizard.Decision{Mode: onboardingwizard.ModeAttach, Target: target})
	}
	// An explicit --target is an intentional machine-style attach: the
	// wizard preselects the Target and the command executes the verified
	// effect path directly (verify-then-persist). A repeat with valid
	// reusable access persists nothing again. The human guided flow runs
	// only without an explicit Target.
	if forced {
		if trustedLocalConnection(ctx, target) {
			return finishOnboardOutput(out, format, onboardingwizard.Decision{Mode: onboardingwizard.ModeAttach, Target: target})
		}
		if err := registerOnboardingConnection(ctx, target); err != nil {
			return err
		}
		return finishOnboardOutput(out, format, onboardingwizard.Decision{Mode: onboardingwizard.ModeAttach, Target: target})
	}
	decision, err := onboardingwizard.Run(ctx, onboardingwizard.Input{
		In:       onboardingInput,
		Out:      out,
		Discover: onboardingAdapters.Discover,
	})
	if err != nil {
		if errors.Is(err, onboardingwizard.ErrCancel) {
			fmt.Fprintln(out, "Cancelled. No changes were made.")
			return nil
		}
		return err
	}
	if err := commitOnboardingDecision(ctx, decision, out); err != nil {
		return err
	}
	return finishOnboardOutput(out, format, decision)
}

func finishOnboardOutput(out io.Writer, format cliOutputFormat, decision onboardingwizard.Decision) error {
	if format != outputJSON {
		return nil
	}
	return writeJSON(out, map[string]any{
		"contract_version": machine.ContractVersion,
		"mode":             string(decision.Mode),
		"target":           decision.Target,
		"app":              string(decision.App),
		"persistence":      string(decision.Persistence),
	})
}
