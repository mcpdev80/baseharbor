package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
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

	compose, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("detect Compose runtime: %v", err)
	}

	var containers []string
	defer func() {
		for _, name := range containers {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := exec.CommandContext(cleanupCtx, string(compose.Kind()), "rm", "-f", name).Run(); err != nil {
				t.Errorf("owned metrics workload cleanup: %v", err)
			}
			cleanupCancel()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		if err := DestroyAllSharedProviders(cleanupCtx, compose); err != nil {
			t.Errorf("destroy shared Prometheus provider: %v", err)
		}
	}()

	issuer := serviceissuer.New(t)
	apps := []string{"metrics-alpha", "metrics-beta"}
	for _, name := range apps {
		m := application.New(name, "test", false, false, false)
		m.Services.SQL = false
		m = application.WithWorkloadComponents(m, "api")
		m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")

		driver := NewDriver(compose, m, issuer)
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
				Scheme:    "http",
				Port:      8080,
				Path:      "/metrics",
			},
		}
		t.Logf("%s: preflight", m.Name)
		if err := driver.Preflight(ctx, resource, binding); err != nil {
			t.Fatalf("%s preflight: %v", m.Name, err)
		}
		t.Logf("%s: provision Prometheus", m.Name)
		if err := driver.Provision(ctx, resource, binding); err != nil {
			t.Fatalf("%s provision: %v", m.Name, err)
		}
		placement, err := PlacementFor(m)
		if err != nil {
			t.Fatalf("%s placement: %v", m.Name, err)
		}
		providerFiles, err := ExistingProviderFiles(m)
		if err != nil {
			t.Fatal(err)
		}
		members, err := providertopology.ExistingMembers(providerFiles.Compose, "prometheus")
		if err != nil {
			t.Fatal(err)
		}
		for _, service := range append(providertopology.Names("prometheus", members), "prometheus-access") {
			if err := containersecurity.VerifyComposeService(ctx, placement.Project, service, containersecurity.Requirements{ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true}); err != nil {
				t.Fatalf("%s actual %s runtime security: %v", m.Name, service, err)
			}
		}

		containerName := "baseharbor-" + name + "-" + m.ApplicationID[:8]
		script := `from http.server import BaseHTTPRequestHandler, HTTPServer

payload = b"# TYPE baseharbor_acceptance_metric gauge\nbaseharbor_acceptance_metric 1\n# EOF\n"

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
		cmd := exec.CommandContext(
			ctx,
			string(compose.Kind()), "run", "-d", "--rm",
			"--name", containerName,
			"--network", application.MetricsProviderNetworkName(m),
			"--network-alias", application.MetricsTargetAlias(m, "api"),
			"docker.io/library/python:3.13-alpine",
			"python", "-c", script,
		)
		t.Logf("%s: start metrics endpoint", m.Name)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s start metrics endpoint: %v\n%s", m.Name, err, output)
		}
		containers = append(containers, containerName)

		t.Logf("%s: bind target", m.Name)
		if err := driver.Bind(ctx, resource, binding); err != nil {
			t.Fatalf("%s bind: %v", m.Name, err)
		}
		t.Logf("%s: verify real scrape", m.Name)
		if err := driver.Verify(ctx, resource, binding); err != nil {
			t.Fatalf("%s verify real scrape/ingestion: %v", m.Name, err)
		}
		instance, err := driver.realization.Existing(ctx)
		if err != nil {
			t.Fatal(err)
		}
		query := fmt.Sprintf(`baseharbor_acceptance_metric{baseharbor_application=%q,baseharbor_environment=%q}`, m.Name, m.Environment)
		if ok, err := queryUp(ctx, driver.client, instance.Endpoint, query); err != nil || !ok {
			t.Fatalf("actual isolated consumer metric missing: %v", err)
		}
		policy, err := serviceaccess.Resolve(m.Environment, "prometheus", serviceaccess.AuthenticationMTLS)
		if err != nil {
			t.Fatal(err)
		}
		material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(providerFiles.Dir, "service-access", "pki"))
		if err != nil {
			t.Fatal(err)
		}
		ca, err := os.ReadFile(material.CA)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca) {
			t.Fatal("actual receiver CA is invalid")
		}
		anonymous := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: policy.ServerName, MinVersion: tls.VersionTLS12}}, Timeout: 3 * time.Second}
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, instance.Endpoint+"/api/v1/query?query=up", nil)
		request.Host = policy.ServerName
		response, probeErr := anonymous.Do(request)
		if probeErr == nil {
			response.Body.Close()
			if response.StatusCode < 400 {
				t.Fatal("managed metrics receiver accepted anonymous access")
			}
		}
		t.Log("actual isolated application gauge equals 1 through verified mTLS; receiver rejects missing client identity")
	}

	files, err := ExistingProviderFiles(application.Manifest{Name: "metrics-alpha", Environment: "test"})
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
		m := application.New(name, "test", false, false, false)
		target := filepath.Join(files.TargetsDir, targetFileName(m, "application"))
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("%s target missing from shared provider state: %v", name, err)
		}
	}
	fmt.Fprintln(os.Stdout, "verified two isolated application targets through one shared Prometheus provider")
}
