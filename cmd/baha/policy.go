package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/policy"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func policyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "policy",
		Summary: "Inspect the effective BaseHarbor policy for an application environment",
		Usage:   "baha policy <check|explain> [NAME] [-e ENV|--environment ENV] [-o json|--output json]",
		Children: []*cli.Command{
			{
				Name:    "check",
				Summary: "Evaluate effective policy against the selected application",
				Usage:   "baha policy check [NAME] [-e ENV|--environment ENV] [-o json|--output json]",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					appArgs, environment, format, err := parsePolicyArgs(args, "policy check")
					if err != nil {
						return err
					}
					result, err := collectApplicationPolicy(ctx, store, appArgs, environment)
					if err != nil {
						return err
					}
					if format == outputJSON {
						if err := writeJSON(out, result); err != nil {
							return err
						}
					} else {
						renderPolicyCheck(out, result)
					}
					if result.Denied() {
						return cli.Presented(errors.New("policy denied"))
					}
					return nil
				},
			},
			{
				Name:    "explain",
				Summary: "Explain effective policy, defaults and bounded overrides",
				Usage:   "baha policy explain [NAME] [-e ENV|--environment ENV] [-o json|--output json]",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					appArgs, environment, format, err := parsePolicyArgs(args, "policy explain")
					if err != nil {
						return err
					}
					result, err := explainApplicationPolicy(ctx, store, appArgs, environment)
					if err != nil {
						return err
					}
					if format == outputJSON {
						return writeJSON(out, result)
					}
					renderPolicyExplanation(out, result)
					return nil
				},
			},
		},
	}
}

func parsePolicyArgs(args []string, command string) ([]string, string, cliOutputFormat, error) {
	filtered := make([]string, 0, len(args))
	environment := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-e" || arg == "--environment":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, "", outputHuman, usageError("--environment requires ENV", "Example: baha "+command+" -e prod")
			}
			i++
			environment = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--environment="):
			environment = strings.TrimSpace(strings.TrimPrefix(arg, "--environment="))
			if environment == "" {
				return nil, "", outputHuman, usageError("--environment requires ENV", "Example: baha "+command+" --environment prod")
			}
		default:
			filtered = append(filtered, arg)
		}
	}
	filtered, format, err := parseReadOutputArgs(filtered, command)
	if err != nil {
		return nil, "", outputHuman, err
	}
	if len(filtered) > 1 {
		return nil, "", outputHuman, usageError("baha "+command+" accepts at most one NAME", "Inside an application repository omit NAME.")
	}
	return filtered, environment, format, nil
}

func collectApplicationPolicy(ctx context.Context, store application.Store, appArgs []string, environment string) (policy.Result, error) {
	resolved, err := resolveApplicationEnvironment(ctx, store, appArgs, "policy check", environment)
	if err != nil {
		return policy.Result{}, err
	}
	result, err := policyExplanationForManifest(resolved.Manifest)
	if err != nil {
		return policy.Result{}, err
	}
	if !resolved.FromRepository {
		return result, nil
	}
	_, found, err := application.ResolveWorkloadCompose(resolved.repositoryRoot(), resolved.Manifest)
	if err != nil {
		return policy.Result{}, err
	}
	if !found {
		return result, nil
	}

	compose, err := detectComposeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle)
	if err != nil {
		return policy.Result{}, err
	}
	report, reportErr := preflightRepositoryWorkloadSecurity(ctx, compose, resolved)
	if reportErr != nil && !report.Denied() {
		return policy.Result{}, reportErr
	}
	for _, finding := range report.Findings {
		source := policy.SourceWorkload
		if finding.Decision == application.WorkloadSecurityAllow && containsString(result.Overrides, finding.Code) {
			source = policy.SourceOperatorOverride
		}
		result.Findings = append(result.Findings, policy.Finding{
			Code:     finding.Code,
			Decision: policy.Decision(finding.Decision),
			Scope:    "workload",
			Subject:  finding.Service,
			Field:    finding.Field,
			Value:    finding.Value,
			Message:  finding.Message,
			Source:   source,
		})
	}
	result.Decision = policy.Aggregate(result.Findings)
	return result, nil
}

func explainApplicationPolicy(ctx context.Context, store application.Store, appArgs []string, environment string) (policy.Result, error) {
	resolved, err := resolveApplicationEnvironment(ctx, store, appArgs, "policy explain", environment)
	if err != nil {
		return policy.Result{}, err
	}
	return policyExplanationForManifest(resolved.Manifest)
}

func policyExplanationForManifest(m application.Manifest) (policy.Result, error) {
	effective, err := application.ResolveWorkloadSecurityPolicy(m)
	if err != nil {
		return policy.Result{}, err
	}
	result := policy.Result{
		ContractVersion: policy.ContractVersion,
		Environment:     m.Environment,
		Profile:         effective.Mode,
		Decision:        policy.Allow,
		Overrides:       append([]string(nil), effective.AllowedOverrides...),
	}
	managed := effective.Mode != "development"
	result.Rules = []policy.Rule{
		{Code: "privileged", Decision: policy.Deny, Description: "Privileged containers bypass the workload isolation boundary."},
		{Code: "runtime-socket", Decision: policy.Deny, Description: "Container runtime socket access is a host-control-plane escape path."},
		{Code: "host-network", Decision: policy.Deny, Description: "Host networking bypasses BaseHarbor network isolation."},
		{Code: "host-pid", Decision: policy.Deny, Description: "Host PID namespace sharing bypasses process isolation."},
		{Code: "host-ipc", Decision: policy.Deny, Description: "Host IPC namespace sharing bypasses workload isolation."},
		{Code: "critical-host-mount", Decision: policy.Deny, Description: "Critical host filesystem mounts bypass filesystem isolation."},
		{Code: "dangerous-capability", Decision: policy.Deny, Description: "Dangerous Linux capabilities materially weaken workload isolation."},
	}
	deviceDecision := policy.Warn
	overridable := true
	if managed {
		deviceDecision = policy.Deny
		overridable = false
	}
	if containsString(effective.AllowedOverrides, "host-device") {
		deviceDecision = policy.Allow
	}
	result.Rules = append(result.Rules, policy.Rule{
		Code:        "host-device",
		Decision:    deviceDecision,
		Overridable: overridable,
		Description: "Host device access is denied outside development and requires an explicit bounded acknowledgement in development.",
	})
	return result, nil
}

func renderPolicyCheck(out io.Writer, result policy.Result) {
	fmt.Fprintf(out, "Environment: %s\n", result.Environment)
	fmt.Fprintf(out, "Policy profile: %s\n", result.Profile)
	if len(result.Findings) == 0 {
		fmt.Fprintln(out, "Policy: ALLOW · no policy findings")
		return
	}
	for _, finding := range result.Findings {
		fmt.Fprintf(out, "[%s] %s %s/%s: %s\n", strings.ToUpper(string(finding.Decision)), finding.Code, finding.Scope, finding.Subject, finding.Message)
	}
	fmt.Fprintf(out, "Policy: %s\n", strings.ToUpper(string(result.Decision)))
}

func renderPolicyExplanation(out io.Writer, result policy.Result) {
	fmt.Fprintf(out, "Environment: %s\n", result.Environment)
	fmt.Fprintf(out, "Policy profile: %s\n", result.Profile)
	fmt.Fprintln(out, "Effective rules:")
	for _, rule := range result.Rules {
		override := ""
		if rule.Overridable {
			override = " · bounded override available"
		}
		fmt.Fprintf(out, "  %-22s %-5s%s\n", rule.Code, strings.ToUpper(string(rule.Decision)), override)
	}
	if len(result.Overrides) > 0 {
		fmt.Fprintf(out, "Operator overrides: %s\n", strings.Join(result.Overrides, ", "))
	} else {
		fmt.Fprintln(out, "Operator overrides: none")
	}
}
