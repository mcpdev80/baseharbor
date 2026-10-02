package logs_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
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

	m := application.WithLogsCollection(application.New("loki-ha-ci", "dev", false, false, false), "application")
	m.HA = true
	driver := logs.NewDriverAt(runtime, m, serviceissuer.New(t), dataDir, namespace)
	resource := capability.Resource{Application: m.Name, Kind: capability.Logs, Name: "api", Provider: capability.ProviderLoki}
	binding := capability.Binding{
		Resource: resource,
		Workload: "service/api",
		Logs: &capability.LogsBinding{Direction: "collect", Format: "syslog-rfc5424", Service: "api"},
	}
	if err := driver.Preflight(ctx, resource, binding); err != nil { t.Fatal(err) }
	if err := driver.Provision(ctx, resource, binding); err != nil { t.Fatalf("provision Loki HA: %v", err) }
	defer func() {
		_ = logs.DestroyProviderAt(context.Background(), runtime, m, dataDir, namespace)
	}()
	if err := driver.Bind(ctx, resource, binding); err != nil { t.Fatal(err) }

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
        syslog-address: "udp://127.0.0.1:PORT"
        syslog-format: rfc5424
        tag: "api"
`, "PORT", fmt.Sprint(registration.SyslogPort))
	if err := os.WriteFile(composeFile, []byte(yaml), 0o600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(envFile, nil, 0o600); err != nil { t.Fatal(err) }
	if err := runtime.UpProject(ctx, project, composeFile, envFile); err != nil { t.Fatal(err) }
	defer func() {
		_ = runtime.DestroyProject(context.Background(), project, composeFile, envFile)
	}()

	waitLokiHA(t, ctx, driver, resource, binding)
	files, err := logs.ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	placement, err := logs.PlacementForAt(dataDir, namespace, m)
	if err != nil {
		t.Fatal(err)
	}
	env := readLokiHAEnv(t, files.Env)

	if err := runtime.StopProjectFilesSelected(ctx, placement.Project, files.Dir, env, []string{"loki-2"}, files.Compose); err != nil {
		t.Fatalf("stop Loki member: %v", err)
	}
	waitLokiHA(t, ctx, driver, resource, binding)
	if err := runtime.UpProjectFilesSelected(ctx, placement.Project, files.Dir, env, []string{"loki-2"}, files.Compose); err != nil {
		t.Fatalf("restart Loki member: %v", err)
	}
	waitLokiHA(t, ctx, driver, resource, binding)
}

func waitLokiHA(t *testing.T, ctx context.Context, driver interface {
	Verify(context.Context, capability.Resource, capability.Binding) error
}, resource capability.Resource, binding capability.Binding) {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if last = driver.Verify(ctx, resource, binding); last == nil {
			return
		}
		time.Sleep(time.Second)
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
