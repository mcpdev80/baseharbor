package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
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

	app := application.New("prometheus-ha-ci", "test", false, false, false)
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
		_ = DestroyAllSharedProvidersAt(context.Background(), runtime, dataDir, namespace)
	}()

	containerName := "baseharbor-prometheus-ha-target"
	script := `from http.server import BaseHTTPRequestHandler, HTTPServer
import os

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
        if os.path.exists("/tmp/down"):
            self.send_response(503)
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
	environment := readPrometheusEnvForHATest(t, files.Env)
	if err := runtime.StopProjectFilesSelected(ctx, placement.Project, files.Dir, environment, []string{"prometheus-1"}, files.Compose); err != nil {
		t.Fatalf("stop Prometheus member: %v", err)
	}
	waitPrometheusHAReady(t, ctx, driver, resource, binding)

	if err := runtime.UpProjectFilesSelected(ctx, placement.Project, files.Dir, environment, []string{"prometheus-1"}, files.Compose); err != nil {
		t.Fatalf("restart Prometheus member: %v", err)
	}
	waitPrometheusHAReady(t, ctx, driver, resource, binding)

	accessPolicy, err := serviceaccess.Resolve(app.Environment, "prometheus", serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	accessPolicy.ServerName = "prometheus"
	accessDir := filepath.Join(files.Dir, "service-access", "pki")
	oldMaterial, err := serviceaccess.ExistingTLSMaterial(accessPolicy, accessDir)
	if err != nil {
		t.Fatal(err)
	}
	oldCA := mustReadPrometheusPKIFile(t, oldMaterial.CA)
	oldClientCert := mustReadPrometheusPKIFile(t, oldMaterial.ClientCertificate)
	oldClientKey := mustReadPrometheusPKIFile(t, oldMaterial.ClientKey)

	issuer.Rotate(t)
	if err := driver.RotatePKI(ctx); err != nil {
		t.Fatalf("rotate Prometheus PKI: %v", err)
	}
	waitPrometheusHAReady(t, ctx, driver, resource, binding)

	newMaterial, err := serviceaccess.ExistingTLSMaterial(accessPolicy, accessDir)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		t.Fatal(err)
	}
	client, err := providerHTTPClient(app, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPrometheusAlertRuleAPI(ctx, client, endpoint); err != nil {
		t.Fatalf("Prometheus did not load and evaluate the managed alert rule: %v", err)
	}
	if output, err := exec.CommandContext(ctx, string(runtime.Kind()), "exec", containerName, "touch", "/tmp/down").CombinedOutput(); err != nil {
		t.Fatalf("mark test metrics target unavailable: %v\n%s", err, output)
	}
	waitPrometheusAlertFiring(t, ctx, client, endpoint)
	restartStartedAt := time.Now()
	members := []string{"prometheus-1", "prometheus-2"}
	if err := runtime.StopProjectFilesSelected(ctx, placement.Project, files.Dir, environment, members, files.Compose); err != nil {
		t.Fatalf("stop Prometheus members with an active alert: %v", err)
	}
	if err := runtime.UpProjectFilesSelected(ctx, placement.Project, files.Dir, environment, members, files.Compose); err != nil {
		t.Fatalf("restart Prometheus members with existing TSDB data: %v", err)
	}
	waitPrometheusRuleAndAlert(t, ctx, client, endpoint)
	assertPrometheusAlertHistorySurvivedRestart(t, ctx, client, endpoint, restartStartedAt)
	if output, err := exec.CommandContext(ctx, string(runtime.Kind()), "exec", containerName, "rm", "-f", "/tmp/down").CombinedOutput(); err != nil {
		t.Fatalf("restore test metrics target: %v\n%s", err, output)
	}
	waitPrometheusHAReady(t, ctx, driver, resource, binding)

	assertPrometheusRetiredMaterialRejected(t, ctx, accessPolicy, endpoint, newMaterial, oldCA, nil, nil, "retired CA")
	assertPrometheusRetiredMaterialRejected(t, ctx, accessPolicy, endpoint, newMaterial, nil, oldClientCert, oldClientKey, "retired client certificate")
}

func mustReadPrometheusPKIFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertPrometheusRetiredMaterialRejected(t *testing.T, ctx context.Context, policy serviceaccess.Policy, endpoint string, current serviceaccess.TLSMaterial, oldCA, oldClientCert, oldClientKey []byte, label string) {
	t.Helper()
	dir := t.TempDir()
	material := current
	if oldCA != nil {
		material.CA = filepath.Join(dir, "old-ca.pem")
		if err := os.WriteFile(material.CA, oldCA, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if oldClientCert != nil {
		material.ClientCertificate = filepath.Join(dir, "old-client.pem")
		material.ClientKey = filepath.Join(dir, "old-client-key.pem")
		if err := os.WriteFile(material.ClientCertificate, oldClientCert, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(material.ClientKey, oldClientKey, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	client, err := serviceaccess.NewHTTPClientForPolicy(material, policy)
	if err != nil {
		t.Fatal(err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/-/ready", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("%s still authenticates/validates the rotated Prometheus endpoint", label)
	}
}

func waitPrometheusAlertFiring(t *testing.T, ctx context.Context, client *http.Client, endpoint string) {
	t.Helper()
	query := `ALERTS{alertname="BaseHarborTargetUnavailable",alertstate="firing"} == 1`
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		firing, err := queryUp(ctx, client, endpoint, query)
		if err == nil && firing {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for Prometheus alert to fire: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatal("Prometheus did not evaluate the unavailable-target alert")
}

func waitPrometheusRuleAndAlert(t *testing.T, ctx context.Context, client *http.Client, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		ruleErr := verifyPrometheusAlertRuleAPI(ctx, client, endpoint)
		firing, alertErr := queryUp(ctx, client, endpoint, `ALERTS{alertname="BaseHarborTargetUnavailable",alertstate="firing"} == 1`)
		if ruleErr == nil && alertErr == nil && firing {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for Prometheus rule and alert after restart: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatal("Prometheus did not reload and evaluate the persisted alert rule after restart")
}

func assertPrometheusAlertHistorySurvivedRestart(t *testing.T, ctx context.Context, client *http.Client, endpoint string, restartStartedAt time.Time) {
	t.Helper()
	query := `ALERTS{alertname="BaseHarborTargetUnavailable",alertstate="firing"}`
	values := url.Values{
		"query": []string{query},
		"start": []string{fmt.Sprintf("%f", float64(restartStartedAt.Add(-2*time.Minute).UnixNano())/1e9)},
		"end":   []string{fmt.Sprintf("%f", float64(time.Now().UnixNano())/1e9)},
		"step":  []string{"5"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/api/v1/query_range?"+values.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("query persisted Prometheus alert history: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("query persisted Prometheus alert history: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][]any `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode Prometheus alert history: %v", err)
	}
	if payload.Status != "success" {
		t.Fatalf("Prometheus alert history status = %q, want success", payload.Status)
	}
	var beforeRestart, afterRestart bool
	for _, series := range payload.Data.Result {
		for _, sample := range series.Values {
			if len(sample) != 2 {
				continue
			}
			timestamp, ok := sample[0].(float64)
			if !ok {
				continue
			}
			if timestamp < float64(restartStartedAt.UnixNano())/1e9 {
				beforeRestart = true
			} else {
				afterRestart = true
			}
		}
	}
	if !beforeRestart || !afterRestart {
		t.Fatalf("Prometheus alert history before restart=%v after restart=%v; want both from the persistent TSDB", beforeRestart, afterRestart)
	}
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
