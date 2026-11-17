package deployment

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// TargetDeploymentPolicy belongs to the owning Target, not portable app intent.
// A nil policy leaves pre-existing Targets unchanged. Preset is informational:
// only the persisted values, never a Target name, determine restrictions.
type TargetDeploymentPolicy struct {
	Preset              string   `yaml:"preset,omitempty" json:"preset,omitempty"`
	AllowedBranches     []string `yaml:"allowed-branches,omitempty" json:"allowed_branches,omitempty"`
	AllowDirty          bool     `yaml:"allow-dirty" json:"allow_dirty"`
	RequireCommit       bool     `yaml:"require-commit" json:"require_commit"`
	RequireRemoteCommit bool     `yaml:"require-remote-commit" json:"require_remote_commit"`
	RequireApproval     bool     `yaml:"require-approval" json:"require_approval"`
	RequireImageDigest  bool     `yaml:"require-image-digest" json:"require_image_digest"`
}

var branchPattern = regexp.MustCompile(`^[A-Za-z0-9_./*-]+$`)

// DeploymentPolicyPreset provides editable best-practice starting values.
// In particular, Production does not silently impose rules on existing Targets.
func DeploymentPolicyPreset(name string) (TargetDeploymentPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "development", "dev":
		return TargetDeploymentPolicy{Preset: "development", AllowDirty: true}, nil
	case "test":
		return TargetDeploymentPolicy{
			Preset: "test", AllowedBranches: []string{"develop", "release/*"},
			RequireCommit: true, RequireImageDigest: true,
		}, nil
	case "staging", "stage":
		return TargetDeploymentPolicy{
			Preset: "staging", AllowedBranches: []string{"main", "release/*"},
			RequireCommit: true, RequireImageDigest: true,
		}, nil
	case "production", "prod":
		return TargetDeploymentPolicy{
			Preset: "production", AllowedBranches: []string{"main"},
			RequireCommit: true, RequireRemoteCommit: true,
			RequireApproval: true, RequireImageDigest: true,
		}, nil
	case "custom":
		return TargetDeploymentPolicy{Preset: "custom"}, nil
	default:
		return TargetDeploymentPolicy{}, fmt.Errorf("unknown target deployment preset %q (choose development, test, staging, production or custom)", name)
	}
}

// Validate checks persisted policy syntax only. Branch and provenance checks
// must run within the Core-owned, authorized candidate admission path.
func (p TargetDeploymentPolicy) Validate() error {
	switch p.Preset {
	case "", "development", "test", "staging", "production", "custom":
	default:
		return fmt.Errorf("unsupported deployment preset %q", p.Preset)
	}
	seen := make(map[string]struct{}, len(p.AllowedBranches))
	for _, ref := range p.AllowedBranches {
		if ref == "" || !branchPattern.MatchString(ref) || strings.HasPrefix(ref, "/") ||
			strings.HasSuffix(ref, "/") || strings.Contains(ref, "..") ||
			strings.Contains(ref, "//") || strings.Contains(ref, "**") {
			return fmt.Errorf("invalid allowed branch pattern %q", ref)
		}
		if _, err := path.Match(ref, ref); err != nil {
			return fmt.Errorf("invalid allowed branch pattern %q: %w", ref, err)
		}
		if _, ok := seen[ref]; ok {
			return fmt.Errorf("duplicate allowed branch pattern %q", ref)
		}
		seen[ref] = struct{}{}
	}
	return nil
}

// AllowsBranch evaluates a verified branch identity. Empty AllowedBranches
// means unrestricted; callers MUST NOT trust a workstation-provided claim.
func (p TargetDeploymentPolicy) AllowsBranch(branch string) bool {
	if len(p.AllowedBranches) == 0 {
		return true
	}
	for _, pattern := range p.AllowedBranches {
		matches, err := path.Match(pattern, branch)
		if err == nil && matches {
			return true
		}
	}
	return false
}
