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

func TestRabbitMQRuntimeSemanticAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_RABBITMQ_ACCEPTANCE") != "1" {
		t.Skip("RabbitMQ runtime acceptance requires BASEHARBOR_RABBITMQ_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}

	store := Store{
		Root:      filepath.Join(t.TempDir(), "apps"),
		Namespace: "rabbitmq-acceptance",
	}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "rabbitmq-ci",
		Environment:   "dev",
		Services: Services{
			MessagingQueue:  true,
			MessagingPubSub: true,
			MessagingStream: true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	files, err := EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatalf("materialize RabbitMQ runtime: %v", err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatalf("validate RabbitMQ runtime: %v", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatalf("start RabbitMQ runtime: %v", err)
	}
	if err := ReconcileRabbitMQCredentials(ctx, runtime, m, files); err != nil {
		t.Fatalf("reconcile RabbitMQ credentials: %v", err)
	}
	defer func() {
		if t.Failed() && os.Getenv("BASEHARBOR_RABBITMQ_KEEP_ON_FAILURE") == "1" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy RabbitMQ runtime: %v", err)
		}
	}()

	deadline := time.Now().Add(90 * time.Second)
	for {
		err = VerifyRabbitMQRuntime(ctx, m, files)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("RabbitMQ runtime never passed semantic verification: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("RabbitMQ runtime verification timed out: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}

	if err := VerifyWorkloadServiceBindings(m, files); err != nil {
		t.Fatalf("verify RabbitMQ workload service binding: %v", err)
	}

	running, err := runtime.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		t.Fatalf("inspect RabbitMQ runtime services: %v", err)
	}
	sort.Strings(running)
	want := []string{"rabbitmq", "rabbitmq-access"}
	if len(running) != len(want) {
		t.Fatalf("unexpected RabbitMQ runtime services: got %#v want %#v", running, want)
	}
	for i := range want {
		if running[i] != want[i] {
			t.Fatalf("unexpected RabbitMQ runtime services: got %#v want %#v", running, want)
		}
	}

	resources, err := InspectOwnedRuntimeResourcesForFiles(ctx, runtime, m, files)
	if err != nil {
		t.Fatalf("inspect RabbitMQ owned resources: %v", err)
	}
	var containers, volumes int
	for _, resource := range resources {
		switch resource.Kind {
		case "container":
			containers++
		case "volume":
			volumes++
		}
	}
	if containers != 2 || volumes != 1 {
		t.Fatalf("RabbitMQ owned resource shape = containers:%d volumes:%d resources:%#v", containers, volumes, resources)
	}
}
