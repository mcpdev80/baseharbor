package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// BF-04 regression: a deployment whose recorded state is failed/stopped/retired
// must surface as DEGRADED in `app list`, never as the stale terminal/ready
// token, so an operator scanning the list never reads a killed workload as "up".
func TestReconciledListStateDowngradesRecordedFailedStoppedRetired(t *testing.T) {
	cases := map[string]string{
		"failed":          "DEGRADED",
		"stopped":         "DEGRADED",
		"degraded":        "DEGRADED",
		"runtime-retired": "DEGRADED",
		"retiring":        "DEGRADED",
	}
	for recorded, want := range cases {
		got := reconciledListState(deployment.DeploymentRecord{
			Identity: deployment.DeploymentIdentity{Application: "x", Environment: "dev"},
			Observed: deployment.ObservedDeployment{State: recorded, VerifiedAt: time.Now().UTC()},
		})
		if got != want {
			t.Errorf("recorded %q => %q, want %q", recorded, got, want)
		}
	}
}

// The SIGKILL/OOM case where the unit dies but the record still says "ready":
// the record-only list view must not report a false ready once the observed
// reference diverges from the last-applied reference (drift).
func TestReconciledListStateFlagsObservedAppliedDivergence(t *testing.T) {
	readyRecord := deployment.DeploymentRecord{
		Identity: deployment.DeploymentIdentity{Application: "x", Environment: "dev"},
		Observed: deployment.ObservedDeployment{State: "ready", Ready: true},
		References: deployment.ReferenceDimensions{
			deployment.ReferenceApplied:  &deployment.ReferenceDimension{Kind: deployment.ReferenceApplied, Release: "img-applied"},
			deployment.ReferenceObserved: &deployment.ReferenceDimension{Kind: deployment.ReferenceObserved, Release: "img-drifted"},
		},
	}
	if got := reconciledListState(readyRecord); got != "DEGRADED" {
		t.Fatalf("observed/applied divergence => %q, want DEGRADED", got)
	}

	convergedRecord := deployment.DeploymentRecord{
		Identity: deployment.DeploymentIdentity{Application: "x", Environment: "dev"},
		Observed: deployment.ObservedDeployment{State: "ready", Ready: true},
		References: deployment.ReferenceDimensions{
			deployment.ReferenceApplied:  &deployment.ReferenceDimension{Kind: deployment.ReferenceApplied, Release: "img"},
			deployment.ReferenceObserved: &deployment.ReferenceDimension{Kind: deployment.ReferenceObserved, Release: "img"},
		},
	}
	if got := reconciledListState(convergedRecord); got != "ready" {
		t.Fatalf("converged record => %q, want ready (casing preserved)", got)
	}
}

// BF-05 regression: a deployment whose source repository is gone (MISSING) gets
// an exact next-action line instead of an opaque MISSING with no way forward.
func TestPrintListNextActionsReportsMissingWithExactCommand(t *testing.T) {
	repo := t.TempDir()
	items := []deployment.DeploymentRecord{
		{
			Identity: deployment.DeploymentIdentity{Application: "gone-app", Environment: "dev"},
			Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo},
			Observed: deployment.ObservedDeployment{State: "ready", Ready: true},
		},
	}
	// Repo present: no misleading MISSING advice.
	var out bytes.Buffer
	printListNextActions(&out, items)
	if strings.Contains(out.String(), "MISSING") {
		t.Fatalf("present source should not advise MISSING:\n%s", out.String())
	}

	// Repo gone: advise the exact prune command.
	items[0].Source.Repository = "/nonexistent/path/definitely/absent"
	out.Reset()
	printListNextActions(&out, items)
	got := out.String()
	if !strings.Contains(got, "MISSING") || !strings.Contains(got, "baha app prune") {
		t.Fatalf("MISSING next-action missing exact prune command:\n%s", got)
	}
}

// A failed/degraded record surfaces the re-up/destroy hint with the observed date.
func TestPrintListNextActionsHintsReupWithObservedDate(t *testing.T) {
	when := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	items := []deployment.DeploymentRecord{
		{
			Identity: deployment.DeploymentIdentity{Application: "killed-app", Environment: "dev"},
			Source:   deployment.DeploymentSource{Kind: "repository", Repository: t.TempDir()},
			Observed: deployment.ObservedDeployment{State: "failed", VerifiedAt: when},
		},
	}
	var out bytes.Buffer
	printListNextActions(&out, items)
	got := out.String()
	if !strings.Contains(got, "DEGRADED") && !strings.Contains(got, "failed") {
		t.Fatalf("failed record should hint re-up, got:\n%s", got)
	}
	if !strings.Contains(got, "re-up") && !strings.Contains(got, "baha up") && !strings.Contains(got, "re-run") {
		t.Fatalf("failed record should offer a rebuild hint, got:\n%s", got)
	}
	if !strings.Contains(got, "2026-11-08") {
		t.Fatalf("failed record should include the observed date, got:\n%s", got)
	}
}

// reconciledListState must never turn a healthy ready record into DEGRADED.
func TestReconciledListStateKeepsHealthyReady(t *testing.T) {
	rec := deployment.DeploymentRecord{
		Identity: deployment.DeploymentIdentity{Application: "x", Environment: "dev"},
		Observed: deployment.ObservedDeployment{State: "ready", Ready: true},
	}
	if got := reconciledListState(rec); got != "ready" {
		t.Fatalf("healthy ready => %q, want ready (casing preserved)", got)
	}
}
