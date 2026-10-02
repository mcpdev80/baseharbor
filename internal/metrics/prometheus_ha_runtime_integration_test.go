package metrics

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestPrometheusHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_PROMETHEUS_HA_ACCEPTANCE") != "1" {
		t.Skip("Prometheus HA acceptance requires BASEHARBOR_PROMETHEUS_HA_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	runtime, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	namespace := "prometheus-ha-acceptance"
	t.Setenv(application.MetricsEnabledEnv, "true")

	app := application.New("prometheus-ha-ci", "dev", false, false, false)
	app.HA = true
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
		_ = DestroyAllSharedProvidersAt(context.Background(), runtime, dataDir, namespace)
	}()

	containerName := "baseharbor-prometheus-ha-target"
	script := `from http.server import BaseHTTPRequestHandler, HTTPServer

payload = b"# TYPE baseharbor_ha_acceptance gauge
baseharbor_ha_acceptance 1
# EOF
"

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
		"docker", "run", "-d", "--rm",
		"--name", containerName,
		"--network", network,
		"--network-alias", application.MetricsTargetAlias(app, "api"),
		"python:3.13-alpine",
		"python", "-c", script,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start Prometheus HA target: %v\\n%s", err, output)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", containerName).Run()
	}()

	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	waitPrometheusHAReady(t, ctx, driver, resource, binding)

	files, err := ExistingProviderFilesAt(dataDir, namespace, app)
	if err != nil {
		t.Fatal(err)
	}
	environment := readPrometheusEnvForHATest(t, files.Env)
	if err := runtime.StopProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{"prometheus-1"}, files.Compose); err != nil {
		t.Fatalf("stop Prometheus member: %v", err)
	}
	waitPrometheusHAReady(t, ctx, driver, resource, binding)

	if err := runtime.UpProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{"prometheus-1"}, files.Compose); err != nil {
		t.Fatalf("restart Prometheus member: %v", err)
	}
	waitPrometheusHAReady(t, ctx, driver, resource, binding)
}

func waitPrometheusHAReady(t *testing.T, ctx context.Context, driver *Driver, resource capability.Resource, binding capability.Binding) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		last = driver.Verify(ctx, resource, binding)
		if last == nil {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("stable Prometheus endpoint did not preserve query/scrape continuity: %v", last)
}

func readPrometheusEnvForHATest(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid provider env line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}
