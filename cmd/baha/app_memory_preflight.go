package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/hostresource"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type memoryPreflightOverrideKey struct{}
type assumeYesKey struct{}

func withAssumeYes(ctx context.Context, enabled bool) context.Context {
	if !enabled {
		return ctx
	}
	return context.WithValue(ctx, assumeYesKey{}, true)
}

func assumeYes(ctx context.Context) bool {
	value, _ := ctx.Value(assumeYesKey{}).(bool)
	return value
}

func withMemoryPreflightOverride(ctx context.Context, enabled bool) context.Context {
	if !enabled {
		return ctx
	}
	return context.WithValue(ctx, memoryPreflightOverrideKey{}, true)
}

func memoryPreflightOverride(ctx context.Context) bool {
	value, _ := ctx.Value(memoryPreflightOverrideKey{}).(bool)
	return value
}

func stripMemoryPreflightOverride(args []string) ([]string, bool) {
	filtered := make([]string, 0, len(args))
	skip := false
	for _, arg := range args {
		if arg == "--skip-memory-preflight" {
			skip = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered, skip
}

func runHostMemoryPreflight(ctx context.Context, in io.Reader, out io.Writer, provider bhruntime.ProviderKind, estimate hostresource.MemoryEstimate, mutating bool) error {
	switch provider {
	case bhruntime.ProviderDocker, bhruntime.ProviderPodman:
		// Local container runtimes consume resources on the runtime host, so
		// Linux MemAvailable/swap/PSI are valid evidence.
	case bhruntime.ProviderKubernetes, bhruntime.ProviderOpenShift:
		return &machine.Error{
			Code:        machine.ErrorUnsupported,
			CauseCode:   "cluster_resource_preflight_required",
			Message:     "Cluster runtimes require provider-specific capacity, quota and scheduling preflight; local host memory is not valid evidence.",
			Resource:    "cluster resources",
			Remediation: "runtime provider implementation required",
			Next:        "Use a supported Docker/Podman target until cluster resource-evidence preflight is implemented.",
		}
	default:
		return &machine.Error{
			Code:        machine.ErrorUnsupported,
			CauseCode:   "runtime_resource_preflight_unsupported",
			Message:     fmt.Sprintf("Runtime provider %q has no resource-evidence preflight implementation.", provider),
			Resource:    "runtime resources",
			Remediation: "runtime provider implementation required",
			Next:        "Select a runtime provider with resource preflight support.",
		}
	}
	evidence, err := hostresource.ReadLinux()
	if err != nil {
		// Linux host evidence is required for local Docker/Podman. If this host
		// does not expose it, fail closed for mutations but keep read-only
		// preflight useful on unsupported development hosts.
		if !mutating {
			fmt.Fprintf(out, "[WARN] host memory: evidence unavailable: %v\n", err)
			return nil
		}
		return &machine.Error{
			Code:        machine.ErrorRuntimeUnavailable,
			CauseCode:   "host_memory_evidence_unavailable",
			Message:     "Host memory evidence is unavailable before runtime mutation.",
			Resource:    "host memory",
			Remediation: "requires operator action",
			Next:        "Verify /proc/meminfo is available on the runtime host and retry.",
			Cause:       err,
		}
	}
	return runMemoryEvidencePreflight(ctx, in, out, evidence, estimate, mutating)
}

func runMemoryEvidencePreflight(ctx context.Context, in io.Reader, out io.Writer, evidence hostresource.MemoryEvidence, estimate hostresource.MemoryEstimate, mutating bool) error {
	if evidence.AvailableBytes == 0 {
		return &machine.Error{Code: machine.ErrorHostResourceInsufficient, CauseCode: "host_memory_insufficient", Message: "The selected runtime host has no available memory.", Resource: "host memory", Next: "Free memory on the selected execution host before retrying."}
	}
	result := hostresource.Evaluate(evidence, estimate, hostresource.DefaultPolicy())
	renderHostMemoryPreflight(out, result)

	switch result.Decision {
	case hostresource.DecisionSafe:
		return nil
	case hostresource.DecisionUnsafe:
		return &machine.Error{
			Code:        machine.ErrorHostResourceInsufficient,
			CauseCode:   "host_memory_insufficient",
			Message:     fmt.Sprintf("Host memory is insufficient: %s available, %s reliable planned minimum.", formatMemoryBytes(evidence.AvailableBytes), formatMemoryBytes(estimate.MinimumBytes)),
			Resource:    "host memory",
			Remediation: "requires host capacity",
			Next:        "Free memory, disable optional capabilities, or choose a host with more available memory. No runtime mutation was performed.",
		}
	case hostresource.DecisionTight:
		if memoryPreflightOverride(ctx) {
			fmt.Fprintln(out, "[WARN] host memory override active: continuing despite tight headroom")
			return nil
		}
		if !mutating {
			return nil
		}
		if assumeYes(ctx) {
			fmt.Fprintln(out, "[WARN] host memory approval accepted by --yes: continuing despite tight headroom")
			return nil
		}
		if noInput(ctx) || !readerIsTerminal(in) {
			return &machine.Error{
				Code:        machine.ErrorApprovalRequired,
				CauseCode:   "host_memory_tight",
				Message:     "Host memory headroom is tight and requires explicit operator approval before mutation.",
				Resource:    "host memory",
				Remediation: "requires operator approval",
				Next:        "Retry interactively and confirm, or use --skip-memory-preflight after reviewing the reported host evidence.",
			}
		}
		reader := bufio.NewReader(in)
		confirmed, promptErr := promptYesNo(reader, out, "Continue with tight memory headroom?", false)
		if promptErr != nil {
			return promptErr
		}
		if !confirmed {
			return &machine.Error{
				Code:        machine.ErrorApprovalRequired,
				CauseCode:   "host_memory_tight",
				Message:     "Runtime mutation was cancelled because host memory headroom is tight.",
				Resource:    "host memory",
				Remediation: "requires operator approval",
				Next:        "Free memory or retry after reviewing the host resource report.",
			}
		}
		return nil
	default:
		return nil
	}
}

func renderHostMemoryPreflight(out io.Writer, result hostresource.Result) {
	fmt.Fprintln(out, "Host resources")
	fmt.Fprintf(out, "  Available RAM       %s\n", formatMemoryBytes(result.Evidence.AvailableBytes))
	fmt.Fprintf(out, "  Planned minimum     %s\n", formatMemoryBytes(result.Estimate.MinimumBytes))
	fmt.Fprintf(out, "  Planned estimate    %s\n", formatMemoryBytes(result.Estimate.EstimatedBytes))
	fmt.Fprintf(out, "  Confidence          %s\n", result.Estimate.Confidence)
	if len(result.Estimate.Components) > 0 {
		names := make([]string, 0, len(result.Estimate.Components))
		for _, component := range result.Estimate.Components {
			names = append(names, component.Name)
		}
		fmt.Fprintf(out, "  Planned components  %d: %s\n", len(names), strings.Join(names, ", "))
		if result.Estimate.MinimumBytes == 0 {
			fmt.Fprintln(out, "  Estimate basis      planning budget; no measured reliable minimum")
		}
	}
	fmt.Fprintf(out, "  Safety headroom     %s\n", result.Decision)
	if result.Evidence.SwapTotalBytes > 0 {
		used := result.Evidence.SwapTotalBytes - result.Evidence.SwapFreeBytes
		fmt.Fprintf(out, "  Swap                %s used / %s total\n", formatMemoryBytes(used), formatMemoryBytes(result.Evidence.SwapTotalBytes))
	} else {
		fmt.Fprintln(out, "  Swap                none")
	}
	if result.Evidence.Pressure.Available {
		fmt.Fprintf(out, "  Memory pressure     some avg300=%.2f, full avg300=%.2f\n", result.Evidence.Pressure.SomeAvg300, result.Evidence.Pressure.FullAvg300)
	} else {
		fmt.Fprintln(out, "  Memory pressure     unavailable")
	}
	for _, reason := range result.Reasons {
		fmt.Fprintf(out, "  Reason              %s\n", reason)
	}
}

func formatMemoryBytes(value uint64) string {
	const gib = uint64(1 << 30)
	const mib = uint64(1 << 20)
	if value >= gib {
		return fmt.Sprintf("%.2f GiB", float64(value)/float64(gib))
	}
	return fmt.Sprintf("%.0f MiB", float64(value)/float64(mib))
}
