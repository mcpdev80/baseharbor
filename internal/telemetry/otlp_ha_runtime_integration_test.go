package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestOTelHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_OTEL_HA_ACCEPTANCE") != "1" {
		t.Skip("OTel HA acceptance requires BASEHARBOR_OTEL_HA_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runtime, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	namespace := "otel-ha-acceptance"
	issuer := serviceissuer.New(t)
	app := application.WithOTLPTelemetry(application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "otel-ha-ci",
		Environment:   "dev",
		HA:            true,
		Workload:      application.WorkloadConfig{Components: []string{"api"}},
	}, "traces")
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
		_ = DestroySharedProviderAt(context.Background(), runtime, dataDir, namespace)
	}()
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	waitOTelHAReady(t, ctx, driver, resource, binding)

	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		t.Fatal(err)
	}
	environment := readProviderEnvForHATest(t, files.Env)
	if err := runtime.StopProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{"otel-collector-1"}, files.Compose); err != nil {
		t.Fatalf("stop OTel member: %v", err)
	}
	waitOTelHAReady(t, ctx, driver, resource, binding)

	if err := runtime.UpProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{"otel-collector-1"}, files.Compose); err != nil {
		t.Fatalf("restart OTel member: %v", err)
	}
	waitOTelHAReady(t, ctx, driver, resource, binding)
}

func waitOTelHAReady(t *testing.T, ctx context.Context, driver *Driver, resource capability.Resource, binding capability.Binding) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		last = driver.Verify(ctx, resource, binding)
		if last == nil {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("stable OTel endpoint did not preserve export continuity: %v", last)
}

func readProviderEnvForHATest(t *testing.T, path string) map[string]string {
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
