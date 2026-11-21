package logs_test

// Implicit-selection regression for card 0428-122 (CODEX-REVIEW REQUEST CHANGES
// at 876329f, comment 7002202503): an application workload with NO explicit
// Workload.Components is an implicit Compose workload, and selectWorkloadServices
// selects ALL available non-managed Compose services when requested is empty.
// The render-time reconcile must record that ACTUALLY selected set, not the
// synthetic singleton "workload".

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// composeServicesFromCWD builds a repository root with a compose.yaml declaring
// the given services and returns (manifest, project, repoRoot).
func composeServicesFromCWD(t *testing.T, services ...string) (application.Manifest, string, string) {
	t.Helper()
	repo := t.TempDir()
	var entries []string
	for _, s := range services {
		entries = append(entries, "  "+s+":\n")
	}
	compose := "version: \"3.8\"\nservices:\n" + join(entries)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	m := application.WithLogsCollection(application.New("demo", "dev", false, false, false), "application")
	return m, application.WorkloadProjectName(m), repo
}

func join(ss []string) string {
	out := ""
	for _, s := range ss {
		out += s + "\n"
	}
	return out
}

func assertImplicitSelectedSources(t *testing.T, m application.Manifest, want ...string) {
	t.Helper()
	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	sources, err := logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatalf("workload log listing: %v", err)
	}
	got := map[string]string{}
	for _, s := range sources {
		_, svc, ok := observability.ParseRuntimeTarget(s.Target)
		if !ok {
			t.Fatalf("source %q has no runtime target", s.ID)
		}
		got[svc] = s.ID
	}
	if len(got) != len(want) {
		t.Fatalf("implicit workload sources = %#v, want exactly %v", sources, want)
	}
	for _, svc := range want {
		if _, ok := got[svc]; !ok {
			t.Fatalf("implicit workload missing service %q in %#v", svc, sources)
		}
	}
}

func TestImplicitComposeWorkloadRecordsSelectedServiceSet(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project, repo := composeServicesFromCWD(t, "api", "worker")
	if err := application.ReconcileManagedRuntimeObservability(m, project, repo); err != nil {
		t.Fatalf("render-time reconcile: %v", err)
	}
	assertImplicitSelectedSources(t, m, "api", "worker")
}

func TestImplicitComposeWorkloadShrinkReclaimsStaleService(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project, repo := composeServicesFromCWD(t, "api", "worker")
	if err := application.ReconcileManagedRuntimeObservability(m, project, repo); err != nil {
		t.Fatalf("render-time reconcile 1: %v", err)
	}
	assertImplicitSelectedSources(t, m, "api", "worker")
	// The compose file loses "worker": the next reconcile must reclaim exactly
	// the stale worker source and keep api.
	m, _, repo = composeServicesFromCWD(t, "api")
	if err := application.ReconcileManagedRuntimeObservability(m, project, repo); err != nil {
		t.Fatalf("render-time reconcile 2: %v", err)
	}
	assertImplicitSelectedSources(t, m, "api")
}
