package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/hostresource"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

var hostMemoryInput io.Reader = os.Stdin

func extractMemoryPreflightOption(args []string) ([]string, bool, error) {
	filtered := make([]string, 0, len(args))
	skip := false
	for _, arg := range args {
		switch arg {
		case "--skip-memory-preflight":
			skip = true
		default:
			if strings.HasPrefix(arg, "--skip-memory-preflight=") {
				return nil, false, usageError("--skip-memory-preflight does not take a value", "Use --skip-memory-preflight by itself.")
			}
			filtered = append(filtered, arg)
		}
	}
	return filtered, skip, nil
}

func evaluateApplicationHostMemory(m application.Manifest) (hostresource.Result, error) {
	evidence, err := hostresource.ReadLinux()
	if err != nil {
		return hostresource.Result{}, err
	}
	return hostresource.Evaluate(evidence, hostresource.EstimateApplication(m), hostresource.DefaultPolicy()), nil
}

func renderHostMemoryResult(out io.Writer, result hostresource.Result) {
	const gib = float64(1 << 30)
	fmt.Fprintln(out, "Host resources")
	fmt.Fprintf(out, "  Available RAM       %.2f GiB\n", float64(result.Evidence.AvailableBytes)/gib)
	fmt.Fprintf(out, "  Planned minimum     %.2f GiB\n", float64(result.Estimate.MinimumBytes)/gib)
	fmt.Fprintf(out, "  Planned estimate    %.2f GiB\n", float64(result.Estimate.EstimatedBytes)/gib)
	if result.Evidence.SwapTotalBytes > 0 {
		used := result.Evidence.SwapTotalBytes - result.Evidence.SwapFreeBytes
		fmt.Fprintf(out, "  Swap used           %.2f / %.2f GiB\n", float64(used)/gib, float64(result.Evidence.SwapTotalBytes)/gib)
	} else {
		fmt.Fprintln(out, "  Swap                not configured")
	}
	if result.Evidence.Pressure.Available {
		fmt.Fprintf(out, "  Memory PSI avg300   some %.2f / full %.2f\n", result.Evidence.Pressure.SomeAvg300, result.Evidence.Pressure.FullAvg300)
	} else {
		fmt.Fprintln(out, "  Memory PSI          unavailable")
	}
	fmt.Fprintf(out, "  Decision            %s (%s)\n", result.Decision, result.Estimate.Confidence)
	for _, reason := range result.Reasons {
		fmt.Fprintf(out, "  - %s\n", reason)
	}
}

func enforceApplicationHostMemory(ctx context.Context, m application.Manifest, out io.Writer, skip bool, allowPrompt bool) error {
	result, err := evaluateApplicationHostMemory(m)
	if err != nil {
		return &machine.Error{
			Code:        machine.ErrorValidationFailed,
			CauseCode:   "host_memory_evidence_unavailable",
			Message:     "Host memory evidence could not be read before mutation.",
			Resource:    "host-memory",
			Next:        "Verify Linux /proc memory evidence is available, then retry.",
			Remediation: "operator_review",
			Cause:       err,
		}
	}
	renderHostMemoryResult(out, result)

	switch result.Decision {
	case hostresource.DecisionSafe:
		return nil
	case hostresource.DecisionUnsafe:
		return &machine.Error{
			Code:        machine.ErrorValidationFailed,
			CauseCode:   "host_memory_insufficient",
			Message:     "Available host memory is below the reliable minimum required for the planned runtime delta.",
			Resource:    "host-memory",
			Next:        "Free host memory, reduce the selected capability set, or move the workload to a target with more headroom.",
			Remediation: "operator_action",
		}
	case hostresource.DecisionTight:
		if skip {
			fmt.Fprintln(out, "[WARN] host memory preflight override accepted; continuing with tight/pressured headroom")
			return nil
		}
		if !allowPrompt || noInput(ctx) || !readerIsTerminal(hostMemoryInput) {
			return &machine.Error{
				Code:        machine.ErrorApprovalRequired,
				CauseCode:   "host_memory_confirmation_required",
				Message:     "Host memory headroom is tight or pressured and requires explicit confirmation before mutation.",
				Resource:    "host-memory",
				Next:        "Retry interactively and confirm, or use --skip-memory-preflight after reviewing the reported host evidence.",
				Remediation: "requires developer input",
			}
		}
		ok, promptErr := promptYesNo(bufio.NewReader(hostMemoryInput), out, "Continue despite tight host memory?", false)
		if promptErr != nil {
			return promptErr
		}
		if !ok {
			return &machine.Error{
				Code:        machine.ErrorApprovalRequired,
				CauseCode:   "host_memory_confirmation_declined",
				Message:     "Host memory preflight was not approved; no runtime mutation was performed.",
				Resource:    "host-memory",
				Next:        "Free host memory or retry after reviewing the planned runtime footprint.",
				Remediation: "requires developer input",
			}
		}
		return nil
	default:
		return &machine.Error{
			Code:        machine.ErrorValidationFailed,
			CauseCode:   "host_memory_decision_unknown",
			Message:     "Host memory preflight returned an unknown decision.",
			Resource:    "host-memory",
			Next:        "Retry after inspecting host resource diagnostics.",
			Remediation: "operator_review",
		}
	}
}
