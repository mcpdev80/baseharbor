package objectstorage

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/capability"
	runtimeprovider "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSeaweedFSDefaultTopologyRuntimeAcceptanceInCI(t *testing.T) {
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
		{name: "false", global: false, overrides: map[string]availability.Override{"object_storage": {HA: &no}}, members: 1},
		{name: "override-single", global: true, overrides: map[string]availability.Override{"object_storage": {HA: &no}}, members: 1},
		{name: "override-ha", overrides: map[string]availability.Override{"object_storage": {HA: &yes}}, members: 3},
	} {
		t.Run(scenario.name, func(t *testing.T) {

			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
			defer cancel()

			runtime, err := runtimeprovider.Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dataDir := filepath.Join(t.TempDir(), "data")
			namespace := "seaweedfs-topology-" + scenario.name
			issuer := serviceissuer.New(t)

			appStore := application.Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: namespace}
			app := application.Manifest{
				Version:       application.CurrentVersion,
				ApplicationID: application.MustNewApplicationID(),
				Name:          "seaweedfs-topology-ci",
				Environment:   "dev",
				HA:            scenario.global,
				Services: application.Services{
					ObjectStorage:             true,
					ObjectStorageManagementUI: true,
				},
			}
			app.Availability = scenario.overrides
			if err := app.Validate(); err != nil {
				t.Fatal(err)
			}
			appFiles, err := application.EnsureRuntime(ctx, issuer, appStore, app)
			if err != nil {
				t.Fatal(err)
			}

			driver := NewDriverAt(runtime, app, appFiles, issuer, dataDir, namespace)
			files, _, _, err := driver.EnsureSharedProvider(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if t.Failed() && os.Getenv("BASEHARBOR_SEAWEEDFS_HA_KEEP_ON_FAILURE") == "1" {
					return
				}
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cleanupCancel()
				if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
					t.Errorf("destroy SeaweedFS HA runtime: %v", err)
				}
			}()

			resource := capability.Resource{
				Application: app.Name,
				Kind:        capability.ObjectStorageS3,
				Name:        "default",
				Provider:    capability.ProviderSeaweedFS,
			}
			security := application.ObjectStorageSecureBinding(app, resource.Name)
			if err := security.Validate(); err != nil {
				t.Fatalf("build S3 secure binding: %v", err)
			}
			binding := capability.Binding{
				ObjectStorageS3: &capability.ObjectStorageS3Binding{Bucket: "default"},
				Security:        &security,
			}
			if err := driver.Preflight(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			if err := driver.Provision(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			if err := driver.Bind(ctx, resource, binding); err != nil {
				t.Fatal(err)
			}
			waitSeaweedFSHAReady(t, ctx, driver, resource, binding, dataDir, namespace)

			before, err := application.LoadObjectStorageCredentials(appFiles, resource.Name)
			if err != nil {
				t.Fatal(err)
			}
			if err := driver.RotateBucketCredentials(ctx, resource.Name); err != nil {
				t.Fatalf("rotate SeaweedFS bucket credentials: %v", err)
			}
			after, err := application.LoadObjectStorageCredentials(appFiles, resource.Name)
			if err != nil {
				t.Fatal(err)
			}
			if before == after || after.AccessKeyID == "" || after.SecretAccessKey == "" {
				t.Fatal("SeaweedFS credential rotation did not replace the application credential")
			}
			waitSeaweedFSHAReady(t, ctx, driver, resource, binding, dataDir, namespace)

			inventory, err := runtime.ListRuntimeContainers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			actual := 0
			for _, container := range inventory {
				if container.Project != files.Project || !container.Running {
					continue
				}
				n, err := strconv.Atoi(strings.TrimPrefix(container.Service, "seaweedfs-node-"))
				if strings.HasPrefix(container.Service, "seaweedfs-node-") && err == nil && n > 0 {
					actual++
				}
			}
			if actual != scenario.members {
				t.Fatalf("native running data/receiver members=%d want=%d", actual, scenario.members)
			}
			composeBefore, err := os.ReadFile(files.Compose)
			if err != nil {
				t.Fatal(err)
			}
			if err := driver.Provision(ctx, resource, binding); err != nil {
				t.Fatalf("repeated up: %v", err)
			}
			composeAfter, err := os.ReadFile(files.Compose)
			if err != nil || string(composeBefore) != string(composeAfter) {
				t.Fatalf("repeated up changed topology: %v", err)
			}
			waitSeaweedFSHAReady(t, ctx, driver, resource, binding, dataDir, namespace)

			// Changing intent on an existing datastore must fail before native
			// containers, credentials, buckets or retained Compose are rewritten.
			inventory, err = runtime.ListRuntimeContainers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			changed := app
			changed.Availability = map[string]availability.Override{"object_storage": {HA: &yes}}
			if scenario.members > 1 {
				changed.Availability["object_storage"] = availability.Override{HA: &no}
			}
			changedDriver := NewDriverAt(runtime, changed, appFiles, issuer, dataDir, namespace)
			if _, _, _, err := changedDriver.EnsureSharedProvider(ctx); err == nil {
				t.Fatal("existing datastore accepted an implicit topology migration")
			}
			retained, err := os.ReadFile(files.Compose)
			if err != nil || string(retained) != string(composeBefore) {
				t.Fatal("rejected topology migration modified retained Compose")
			}
			inventoryAfter, err := runtime.ListRuntimeContainers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, before := range inventory {
				if before.Project != files.Project || !before.Running {
					continue
				}
				found := false
				for _, after := range inventoryAfter {
					if after.ID == before.ID && after.Project == before.Project && after.Service == before.Service && after.Running {
						found = true
					}
				}
				if !found {
					t.Fatalf("rejected migration replaced or stopped owned service %s", before.Service)
				}
			}
			waitSeaweedFSHAReady(t, ctx, driver, resource, binding, dataDir, namespace)
			t.Logf("native topology qualified: data members=%d, authenticated S3/readiness/rotation/repeated up/destroy; existing topology transition rejected", actual)

		})
	}
}
