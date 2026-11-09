package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// targetPolicyCommand edits only the protected Target policy. Runtime, tenant,
// access binding, application data and Core installation identity are unchanged.
func targetPolicyCommand() *cli.Command {
	return &cli.Command{
		Name:    "policy",
		Summary: "Inspect or change editable Target deployment policy",
		Usage:   "baha target policy <show|set> NAME",
		Children: []*cli.Command{
			{
				Name:    "show",
				Summary: "Show the persisted deployment policy of a Target",
				Usage:   "baha target policy show NAME",
				Run:     showTargetDeploymentPolicy,
			},
			{
				Name:    "set",
				Summary: "Update the protected deployment policy without changing Runtime or Access",
				Usage:   "baha target policy set NAME [--deployment-preset development|test|staging|production|custom|none] [--deployment-branches BRANCHES] [--deployment-dirty yes|no] [--deployment-commit yes|no] [--deployment-remote-commit yes|no] [--deployment-approval yes|no] [--deployment-image-digest yes|no]",
				Run:     setTargetDeploymentPolicy,
			},
		},
	}
}

func showTargetDeploymentPolicy(ctx context.Context, args []string, out, _ io.Writer) error {
	if len(args) != 1 {
		return usageError("target policy show requires NAME", "Example: baha target policy show docker-dev")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	target, exists := cfg.Targets[args[0]]
	if !exists {
		return machine.NewError(machine.ErrorNotFound, "Target is not configured.", "Run baha target list.", false)
	}
	if target.DeploymentPolicy == nil {
		_, err = fmt.Fprintf(out, "Target %s has no configured deployment policy (legacy defaults).\n", args[0])
		return err
	}
	return writeJSON(out, target.DeploymentPolicy)
}

func setTargetDeploymentPolicy(ctx context.Context, args []string, out, _ io.Writer) error {
	if len(args) < 3 {
		return usageError("target policy set requires NAME and an explicit policy option", "Use --deployment-preset or an individual --deployment-* override.")
	}
	name := strings.TrimSpace(args[0])
	if err := deployment.ValidateTargetName(name); err != nil {
		return err
	}
	var preset, branches string
	flags := map[string]string{}
	for i := 1; i < len(args); i++ {
		option := args[i]
		switch option {
		case "--deployment-preset", "--deployment-branches",
			"--deployment-dirty", "--deployment-commit",
			"--deployment-remote-commit", "--deployment-approval",
			"--deployment-image-digest":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return usageError(option+" requires a value", "Run baha target policy set --help.")
			}
			value := strings.TrimSpace(args[i+1])
			i++
			switch option {
			case "--deployment-preset":
				preset = value
			case "--deployment-branches":
				branches = value
			default:
				flags[option] = value
			}
		default:
			return unknownOptionUsage("baha target policy set", option,
				"--deployment-preset", "--deployment-branches",
				"--deployment-dirty", "--deployment-commit",
				"--deployment-remote-commit", "--deployment-approval",
				"--deployment-image-digest")
		}
	}
	if err := authorizeCurrentMCPContext(ctx, "target.create", name, "", ""); err != nil {
		return err
	}
	cfg, expected, err := deployment.LoadConfigForMutation()
	if err != nil {
		return err
	}
	current, exists := cfg.Targets[name]
	if !exists {
		return machine.NewError(machine.ErrorNotFound, "Target is not configured.", "Run baha target list.", false)
	}
	access, exists := cfg.Access[current.Access.Reference]
	if !exists || access.Provider != "local" {
		return machine.NewError(machine.ErrorUnsupported,
			"Editing a remote Target policy requires the owning Core operator mutation endpoint.",
			"Use an authenticated Core operator session; no workstation-local policy substitution is permitted.", false)
	}
	var policy *deployment.TargetDeploymentPolicy
	switch {
	case strings.EqualFold(preset, "none"):
		if branches != "" || len(flags) != 0 {
			return usageError("--deployment-preset none cannot be combined with policy overrides", "Use a policy preset or remove the policy.")
		}
	case preset != "":
		policy, err = resolveTargetDeploymentPolicyFlags(preset, branches, flags)
		if err != nil {
			return err
		}
	case current.DeploymentPolicy != nil:
		// Preserve every existing operator override unless expressly changed.
		value := *current.DeploymentPolicy
		value.AllowedBranches = append([]string(nil), value.AllowedBranches...)
		policy = &value
		if branches != "" {
			if strings.EqualFold(branches, "all") || branches == "*" {
				policy.AllowedBranches = nil
			} else {
				policy.AllowedBranches = strings.Split(branches, ",")
				for i := range policy.AllowedBranches {
					policy.AllowedBranches[i] = strings.TrimSpace(policy.AllowedBranches[i])
				}
			}
		}
		for flag, raw := range flags {
			value, parseErr := parseDeploymentPolicyBool(raw)
			if parseErr != nil {
				return fmt.Errorf("%s: %w", flag, parseErr)
			}
			switch flag {
			case "--deployment-dirty":
				policy.AllowDirty = value
			case "--deployment-commit":
				policy.RequireCommit = value
			case "--deployment-remote-commit":
				policy.RequireRemoteCommit = value
			case "--deployment-approval":
				policy.RequireApproval = value
			case "--deployment-image-digest":
				policy.RequireImageDigest = value
			}
		}
	default:
		return usageError("an unconfigured Target policy needs an explicit --deployment-preset", "Start with development, test, staging, production or custom.")
	}
	if policy != nil {
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	current.DeploymentPolicy = policy
	cfg.Targets[name] = current
	if err := cfg.SaveExpected(expected); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Target %s deployment policy updated.\n", name)
	return err
}
