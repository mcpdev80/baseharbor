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

func TestMongoDBRuntimeSemanticAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_MONGODB_ACCEPTANCE") != "1" {
		t.Skip("MongoDB runtime acceptance requires BASEHARBOR_MONGODB_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}

	store := Store{
		Root:      filepath.Join(t.TempDir(), "apps"),
		Namespace: "mongodb-acceptance",
	}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "mongodb-ci",
		Environment:   "dev",
		Services: Services{
			DocumentDatabase: true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	files, err := EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatalf("materialize MongoDB runtime: %v", err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatalf("validate MongoDB runtime: %v", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatalf("start MongoDB runtime: %v", err)
	}
	defer func() {
		if t.Failed() && os.Getenv("BASEHARBOR_MONGODB_KEEP_ON_FAILURE") == "1" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy MongoDB runtime: %v", err)
		}
	}()

	deadline := time.Now().Add(90 * time.Second)
	for {
		err = VerifyMongoDBRuntime(ctx, m, files)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("MongoDB runtime never passed semantic verification: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("MongoDB runtime verification timed out: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}

	if err := VerifyWorkloadServiceBindings(m, files); err != nil {
		t.Fatalf("verify MongoDB workload service binding: %v", err)
	}

	running, err := runtime.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		t.Fatalf("inspect MongoDB runtime services: %v", err)
	}
	sort.Strings(running)
	want := []string{"mongodb", "mongodb-access"}
	if len(running) != len(want) {
		t.Fatalf("unexpected MongoDB runtime services: got %#v want %#v", running, want)
	}
	for i := range want {
		if running[i] != want[i] {
			t.Fatalf("unexpected MongoDB runtime services: got %#v want %#v", running, want)
		}
	}

	resources, err := InspectOwnedRuntimeResourcesForFiles(ctx, runtime, m, files)
	if err != nil {
		t.Fatalf("inspect MongoDB owned resources: %v", err)
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
		t.Fatalf("MongoDB owned resource shape = containers:%d volumes:%d resources:%#v", containers, volumes, resources)
	}
}
