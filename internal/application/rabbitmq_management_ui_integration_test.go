package application

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/provideroperation"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRabbitMQManagementUIRuntimeAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_RABBITMQ_UI_ACCEPTANCE") != "1" {
		t.Skip("RabbitMQ management UI acceptance requires BASEHARBOR_RABBITMQ_UI_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "rabbitmq-ui-acceptance"}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "rabbitmq-ui-ci",
		Environment:   "dev",
		Services: Services{
			MessagingQueue:        true,
			MessagingManagementUI: true,
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
	if err := ReconcileRabbitMQCredentials(ctx, provideroperation.New(runtime, files.Project, files.Compose, files.Env), m, files); err != nil {
		t.Fatalf("reconcile RabbitMQ credentials: %v", err)
	}
	defer func() {
		if t.Failed() && os.Getenv("BASEHARBOR_RABBITMQ_UI_KEEP_ON_FAILURE") == "1" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy RabbitMQ UI runtime: %v", err)
		}
	}()

	deadline := time.Now().Add(90 * time.Second)
	for {
		err = VerifyApplicationManagementUIs(ctx, m, files)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("RabbitMQ management UI never became ready: %v", err)
		}
		time.Sleep(time.Second)
	}

	surfaces, err := ApplicationManagementUISurfaces(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(surfaces) != 1 || surfaces[0].Service != "rabbitmq" || surfaces[0].Authentication != "rabbitmq-native" {
		t.Fatalf("unexpected RabbitMQ management surfaces: %#v", surfaces)
	}

	running, err := runtime.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(running)
	want := []string{"rabbitmq", "rabbitmq-access", "rabbitmq-ui"}
	if len(running) != len(want) {
		t.Fatalf("unexpected RabbitMQ UI runtime services: got %#v want %#v", running, want)
	}
	for i := range want {
		if running[i] != want[i] {
			t.Fatalf("unexpected RabbitMQ UI runtime services: got %#v want %#v", running, want)
		}
	}
}
