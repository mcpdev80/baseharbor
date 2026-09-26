package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestManagedCollectorRealOTLPExport(t *testing.T) {
	if os.Getenv("BASEHARBOR_OTLP_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_OTLP_INTEGRATION=1 to run real Collector acceptance")
	}

	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("BASEHARBOR_STATE_DIR", state)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect Compose runtime: %v", err)
	}
	defer func() {
		// Keep failed provider state alive until the acceptance workflow has
		// captured container status/logs. GitHub-hosted runners are ephemeral.
		if t.Failed() {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		if err := DestroySharedProvider(cleanupCtx, compose); err != nil {
			t.Errorf("destroy shared Collector provider: %v", err)
		}
	}()

	m := application.WithOTLPTelemetry(application.Manifest{
		Version:     1,
		Name:        "otlp-acceptance",
		Environment: "test",
		Workload:    application.WorkloadConfig{Services: []string{"api"}},
	}, "traces", "metrics", "logs")
	store := application.Store{Root: filepath.Join(state, "apps")}
	files, err := application.EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatalf("materialize application runtime: %v", err)
	}

	issuer := serviceissuer.New(t)
	driver := NewDriver(compose, m, files, issuer)
	binding := capability.Binding{
		Resource: capability.Resource{
			Application: m.Name,
			Kind:        capability.TelemetryOTLP,
			Name:        "default",
			Provider:    capability.ProviderOTelCollector,
		},
		Workload: "application/" + m.Name,
		TelemetryOTLP: &capability.OTLPTelemetryBinding{
			Direction: "export",
			Protocol:  "http/protobuf",
			Signals:   []string{"traces", "metrics", "logs"},
		},
	}
	resource := binding.Resource

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatalf("preflight Collector: %v", err)
	}
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("provision Collector: %v", err)
	}
	providerFiles, err := ExistingProviderFiles()
	if err != nil {
		t.Fatalf("load Collector provider files: %v", err)
	}
	if err := containersecurity.VerifyComposeService(ctx, providerFiles.Project, "otel-collector", containersecurity.Requirements{
		ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true,
	}); err != nil {
		t.Fatalf("Collector runtime security: %v", err)
	}
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatalf("bind Collector: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("verify real OTLP export: %v", err)
	}

	values, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatalf("read application environment: %v", err)
	}
	if len(values) == 0 {
		t.Fatal("standard OpenTelemetry application binding was not materialized")
	}
}
