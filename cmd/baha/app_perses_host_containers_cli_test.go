package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestBahaStatusHostContainersDashboardsExposesUnavailable drives the genuine
// `baha status --json` for a manifest that selects Perses and asserts the
// host_containers_dashboards face is deterministic and explicit: raw host node
// metrics and cAdvisor per-container stats are exposed as "unavailable" with a
// reason naming the missing native signal, and are never silently omitted.

type persesHostContainersReport struct {
	State                    string                                  `json:"state"`
	Ready                    bool                                    `json:"ready"`
	Perses                   *string                                 `json:"perses"`
	HostContainersDashboards map[string]hostContainersDashboardAvail `json:"host_containers_dashboards"`
}

type hostContainersDashboardAvail struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

func runPersesHostContainersStatusCommand(t *testing.T, ctx context.Context) persesHostContainersReport {
	t.Helper()
	var out, errOut bytes.Buffer
	err := runWithIO(ctx, []string{"status", "--json"}, &out, &errOut)
	if err != nil && !strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var report persesHostContainersReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("baha status --json output is not the public report: %v\n%s", err, out.String())
	}
	return report
}

// hostContainersProbeAppReport authors an initialized repository whose
// observability management UI optionally realizes Perses, keeps the not-applied
// face, and returns its parsed host_containers_dashboards status report.
func hostContainersProbeAppReport(t *testing.T, ctx context.Context, perses bool) persesHostContainersReport {
	t.Helper()
	root := t.TempDir()
	m := application.New("perses-host-cli", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(persesProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if _, err := resolveApplication(ctx, application.DefaultStore(), nil, "status"); err != nil {
		t.Fatalf("resolve probe application: %v", err)
	}
	return runPersesHostContainersStatusCommand(t, ctx)
}

func TestBahaStatusHostContainersDashboardsExposesUnavailable(t *testing.T) {
	ctx := t.Context()
	report := hostContainersProbeAppReport(t, ctx, true)

	if report.HostContainersDashboards == nil {
		t.Fatal("status JSON must expose the host_containers_dashboards face")
	}
	// Negative witness: the two host/container views with no native signal are
	// exposed as "unavailable" with a reason, never silently omitted.
	for _, key := range []string{"host-node-metrics", "container-stats"} {
		avail, ok := report.HostContainersDashboards[key]
		if !ok {
			t.Fatalf("host_containers_dashboards must expose %q explicitly", key)
		}
		if avail.Available != "unavailable" {
			t.Fatalf("host/container view %q must report unavailable, got %q", key, avail.Available)
		}
		if avail.Reason == "" {
			t.Fatalf("host/container view %q must carry a concrete reason", key)
		}
		if !strings.Contains(avail.Reason, "no native") {
			t.Fatalf("host/container view %q reason must name the missing native signal, got %q", key, avail.Reason)
		}
	}
	// Positive witness: the three real backends are exposed as available.
	for _, key := range []string{"prometheus-host-container", "loki-host-container", "tempo-host-container"} {
		avail, ok := report.HostContainersDashboards[key]
		if !ok {
			t.Fatalf("host_containers_dashboards must expose %q", key)
		}
		if avail.Available != "available" {
			t.Fatalf("host/container backend %q must report available, got %q", key, avail.Available)
		}
		if avail.Datasource == "" {
			t.Fatalf("available host/container backend %q must carry its datasource kind", key)
		}
	}
}
