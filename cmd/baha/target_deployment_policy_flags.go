package main

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// resolveTargetDeploymentPolicyFlags applies explicit operator overrides to a
// suggested profile. No flags means nil: existing behavior is unchanged.
func resolveTargetDeploymentPolicyFlags(preset, branches string, flags map[string]string) (*deployment.TargetDeploymentPolicy, error) {
	if preset == "" && branches == "" && len(flags) == 0 {
		return nil, nil
	}
	if preset == "" {
		preset = "custom"
	}
	policy, err := deployment.DeploymentPolicyPreset(preset)
	if err != nil {
		return nil, err
	}
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
	for name, value := range flags {
		enabled, err := parseDeploymentPolicyBool(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		switch name {
		case "--deployment-dirty":
			policy.AllowDirty = enabled
		case "--deployment-commit":
			policy.RequireCommit = enabled
		case "--deployment-remote-commit":
			policy.RequireRemoteCommit = enabled
		case "--deployment-approval":
			policy.RequireApproval = enabled
		case "--deployment-image-digest":
			policy.RequireImageDigest = enabled
		default:
			return nil, fmt.Errorf("unknown target deployment setting %q", name)
		}
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &policy, nil
}

func parseDeploymentPolicyBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "on", "allow", "required":
		return true, nil
	case "false", "no", "n", "off", "deny", "optional":
		return false, nil
	default:
		return false, fmt.Errorf("expected yes/no (got %q)", value)
	}
}
