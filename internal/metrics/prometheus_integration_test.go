package metrics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestManagedPrometheusScrapesTwoIsolatedApplications(t *testing.T) {
	if os.Getenv("BASEHARBOR_METRICS_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_METRICS_INTEGRATION=1 to run real Prometheus acceptance")
	}

	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect Compose runtime: %v", err)
	}

	var containers []string
	defer func() {
		for _, name := range containers {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", name).Run()
			cleanupCancel()
		}
		if t.Failed() {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		if err := DestroySharedProvider(cleanupCtx, compose); err != nil {
			t.Errorf("destroy shared Prometheus provider: %v", err)
		}
	}()

	apps := []string{"metrics-alpha", "metrics-beta"}
	for _, name := range apps {
		m := application.New(name, "dev", false, false, false)
		m.Services.Postgres = false
		m = application.WithWorkload(m, "compose.yaml", "api")
		m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")

		driver := NewDriver(compose, m)
		resource := capability.Resource{
			Application: m.Name,
			Kind:        capability.Metrics,
			Name:        "application",
			Provider:    capability.ProviderPrometheus,
		}
		binding := capability.Binding{
			Resource: resource,
			Workload: "service/api",
			Metrics: &capability.MetricsBinding{
				Direction: "provide",
				Format:    "openmetrics",
				Service:   "api",
				Port:      8080,
				Path:      "/metrics",
			},
		}
		if err := driver.Preflight(ctx, resource, binding); err != nil {
			t.Fatalf("%s preflight: %v", m.Name, err)
		}
		if err := driver.Provision(ctx, resource, binding); err != nil {
			t.Fatalf("%s provision: %v", m.Name, err)
		}

		containerName := "baseharbor-" + name
		containers = append(containers, containerName)
		script := "mkdir -p /srv; printf '# TYPE baseharbor_acceptance_metric gauge\\nbaseharbor_acceptance_metric 1\\n' > /srv/metrics; exec python -m http.server 8080 --directory /srv"
		cmd := exec.CommandContext(
			ctx,
			"docker", "run", "-d", "--rm",
			"--name", containerName,
			"--network", ProviderNetwork,
			"--network-alias", application.MetricsTargetAlias(m, "api"),
			"python:3.13-alpine",
			"sh", "-c", script,
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s start metrics endpoint: %v\n%s", m.Name, err, output)
		}

		if err := driver.Bind(ctx, resource, binding); err != nil {
			t.Fatalf("%s bind: %v", m.Name, err)
		}
		if err := driver.Verify(ctx, resource, binding); err != nil {
			t.Fatalf("%s verify real scrape/ingestion: %v", m.Name, err)
		}
	}

	files, err := ExistingProviderFiles()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(files.TargetsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("shared Prometheus target count = %d, want 2: %#v", len(entries), entries)
	}
	for _, name := range apps {
		m := application.New(name, "dev", false, false, false)
		target := filepath.Join(files.TargetsDir, targetFileName(m, "application"))
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("%s target missing from shared provider state: %v", name, err)
		}
	}
	fmt.Fprintln(os.Stdout, "verified two isolated application targets through one shared Prometheus provider")
}
