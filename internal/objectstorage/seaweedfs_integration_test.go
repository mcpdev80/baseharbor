package objectstorage

import (
	"context"
	"os"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestManagedSeaweedFSRunsUnprivileged(t *testing.T) {
	if os.Getenv("BASEHARBOR_OBJECT_STORAGE_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_OBJECT_STORAGE_INTEGRATION=1 to run real SeaweedFS acceptance")
	}

	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect Compose runtime: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		if err := DestroySharedProvider(cleanupCtx, compose); err != nil {
			t.Errorf("destroy SeaweedFS provider: %v", err)
		}
	}()

	files, _, _, err := EnsureSharedProvider(ctx, compose, serviceissuer.New(t))
	if err != nil {
		t.Fatalf("provision SeaweedFS: %v", err)
	}
	if err := containersecurity.VerifyComposeService(ctx, files.Project, ProviderService, containersecurity.Requirements{
		ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true,
	}); err != nil {
		t.Fatalf("SeaweedFS runtime security: %v", err)
	}
}
