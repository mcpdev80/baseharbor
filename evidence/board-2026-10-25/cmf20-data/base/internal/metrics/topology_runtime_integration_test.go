package metrics

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/capability"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPrometheusDefaultTopologyRuntimeAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_PROVIDER_TOPOLOGY_ACCEPTANCE") != "1" {
		t.Skip("isolated native topology acceptance is not enabled")
	}
	no, yes := false, true
	for _, scenario := range []struct {
		name      string
		global    bool
		overrides map[string]availability.Override
		members   int
	}{
		{name: "omitted", members: 1},
		{name: "false", global: false, overrides: map[string]availability.Override{"metrics": {HA: &no}}, members: 1},
		{name: "override-single", global: true, overrides: map[string]availability.Override{"metrics": {HA: &no}}, members: 1},
		{name: "override-ha", overrides: map[string]availability.Override{"metrics": {HA: &yes}}, members: 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {

			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()

			runtime, err := testruntime.Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dataDir := filepath.Join(t.TempDir(), "data")
			namespace := "prometheus-topology-" + scenario.name
			t.Setenv(application.MetricsEnabledEnv, "true")

			app := application.New("prometheus-topology-ci", "test", false, false, false)
			app.HA = scenario.global
			app.Availability = scenario.overrides
			app.Services.SQL = false
			app = application.WithWorkloadComponents(app, "api")
			app = application.WithMetricsSource(app, "application", "api", 8080, "/metrics")
			issuer := serviceissuer.New(t)

			driver := NewDriverAt(runtime, app, issuer, dataDir, namespace)
			resource := capability.Resource{
				Application: app.Name,
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
					Scheme:    "http",
					Port:      8080,
					Path:      "/metrics",
				},
			}
			if err := driver.Preflight(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			if err := driver.Provision(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := DestroyAllSharedProvidersAt(context.Background(), runtime, dataDir, namespace); err != nil {
					t.Errorf("safe destroy: %v", err)
				}
			}()

			containerName := "baseharbor-prometheus-ha-target"
			script := `from http.server import BaseHTTPRequestHandler, HTTPServer

payload = b"""# TYPE baseharbor_ha_acceptance gauge
baseharbor_ha_acceptance 1
# EOF
"""

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/metrics":
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/openmetrics-text; version=1.0.0; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        pass

HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
`
			network := application.MetricsProviderNetworkNameForNamespace(app, namespace)
			cmd := exec.CommandContext(
				ctx,
				string(runtime.Kind()), "run", "-d", "--rm",
				"--name", containerName,
				"--network", network,
				"--network-alias", application.MetricsTargetAlias(app, "api"),
				"docker.io/library/python:3.13-alpine",
				"python", "-c", script,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("start Prometheus HA target: %v\\n%s", err, output)
			}
			defer func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				_ = exec.CommandContext(cleanupCtx, string(runtime.Kind()), "rm", "-f", containerName).Run()
			}()

			if err := driver.Bind(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			waitPrometheusHAReady(t, ctx, driver, resource, binding)

			files, err := ExistingProviderFilesAt(dataDir, namespace, app)
			if err != nil {
				t.Fatal(err)
			}
			placement, err := PlacementForAt(dataDir, namespace, app)
			if err != nil {
				t.Fatal(err)
			}

			inventory, err := runtime.ListRuntimeContainers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			actual := 0
			for _, container := range inventory {
				if container.Project != placement.Project || !container.Running {
					continue
				}
				n, err := strconv.Atoi(strings.TrimPrefix(container.Service, "prometheus-"))
				if strings.HasPrefix(container.Service, "prometheus-") && err == nil && n > 0 {
					actual++
				}
			}
			if actual != scenario.members {
				t.Fatalf("native running data/receiver members=%d want=%d", actual, scenario.members)
			}
			before, err := os.ReadFile(files.Compose)
			if err != nil {
				t.Fatal(err)
			}
			if err := driver.Provision(ctx, resource, binding); err != nil {
				t.Fatalf("repeated up: %v", err)
			}
			after, err := os.ReadFile(files.Compose)
			if err != nil || string(before) != string(after) {
				t.Fatalf("repeated up changed topology: %v", err)
			}
			waitPrometheusHAReady(t, ctx, driver, resource, binding)

		})
	}
}
