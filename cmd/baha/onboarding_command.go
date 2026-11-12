package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/onboardingwizard"
)

// onboardCommand implements the guided flow. The command is public and registered in the CLI tree.

func onboardCommand() *cli.Command {
	return &cli.Command{
		Name:    "onboard",
		Summary: "Guided onboarding: attach a workstation to a BaseHarbor installation",
		Usage:   "baha onboard [--target NAME] [--json] | baha onboard --import PRIVATE_FILE [--target-name NAME] [--trust-host-ca|--skip-host-trust] [--configure-hosts|--skip-hosts] | baha onboard --invite [--target NAME] [--core-url HTTPS_URL] [--station LABEL] [--output PRIVATE_FILE] | baha onboard --init-tenant [-e ENV] [--tenant-slug SLUG --tenant-name NAME]",
		Long:    "Guided path from a fresh workstation to a functioning installation with minimal questions. Bootstrap and attach are distinct intents; joining an existing installation never re-installs services and never repeats credential entry while a valid reusable connection exists. Cancelling guided setup before applying its plan changes nothing. An imported connection stays saved if optional system/browser CA trust is declined or fails.",
		Run:     runOnboardCommand,
	}
}

func runOnboardCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) > 0 && args[0] == "--invite" {
		return runOnboardInvite(ctx, args[1:], out)
	}
	if len(args) > 0 && args[0] == "--import" {
		return runOnboardImport(ctx, args[1:], out)
	}
	// First-owner setup reuses the exact same owner-bound OIDC + PKCE + tenant
	// transaction as baha login; the wizard never creates an independent admin.
	if len(args) > 0 && args[0] == "--init-tenant" {
		return runOnboardInitialTenant(ctx, args[1:], out, errOut)
	}
	// On the owning VM a bare "baha onboard" exports the one-use
	// workstation invitation. Do not send a READY Core owner back through
	// the client-side attach wizard or demand a browser login.
	if len(args) == 0 {
		if target, err := effectiveTarget(ctx); err == nil &&
			(target.AccessProvider == "" || target.AccessProvider == "local") {
			// Existing local onboarding sessions must retain their guided
			// manage/remove path. Only a fresh owning Core defaults to an
			// invitation instead of the attach wizard.
			if _, found, err := loadOnboardingConnection(target.Name); err != nil {
				return err
			} else if !found {
				if root, err := targetRuntimeStateRoot(target); err == nil {
					if state, err := coreinstallation.Load(root); err == nil && state.Ready {
						return runOnboardInvite(ctx, nil, out)
					}
				}
			}
		}
	}
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

func runOnboardInitialTenant(ctx context.Context, args []string, out, errOut io.Writer) error {
	environment := "dev"
	slug, name := "baseharbor", "BaseHarbor"
	advanced := false
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return usageError(args[i]+" requires a value", "Use --environment, --tenant-slug or --tenant-name with a value.")
		}
		flag, value := args[i], strings.TrimSpace(args[i+1])
		i++
		switch flag {
		case "-e", "--environment":
			environment = value
		case "--tenant-slug":
			slug, advanced = value, true
		case "--tenant-name":
			name, advanced = value, true
		default:
			return unknownOptionUsage("baha onboard --init-tenant", flag, "-e", "--environment", "--tenant-slug", "--tenant-name")
		}
	}
	if strings.TrimSpace(environment) == "" {
		return usageError("operator environment cannot be empty", "Select the existing authentication environment.")
	}
	// A normal interactive installation asks only one decision; advanced
	// settings remain optional. The user must still complete a real OIDC login.
	if !advanced && !noInput(ctx) && readerIsTerminal(onboardingInput) {
		reader := bufio.NewReader(onboardingInput)
		fmt.Fprintln(out, "Initial Core operator and tenant setup:")
		fmt.Fprintln(out, "  1. Recommended: managed Keycloak, tenant baseharbor")
		fmt.Fprintln(out, "  2. Customize: choose tenant name and slug (OIDC provider follows)")
		selection, err := promptLine(reader, out, "Selection", "1")
		if err != nil {
			return err
		}
		switch selection {
		case "1", "":
		case "2":
			slug, err = promptLine(reader, out, "Tenant slug", "baseharbor")
			if err != nil {
				return err
			}
			name, err = promptLine(reader, out, "Tenant name", "BaseHarbor")
			if err != nil {
				return err
			}
		default:
			return usageError("unsupported initial tenant setup choice", "Choose 1 or 2.")
		}
	}
	loginArgs := []string{"-e", environment, "--init-tenant", "--tenant-slug", slug, "--tenant-name", name}
	return operatorLoginCommand().Run(ctx, loginArgs, out, errOut)
}
