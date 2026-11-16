package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// reconciledListState maps a deployment record's recorded observed state to the
// single token shown in `app list`. It reconciles the recorded state against the
// reference dimensions so a stale "ready"/"applied" never survives on the column
// once the runtime has drifted: a recorded failed/stopped/retired state, or an
// observed reference that diverges from the last-applied reference, both surface
// as DEGRADED instead of the recorded terminal/ready token. The live SIGKILL/OOM
// case (recorded state still "ready" but the unit is gone) is reconciled by
// `baha status`, which probes the workload; this record-only view still never lies.
func reconciledListState(record deployment.DeploymentRecord) string {
	if record.Observed.State == "" {
		if record.Applied.RuntimeProvider == "" {
			return "not_applied"
		}
		return "applied"
	}
	switch strings.ToLower(strings.TrimSpace(record.Observed.State)) {
	case "failed", "stopped", "degraded", "runtime-retired", "retiring":
		return "DEGRADED"
	}
	if record.Observed.Ready || strings.EqualFold(strings.TrimSpace(record.Observed.State), "ready") {
		// Only flag divergence when both dimensions are recorded and differ; an
		// unrecorded observed reference (older or reference-less workloads) is not
		// drift and must not be reported as DEGRADED.
		observed, hasObserved := record.References[deployment.ReferenceObserved]
		applied, hasApplied := record.References[deployment.ReferenceApplied]
		if hasObserved && hasApplied && observed != nil && applied != nil {
			if strings.TrimSpace(observed.Release) != strings.TrimSpace(applied.Release) {
				return "DEGRADED"
			}
		}
	}
	return strings.TrimSpace(record.Observed.State)
}

// printListNextActions prints actionable next-steps for deployments whose recorded
// state is not cleanly ready: a failed/degraded workload gets a re-up/destroy hint
// with the observed date, and a deployment whose source repository is gone (MISSING)
// gets an exact prune/re-attach command.
func printListNextActions(out io.Writer, items []deployment.DeploymentRecord) {
	changed := false
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(item.Observed.State))
		switch {
		case state == "failed" || state == "stopped" || state == "degraded" || state == "runtime-retired" || state == "retiring":
			if !changed {
				fmt.Fprintln(out, "\nNext:")
				changed = true
			}
			if !item.Observed.VerifiedAt.IsZero() {
				fmt.Fprintf(out, "  %s/%s: %s is not applied (observed %s) - re-run 'baha up' to rebuild it, or 'baha app destroy' if it is gone for good\n",
					item.Identity.Application, item.Identity.Environment, item.Observed.State, item.Observed.VerifiedAt.UTC().Format(time.RFC3339))
			} else {
				fmt.Fprintf(out, "  %s/%s: %s is not applied - re-run 'baha up' to rebuild it, or 'baha app destroy' if it is gone for good\n",
					item.Identity.Application, item.Identity.Environment, item.Observed.State)
			}
		case deployment.SourceAvailable(item.Source):
		default:
			if !changed {
				fmt.Fprintln(out, "\nNext:")
				changed = true
			}
			fmt.Fprintf(out, "  %s/%s: source repository gone (MISSING) - prune the dangling record with `baha app prune %s`, or re-run 'baha up' once the repo is restored\n",
				item.Identity.Application, item.Identity.Environment, item.Identity.Application)
		}
	}
}
