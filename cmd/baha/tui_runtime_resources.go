package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

// Render only resources obtained through the already scoped Runtime Explorer.
// Never inspect a remote Target using the Current Device's Docker/Podman socket.
func renderTargetRuntimeInventory(ctx context.Context, target string) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Runtime resources (selected Target)")
	if err := authorizeCurrentMCPContext(ctx, "runtime.list", target, "dev", ""); err != nil {
		fmt.Fprintln(&b, "  Access unavailable or denied; see baha doctor")
		return b.String()
	}
	resources, err := collectRuntimeResources(ctx, machineRuntimeListInput{Target: target, Kinds: []string{string(runtimeexplorer.KindContainer)}})
	if err != nil {
		fmt.Fprintf(&b, "  Runtime Explorer unavailable: %v\n", err)
		return b.String()
	}
	if len(resources) == 0 {
		fmt.Fprintln(&b, "  No containers returned by this Target")
		return b.String()
	}
	grouped := map[string][]runtimeexplorer.Resource{}
	for _, resource := range resources {
		group := "Other / Unassigned"
		if app := strings.TrimSpace(resource.Relationship.Application); app != "" {
			group = "Application: " + app
		} else if resource.Ownership == runtimeexplorer.OwnershipManaged {
			group = "Managed Core / Providers"
		}
		grouped[group] = append(grouped[group], resource)
	}
	names := make([]string, 0, len(grouped))
	for group := range grouped {
		names = append(names, group)
	}
	sort.Strings(names)
	metricsAllowed := authorizeCurrentMCPContext(ctx, "runtime.metrics", target, "dev", "") == nil
	for _, group := range names {
		fmt.Fprintln(&b, "  "+group)
		for _, resource := range grouped[group] {
			name := resource.DisplayName
			if name == "" {
				name = resource.Ref.ResourceID
			}
			fmt.Fprintf(&b, "    %-24s %-12s (%s)\n", name, resource.State.Observed, resource.Ownership)
			if !metricsAllowed {
				fmt.Fprintln(&b, "      metrics: access unavailable or denied")
				continue
			}
			metrics, err := collectRuntimeMetrics(ctx, machineRuntimeInspectInput{Target: target, Provider: resource.Ref.Provider, Kind: string(resource.Ref.Kind), ResourceID: resource.Ref.ResourceID, Environment: resource.Relationship.Environment})
			if err != nil {
				fmt.Fprintln(&b, "      metrics: unavailable")
				continue
			}
			if !metrics.Available || metrics.Sample == nil {
				fmt.Fprintln(&b, "      metrics: provider does not report a sample")
				continue
			}
			fmt.Fprintf(&b, "      CPU %s  RAM %s\n", metrics.Sample.CPUPercent, metrics.Sample.MemoryUsage)
		}
	}
	return b.String()
}
