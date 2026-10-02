package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/provideroperation"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestValkeyHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_VALKEY_HA_ACCEPTANCE") != "1" {
		t.Skip("Valkey HA acceptance requires BASEHARBOR_VALKEY_HA_ACCEPTANCE=1")
	}
	useApplicationScopedDataProviders(t)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "valkey-ha-acceptance"}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "valkey-ha-ci",
		Environment:   "dev",
		HA:            true,
		Services: Services{
			KeyValue:             true,
			KeyValueManagementUI: true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	files, err := EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if t.Failed() && os.Getenv("BASEHARBOR_VALKEY_HA_KEEP_ON_FAILURE") == "1" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy Valkey HA runtime: %v", err)
		}
	}()

	op := provideroperation.New(runtime, files.Project, files.Compose, files.Env)
	waitValkeyHAReady(t, ctx, runtime, op, m, files)

	failedMember, err := ValkeyHAMaster(ctx, op, m, files, defaultServiceInstance)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.StopProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{failedMember}, files.Compose); err != nil {
		t.Fatalf("stop Valkey HA primary %s: %v", failedMember, err)
	}

	failoverDeadline := time.Now().Add(60 * time.Second)
	for {
		master, masterErr := ValkeyHAMaster(ctx, op, m, files, defaultServiceInstance)
		semanticErr := VerifyValkeyRuntime(ctx, runtime, m, files)
		uiErr := VerifyApplicationManagementUIs(ctx, m, files)
		if masterErr == nil && master != failedMember && semanticErr == nil && uiErr == nil {
			break
		}
		if time.Now().After(failoverDeadline) {
			t.Fatalf("Valkey stable binding/UI did not survive primary failure: master=%q masterErr=%v semantic=%v ui=%v gateway=%s", master, masterErr, semanticErr, uiErr, valkeyGatewayHealthDiagnostics(ctx, op, defaultServiceInstance))
		}
		time.Sleep(time.Second)
	}

	if err := runtime.UpProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{failedMember}, files.Compose); err != nil {
		t.Fatalf("restart Valkey HA member %s: %v", failedMember, err)
	}
	waitValkeyHAReady(t, ctx, runtime, op, m, files)
}

func waitValkeyHAReady(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, op valkeyHAProbeRuntime, m Manifest, files RuntimeFiles) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for {
		semanticErr := VerifyValkeyRuntime(ctx, runtime, m, files)
		haErr := VerifyValkeyHACluster(ctx, op, m, files)
		uiErr := VerifyApplicationManagementUIs(ctx, m, files)
		if semanticErr == nil && haErr == nil && uiErr == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Valkey HA did not become ready: semantic=%v ha=%v ui=%v gateway=%s", semanticErr, haErr, uiErr, valkeyGatewayHealthDiagnostics(ctx, op, defaultServiceInstance))
		}
		time.Sleep(time.Second)
	}
}

func valkeyGatewayHealthDiagnostics(ctx context.Context, op valkeyHAProbeRuntime, instance string) string {
	service := valkeyAccessService(instance)
	envState, envErr := op.Run(ctx, service, "sh", "-ec", `if [ -n "$VALKEY_HEALTH_PASSWORD" ]; then printf configured; else printf missing; fi`)
	stats, statsErr := op.Run(ctx, service, "sh", "-ec", `wget -qO- 'http://127.0.0.1:8404/stats;csv' 2>/dev/null | grep '^upstream,' || true`)
	if envErr != nil {
		envState = "probe-error"
	}
	if statsErr != nil {
		stats = "stats-error"
	}
	return "credential=" + strings.TrimSpace(envState) + " backends=" + strings.TrimSpace(stats)
}
