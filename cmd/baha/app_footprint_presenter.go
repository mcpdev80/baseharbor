package main

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// appFootprintPresenterOptions affect presentation only; no capacity or
// dependency inference is performed here.
type appFootprintPresenterOptions struct {
	Details bool
}

// presentAppFootprint renders the already resolved authoritative footprint.
// It neither resolves providers nor assumes resources from capability names.
func presentAppFootprint(ctx GuidedReadContext, options appFootprintPresenterOptions) string {
	return renderAppFootprint(ctx.Footprint, ctx.Complete, options)
}

func renderAppFootprint(fp application.Footprint, contextComplete bool, options appFootprintPresenterOptions) string {
	var out strings.Builder
	counts := map[application.ProviderState]int{}
	for _, resource := range fp.Existing {
		counts[resource.State]++
	}
	for _, resource := range fp.Additional {
		counts[resource.State]++
	}
	fmt.Fprintf(&out, "Footprint · Target: %s\n", footprintLabel(fp.Target))
	fmt.Fprintf(&out, "Realizations: %d new, %d reused/running, %d stopped/owned, %d unverified\n",
		counts[application.ProviderNewRequired], counts[application.ProviderRunning],
		counts[application.ProviderStoppedOwned], counts[application.ProviderUnverifiable])
	switch fp.Core {
	case application.CoreNotRequired:
		fmt.Fprintln(&out, "Core: not required")
	case application.CoreRequired:
		fmt.Fprintln(&out, "Core: required by selected providers")
	default:
		fmt.Fprintln(&out, "Core: unknown")
	}
	if fp.Complete && contextComplete {
		fmt.Fprintln(&out, "Resolution: complete")
	} else {
		fmt.Fprintln(&out, "Resolution: incomplete (unknown facts or dependencies)")
	}
	if len(fp.Existing)+len(fp.Additional) == 0 {
		fmt.Fprintln(&out, "No provider realizations selected.")
	}
	if !options.Details {
		if len(fp.Existing)+len(fp.Additional) > 0 {
			fmt.Fprintln(&out, "RAM and containers: see details (individual evidence only; no assumed totals)")
		}
		return strings.TrimSuffix(out.String(), "\n")
	}
	for _, resource := range fp.Additional {
		writeAppFootprintResource(&out, resource)
	}
	for _, resource := range fp.Existing {
		writeAppFootprintResource(&out, resource)
	}
	for _, requirement := range fp.Requirements {
		if requirement.InstanceID == "" {
			fmt.Fprintf(&out, "Unbound requirement: %s/%s (provider unknown)\n",
				requirement.Requirement.Kind, footprintLabel(requirement.Requirement.Name))
		}
	}
	if !fp.Complete || !contextComplete {
		fmt.Fprintln(&out, "Unverified dependency inventories and capacity remain unknown.")
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func writeAppFootprintResource(out *strings.Builder, resource application.ProviderFootprint) {
	state := "unverified"
	switch resource.State {
	case application.ProviderNewRequired:
		state = "new"
	case application.ProviderRunning:
		state = "reused/running"
	case application.ProviderStoppedOwned:
		state = "stopped/owned (not running)"
	}
	fmt.Fprintf(out, "- %s: %s; RAM %s; containers %s\n",
		footprintLabel(resource.InstanceID), state,
		footprintQuantity(resource.MemoryBytes, "bytes"),
		footprintQuantity(resource.Containers, ""))
	if resource.Dependencies == nil {
		fmt.Fprintln(out, "  Dependencies: unknown")
	} else if len(resource.Dependencies) > 0 {
		fmt.Fprintf(out, "  Dependencies: %s\n", strings.Join(resource.Dependencies, ", "))
	} else {
		fmt.Fprintln(out, "  Dependencies: none verified")
	}
}

func footprintQuantity(q application.Quantity, unit string) string {
	if q.Value == nil || (q.Classification != "measured" && q.Classification != "estimated") || q.Source == "" || *q.Value < 0 {
		return "unknown"
	}
	if unit != "" {
		return fmt.Sprintf("%d %s (%s)", *q.Value, unit, q.Classification)
	}
	return fmt.Sprintf("%d (%s)", *q.Value, q.Classification)
}

func footprintLabel(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
