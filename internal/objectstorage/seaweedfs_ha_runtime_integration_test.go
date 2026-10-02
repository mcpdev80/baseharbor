package objectstorage

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestSeaweedFSHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_SEAWEEDFS_HA_ACCEPTANCE") != "1" {
		t.Skip("SeaweedFS HA acceptance requires BASEHARBOR_SEAWEEDFS_HA_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	namespace := "seaweedfs-ha-acceptance"
	issuer := serviceissuer.New(t)

	appStore := application.Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: namespace}
	app := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "seaweedfs-ha-ci",
		Environment:   "dev",
		HA:            true,
		Services: application.Services{
			ObjectStorage:             true,
			ObjectStorageManagementUI: true,
		},
	}
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
	binding := capability.Binding{ObjectStorageS3: &capability.ObjectStorageS3Binding{Bucket: "default"}}
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

	envData, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := parseEnv(envData)
	if err != nil {
		t.Fatal(err)
	}
	failedMember := "seaweedfs-node-1"
	if err := runtime.StopProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{failedMember}, files.Compose); err != nil {
		t.Fatalf("stop SeaweedFS HA member %s: %v", failedMember, err)
	}

	failoverDeadline := time.Now().Add(75 * time.Second)
	for {
		semanticErr := driver.Verify(ctx, resource, binding)
		uiErr := VerifyManagementUIAt(ctx, dataDir, namespace)
		if semanticErr == nil && uiErr == nil {
			break
		}
		if time.Now().After(failoverDeadline) {
			t.Fatalf("SeaweedFS stable S3/Admin endpoints did not survive member failure: s3=%v ui=%v", semanticErr, uiErr)
		}
		time.Sleep(time.Second)
	}

	if err := runtime.UpProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{failedMember}, files.Compose); err != nil {
		t.Fatalf("restart SeaweedFS HA member %s: %v", failedMember, err)
	}
	waitSeaweedFSHAReady(t, ctx, driver, resource, binding, dataDir, namespace)

	// Prove the recovered member did not alter stable endpoint semantics.
	instance, err := driver.realization.Existing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	status, _, err := signedS3Request(ctx, instance.HTTPClient, instance.Endpoint, http.MethodHead, PhysicalBucketName(app, resource.Name), "", after, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("SeaweedFS bucket is not reachable after member recovery: HTTP %d", status)
	}
}

func waitSeaweedFSHAReady(t *testing.T, ctx context.Context, driver *Driver, resource capability.Resource, binding capability.Binding, dataDir, namespace string) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for {
		s3Err := driver.Verify(ctx, resource, binding)
		uiErr := VerifyManagementUIAt(ctx, dataDir, namespace)
		if s3Err == nil && uiErr == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("SeaweedFS HA did not become ready: s3=%v ui=%v", s3Err, uiErr)
		}
		time.Sleep(time.Second)
	}
}
