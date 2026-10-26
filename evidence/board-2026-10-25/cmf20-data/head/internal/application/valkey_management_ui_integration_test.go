package application

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestDurableValkeyManagementUIRuntimeAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_VALKEY_UI_ACCEPTANCE") != "1" {
		t.Skip("durable Valkey management UI acceptance requires BASEHARBOR_VALKEY_UI_ACCEPTANCE=1")
	}
	useApplicationScopedDataProviders(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "valkey-ui-acceptance"}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "valkey-ui-ci",
		Environment:   "dev",
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
		if t.Failed() && os.Getenv("BASEHARBOR_VALKEY_UI_KEEP_ON_FAILURE") == "1" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy durable Valkey UI runtime: %v", err)
		}
	}()

	deadline := time.Now().Add(120 * time.Second)
	for {
		uiErr := VerifyApplicationManagementUIs(ctx, m, files)
		valkeyErr := VerifyValkeyRuntime(ctx, runtime, m, files)
		if uiErr == nil && valkeyErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("durable Valkey management UI never became ready: ui=%v valkey=%v", uiErr, valkeyErr)
		}
		time.Sleep(time.Second)
	}

	surfaces, err := ApplicationManagementUISurfaces(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(surfaces) != 1 || surfaces[0].Service != "key-value" || surfaces[0].Authentication != "http-basic" {
		t.Fatalf("unexpected durable Valkey management surfaces: %#v", surfaces)
	}

	running, err := runtime.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(running)
	want := []string{"cache-ui", "cache-ui-access", "valkey", "valkey-access"}
	if len(running) != len(want) {
		t.Fatalf("unexpected durable Valkey UI runtime services: got %#v want %#v", running, want)
	}
	for i := range want {
		if running[i] != want[i] {
			t.Fatalf("unexpected durable Valkey UI runtime services: got %#v want %#v", running, want)
		}
	}
}
