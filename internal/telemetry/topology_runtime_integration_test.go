package telemetry

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/capability"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOTelDefaultTopologyRuntimeAcceptanceInCI(t *testing.T) {
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
		{name: "false", global: false, overrides: map[string]availability.Override{"telemetry": {HA: &no}}, members: 1},
		{name: "override-single", global: true, overrides: map[string]availability.Override{"telemetry": {HA: &no}}, members: 1},
		{name: "override-ha", overrides: map[string]availability.Override{"telemetry": {HA: &yes}}, members: 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			runtime, err := testruntime.Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dataDir := filepath.Join(t.TempDir(), "data")
			namespace := "otel-topology-" + scenario.name
			issuer := serviceissuer.New(t)
			app := application.WithOTLPTelemetry(application.Manifest{
				Version:       application.CurrentVersion,
				ApplicationID: application.MustNewApplicationID(),
				Name:          "otel-topology-ci",
				Environment:   "test",
				HA:            scenario.global,
				Workload:      application.WorkloadConfig{Components: []string{"api"}},
			}, "traces")
			app.Availability = scenario.overrides
			store := application.Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: namespace}
			appFiles, err := application.EnsureRuntime(ctx, issuer, store, app)
			if err != nil {
				t.Fatal(err)
			}

			driver := NewDriverAt(runtime, app, appFiles, issuer, dataDir, namespace)
			resource := capability.Resource{
				Application: app.Name,
				Kind:        capability.TelemetryOTLP,
				Name:        "default",
				Provider:    capability.ProviderOTelCollector,
			}
			binding := capability.Binding{
				Resource: resource,
				Workload: "application/" + app.Name,
				TelemetryOTLP: &capability.OTLPTelemetryBinding{
					Direction: "export",
					Protocol:  "http/protobuf",
					Signals:   []string{"traces"},
				},
			}
			if err := driver.Preflight(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			if err := driver.Provision(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := DestroySharedProviderAt(context.Background(), runtime, dataDir, namespace); err != nil {
					t.Errorf("safe destroy: %v", err)
				}
			}()
			if err := driver.Bind(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			waitOTelHAReady(t, ctx, driver, resource, binding)

			files, err := ExistingProviderFilesAt(dataDir, namespace)
			if err != nil {
				t.Fatal(err)
			}

			inventory, err := runtime.ListRuntimeContainers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			actual := 0
			for _, container := range inventory {
				if container.Project != files.Project || !container.Running {
					continue
				}
				n, err := strconv.Atoi(strings.TrimPrefix(container.Service, "otel-collector-"))
				if strings.HasPrefix(container.Service, "otel-collector-") && err == nil && n > 0 {
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
			waitOTelHAReady(t, ctx, driver, resource, binding)

		})
	}
}
