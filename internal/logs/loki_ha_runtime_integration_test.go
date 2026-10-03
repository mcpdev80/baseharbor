package logs_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestLokiHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_LOKI_HA_ACCEPTANCE") != "1" {
		t.Skip("Loki HA acceptance requires BASEHARBOR_LOKI_HA_ACCEPTANCE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	runtime, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	namespace := "loki-ha-acceptance"
	t.Setenv(application.LogsEnabledEnv, "true")

	m := application.WithLogsCollection(application.New("loki-ha-ci", "test", false, false, false), "application")
	m.HA = true
	driver := logs.NewDriverAt(runtime, m, serviceissuer.New(t), dataDir, namespace)
	resource := capability.Resource{Application: m.Name, Kind: capability.Logs, Name: "api", Provider: capability.ProviderLoki}
	binding := capability.Binding{
		Resource: resource,
		Workload: "service/api",
		Logs:     &capability.LogsBinding{Direction: "collect", Format: "syslog-rfc5424", Service: "api"},
	}
	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("provision Loki HA: %v", err)
	}
	defer func() {
		_ = logs.DestroyProviderAt(context.Background(), runtime, dataDir, namespace, m)
	}()
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}

	registration, err := logs.ApplicationRegistrationAt(dataDir, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	workdir := t.TempDir()
	composeFile := filepath.Join(workdir, "compose.yaml")
	envFile := filepath.Join(workdir, "runtime.env")
	project := application.WorkloadProjectName(m)
	yaml := strings.ReplaceAll(`services:
  api:
    image: busybox:1.37
    command: ["sh", "-c", "while true; do echo baseharbor-loki-ha-acceptance; sleep 1; done"]
    logging:
      driver: syslog
      options:
        syslog-address: "tcp://127.0.0.1:PORT"
        syslog-format: rfc5424
        tag: "api"
`, "PORT", fmt.Sprint(registration.SyslogPort))
	if err := os.WriteFile(composeFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.UpProject(ctx, project, composeFile, envFile); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = runtime.DestroyProject(context.Background(), project, composeFile, envFile)
	}()

	files, err := logs.ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	placement, err := logs.PlacementForAt(dataDir, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	env := readLokiHAEnv(t, files.Env)
	diagnose := func() string {
		diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer diagnosticCancel()
		detail := runtime.DiagnosticsProject(diagnosticCtx, placement.Project, files.Compose, files.Env)
		metrics, metricsErr := runtime.ExecProject(
			diagnosticCtx,
			placement.Project,
			files.Compose,
			files.Env,
			"alloy",
			"/bin/bash",
			"-lc",
			"exec 3<>/dev/tcp/127.0.0.1/12345; printf 'GET /metrics HTTP/1.0\\r\\nHost: localhost\\r\\n\\r\\n' >&3; cat <&3",
		)
		if metricsErr != nil {
			return detail + "\nalloy metrics: " + metricsErr.Error()
		}
		var relevant []string
		for _, line := range strings.Split(metrics, "\n") {
			if strings.Contains(line, "loki_source_syslog_") || strings.Contains(line, "loki_write_") {
				relevant = append(relevant, line)
			}
		}
		if len(relevant) > 0 {
			detail += "\nalloy metrics:\n" + strings.Join(relevant, "\n")
		}
		return detail
	}

	emitLokiHAProbes(t, registration.SyslogPort)
	waitLokiHA(t, ctx, driver, resource, binding, diagnose)
	if err := runtime.StopProjectFilesSelected(ctx, placement.Project, files.Dir, env, []string{"loki-2"}, files.Compose); err != nil {
		t.Fatalf("stop Loki member: %v", err)
	}
	emitLokiHAProbes(t, registration.SyslogPort)
	waitLokiHA(t, ctx, driver, resource, binding, diagnose)
	if err := runtime.UpProjectFilesSelected(ctx, placement.Project, files.Dir, env, []string{"loki-2"}, files.Compose); err != nil {
		t.Fatalf("restart Loki member: %v", err)
	}
	emitLokiHAProbes(t, registration.SyslogPort)
	waitLokiHA(t, ctx, driver, resource, binding, diagnose)

	if err := driver.RotateStorageCredentials(ctx); err != nil {
		t.Fatalf("rotate Loki platform-storage credentials: %v", err)
	}
	emitLokiHAProbes(t, registration.SyslogPort)
	waitLokiHA(t, ctx, driver, resource, binding, diagnose)

	accessPolicy, err := serviceaccess.Resolve(m.Environment, "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	accessDir := filepath.Join(files.Dir, "service-access", "pki")
	oldMaterial, err := serviceaccess.ExistingTLSMaterial(accessPolicy, accessDir)
	if err != nil {
		t.Fatal(err)
	}
	oldCA := mustReadLokiPKIFile(t, oldMaterial.CA)
	oldClientCert := mustReadLokiPKIFile(t, oldMaterial.ClientCertificate)
	oldClientKey := mustReadLokiPKIFile(t, oldMaterial.ClientKey)

	issuer.Rotate(t)
	if err := driver.RotateAccessPKI(ctx); err != nil {
		t.Fatalf("rotate Loki access PKI: %v", err)
	}
	emitLokiHAProbes(t, registration.SyslogPort)
	waitLokiHA(t, ctx, driver, resource, binding, diagnose)

	newMaterial, err := serviceaccess.ExistingTLSMaterial(accessPolicy, accessDir)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := logs.ProviderEndpoint(files)
	if err != nil {
		t.Fatal(err)
	}
	assertLokiRetiredMaterialRejected(t, ctx, accessPolicy, endpoint, newMaterial, oldCA, nil, nil, "retired CA")
	assertLokiRetiredMaterialRejected(t, ctx, accessPolicy, endpoint, newMaterial, nil, oldClientCert, oldClientKey, "retired client certificate")
}

func mustReadLokiPKIFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertLokiRetiredMaterialRejected(t *testing.T, ctx context.Context, policy serviceaccess.Policy, endpoint string, current serviceaccess.TLSMaterial, oldCA, oldClientCert, oldClientKey []byte, label string) {
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
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/loki/api/v1/status/buildinfo", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("%s still authenticates/validates the rotated Loki endpoint", label)
	}
}

func emitLokiHAProbes(t *testing.T, port int) {
	t.Helper()
	for i := 0; i < 5; i++ {
		emitLokiHAProbe(t, port)
		time.Sleep(250 * time.Millisecond)
	}
}

func emitLokiHAProbe(t *testing.T, port int) {
	t.Helper()
	conn, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect to Alloy syslog ingress: %v", err)
	}
	defer conn.Close()
	message := fmt.Sprintf(
		"<14>1 %s localhost api - - - baseharbor-loki-ha-probe\n",
		time.Now().UTC().Format("2006-01-02T15:04:05.000000Z"),
	)
	if _, err := conn.Write([]byte(message)); err != nil {
		t.Fatalf("write Alloy syslog ingress probe: %v", err)
	}
}

func waitLokiHA(t *testing.T, ctx context.Context, driver interface {
	Verify(context.Context, capability.Resource, capability.Binding) error
}, resource capability.Resource, binding capability.Binding, diagnose func() string) {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if last = driver.Verify(ctx, resource, binding); last == nil {
			return
		}
		time.Sleep(time.Second)
	}
	detail := ""
	if diagnose != nil {
		detail = strings.TrimSpace(diagnose())
	}
	if detail != "" {
		t.Fatalf("stable Loki endpoint lost ingestion/query continuity: %v\n%s", last, detail)
	}
	t.Fatalf("stable Loki endpoint lost ingestion/query continuity: %v", last)
}

func readLokiHAEnv(t *testing.T, path string) map[string]string {
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
			t.Fatalf("invalid Loki env line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}
