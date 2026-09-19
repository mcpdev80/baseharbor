package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

// reportRepositoryContractEvolution gives normal repository-first `baha up`
// enough awareness to notice capability evolution without turning heuristic
// detection into mutation or authorization.
func reportRepositoryContractEvolution(ctx context.Context, out, errOut io.Writer, resolved resolvedApplication) {
	if !resolved.FromRepository || strings.TrimSpace(resolved.ManifestPath) == "" {
		return
	}
	result, err := repositoryinspect.Inspect(ctx, filepath.Dir(resolved.ManifestPath))
	if err != nil {
		fmt.Fprintf(errOut, "[WARN] repository capability reconciliation unavailable: %v\n", err)
		return
	}

	var relevant []repositoryinspect.ReconciliationItem
	for _, item := range result.Reconciliation {
		if item.State == repositoryinspect.ReconciliationNew ||
			item.State == repositoryinspect.ReconciliationAmbiguous ||
			len(item.Operations) > 0 {
			relevant = append(relevant, item)
		}
	}
	if len(relevant) == 0 {
		return
	}

	fmt.Fprintln(out, "Application contract review:")
	for _, item := range relevant {
		label := item.Capability
		if item.Name != "" {
			label += "/" + item.Name
		}
		direction := ""
		if item.Direction != "" {
			direction = " " + string(item.Direction)
		}
		operations := ""
		if len(item.Operations) > 0 {
			values := make([]string, 0, len(item.Operations))
			for _, operation := range item.Operations {
				values = append(values, string(operation))
			}
			operations = " [" + strings.Join(values, ", ") + "]"
		}
		fmt.Fprintf(out, "  [%s] %s%s%s\n", strings.ToUpper(string(item.State)), label, direction, operations)
	}
	fmt.Fprintln(out, "Repository evolution was detected; no contract changes or runtime permissions were applied.")
	fmt.Fprintln(out, "Run 'baha app inspect .' to review the supporting evidence.")
}
