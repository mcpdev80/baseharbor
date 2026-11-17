package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// guidedTargetDeploymentPolicy proposes best practices without hard-coding
// policy by target name. An older scripted/EOF flow leaves policy unset:
// an explicit operator selection is required before imposing restrictions.
func guidedTargetDeploymentPolicy(reader *bufio.Reader, out io.Writer) ([]string, error) {
	fmt.Fprintln(out, "Deployment policy is optional; choose editable best-practice defaults.")
	fmt.Fprint(out, "Deployment preset (development/test/staging/production/custom/none) [development]: ")
	line, err := reader.ReadString('\n')
	if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
		return nil, nil
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read deployment preset: %w", err)
	}
	preset := strings.ToLower(strings.TrimSpace(line))
	if preset == "" {
		preset = "development"
	}
	if preset == "none" || preset == "cancel" {
		return nil, nil
	}
	policy, err := deployment.DeploymentPolicyPreset(preset)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "Suggested policy %s: branches=%s, dirty=%t, commit=%t, verified-remote-commit=%t, approval=%t, image-digest=%t\n",
		policy.Preset, displayTargetDeploymentBranches(policy.AllowedBranches),
		policy.AllowDirty, policy.RequireCommit, policy.RequireRemoteCommit,
		policy.RequireApproval, policy.RequireImageDigest)
	fmt.Fprint(out, "Customize these values? [y/N]: ")
	line, err = reader.ReadString('\n')
	if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
		return []string{"--deployment-preset", policy.Preset}, nil
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read deployment policy customization: %w", err)
	}
	args := []string{"--deployment-preset", policy.Preset}
	if !isAffirmative(strings.TrimSpace(line)) {
		return args, nil
	}
	questions := []struct {
		label string
		flag  string
		value string
	}{
		{"Allowed branches (all or comma-separated)", "--deployment-branches", displayTargetDeploymentBranches(policy.AllowedBranches)},
		{"Allow uncommitted changes (yes/no)", "--deployment-dirty", targetPolicyYesNo(policy.AllowDirty)},
		{"Require Git commit (yes/no)", "--deployment-commit", targetPolicyYesNo(policy.RequireCommit)},
		{"Require verified remote commit (yes/no)", "--deployment-remote-commit", targetPolicyYesNo(policy.RequireRemoteCommit)},
		{"Require approval (yes/no)", "--deployment-approval", targetPolicyYesNo(policy.RequireApproval)},
		{"Require immutable image digest (yes/no)", "--deployment-image-digest", targetPolicyYesNo(policy.RequireImageDigest)},
	}
	for _, q := range questions {
		fmt.Fprintf(out, "%s [%s]: ", q.label, q.value)
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", q.label, err)
		}
		value := strings.TrimSpace(line)
		if value == "" {
			value = q.value
		}
		args = append(args, q.flag, value)
	}
	_, err = resolveTargetDeploymentPolicyFlags(args[1], args[3], map[string]string{
		"--deployment-dirty":         args[5],
		"--deployment-commit":        args[7],
		"--deployment-remote-commit": args[9],
		"--deployment-approval":      args[11],
		"--deployment-image-digest":  args[13],
	})
	if err != nil {
		return nil, err
	}
	return args, nil
}

func displayTargetDeploymentBranches(refs []string) string {
	if len(refs) == 0 {
		return "all"
	}
	return strings.Join(refs, ",")
}

func targetPolicyYesNo(yes bool) string {
	if yes {
		return "yes"
	}
	return "no"
}
