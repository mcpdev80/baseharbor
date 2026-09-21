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
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
)

func TestManagedLokiIngestsRealComposeWorkloadLogs(t *testing.T) {
	if os.Getenv("BASEHARBOR_LOGS_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_LOGS_INTEGRATION=1 for real Docker/Podman Loki acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.LogsEnabledEnv, "true")

	m := application.WithLogsCollection(application.New("logs-acceptance", "dev", false, false, false), "application")
	driver := logs.NewDriver(compose, m)
	resource := capability.Resource{Application: m.Name, Kind: capability.Logs, Name: "api", Provider: capability.ProviderLoki}
	binding := capability.Binding{
		Resource: resource,
		Workload: "service/api",
		Logs:     &capability.LogsBinding{Direction: "collect", Format: "syslog-rfc5424", Service: "api"},
	}

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	provisionCtx, cancelProvision := context.WithTimeout(ctx, 90*time.Second)
	err = driver.Provision(provisionCtx, resource, binding)
	cancelProvision()
	if err != nil {
		placement, placementErr := logs.PlacementFor(m)
		files, filesErr := logs.ExistingProviderFiles(m)
		diagnostics := ""
		if placementErr == nil && filesErr == nil {
			diagCtx, cancelDiag := context.WithTimeout(context.Background(), 10*time.Second)
			status, statusErr := compose.StatusProject(diagCtx, placement.Project, files.Compose, files.Env)
			providerLogs, logsErr := compose.LogsProject(diagCtx, placement.Project, files.Compose, files.Env, "loki", "alloy")
			cancelDiag()
			diagnostics = fmt.Sprintf("\ncompose ps (err=%v):\n%s\nprovider logs (err=%v):\n%s", statusErr, status, logsErr, providerLogs)
		} else {
			diagnostics = fmt.Sprintf("\nprovider diagnostics unavailable: placement=%v files=%v", placementErr, filesErr)
		}
		t.Fatalf("provision Loki provider: %v%s", err, diagnostics)
	}
	for _, service := range []string{"loki", "alloy"} {
		if err := containersecurity.VerifyComposeService(ctx, "baseharbor-logs", service, containersecurity.Requirements{
			ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true,
		}); err != nil {
			t.Fatalf("%s runtime security: %v", service, err)
		}
	}
	defer func() { _ = logs.DestroyProvider(context.Background(), compose, m) }()
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	registration, err := logs.ApplicationRegistration(m)
	if err != nil {
		t.Fatal(err)
	}

	workdir := t.TempDir()
	composeFile := filepath.Join(workdir, "compose.yaml")
	envFile := filepath.Join(workdir, "runtime.env")
	project := "baseharbor-logs-acceptance-workload"
	yaml := `services:
  api:
    image: busybox:1.37
    command: ["sh", "-c", "while true; do echo baseharbor-loki-acceptance; sleep 1; done"]
    logging:
      driver: syslog
      options:
        syslog-address: "udp://127.0.0.1:PORT"
        syslog-format: rfc5424
        tag: "api"
`
	yaml = strings.ReplaceAll(yaml, "PORT", fmt.Sprint(registration.SyslogPort))
	if err := os.WriteFile(composeFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := compose.UpProject(ctx, project, composeFile, envFile); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = compose.DestroyProject(context.Background(), project, composeFile, envFile) }()

	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
}
