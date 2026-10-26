package objectstorage

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestSeaweedRetirementRetainsOriginalAuthenticatedObject(t *testing.T) {
	if os.Getenv("BASEHARBOR_STANDALONE_RETIREMENT_ACCEPTANCE") != "1" {
		t.Skip("isolated standalone retirement acceptance is not enabled")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	provider, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
	if !ok {
		t.Fatal("selected native retirement adapter unavailable")
	}
	root, namespace := t.TempDir(), "retained-object"
	issuer := serviceissuer.New(t)
	m := application.WithObjectStorageBuckets(application.New("retained-object", "prod", false, false, false), "assets")
	appFiles, err := application.EnsureRuntime(ctx, issuer, application.Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: namespace}, m)
	if err != nil {
		t.Fatal(err)
	}
	driver := NewDriverAt(provider, m, appFiles, issuer, root, namespace)
	resource := capability.Resource{Application: m.Name, Kind: capability.ObjectStorageS3, Name: "assets", Provider: capability.ProviderSeaweedFS}
	if err := driver.Provision(ctx, resource, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := driver.Bind(ctx, resource, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	files, err := ExistingProviderFilesAt(root, namespace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		if err := provider.DestroyProject(cleanup, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("isolated cleanup: %v", err)
		}
	}()
	credentials, err := application.LoadObjectStorageCredentials(appFiles, "assets")
	if err != nil {
		t.Fatal(err)
	}
	instance, err := driver.realization.Existing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bucket, key, original := PhysicalBucketName(m, "assets"), "retained-original", []byte("original-object-before-runtime-retirement")
	status, _, err := signedS3Request(ctx, instance.HTTPClient, instance.Endpoint, http.MethodPut, bucket, key, credentials, original)
	if err != nil || (status != http.StatusOK && status != http.StatusNoContent) {
		t.Fatalf("seed original object: status=%d err=%v", status, err)
	}
	before, _ := os.ReadFile(files.Env)
	retirement, err := RetireRuntimeAt(ctx, native, root, namespace)
	if err != nil || len(retirement.Removed) == 0 {
		t.Fatalf("retire runtime: %v", err)
	}
	if err := provider.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	readinessCtx, readinessDone := context.WithTimeout(ctx, providerReadinessTimeout)
	client, err := s3HTTPClient(files)
	if err != nil {
		readinessDone()
		t.Fatal(err)
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		readinessDone()
		t.Fatal(err)
	}
	readyErr := waitS3(readinessCtx, client, endpoint)
	readinessDone()
	if readyErr != nil {
		t.Log(provider.DiagnosticsProject(ctx, files.Project, files.Compose, files.Env))
		t.Fatalf("retained S3 restart readiness: %v", readyErr)
	}
	instance, err = driver.realization.Existing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	status, body, err := signedS3Request(ctx, instance.HTTPClient, instance.Endpoint, http.MethodGet, bucket, key, credentials, nil)
	if err != nil || status != http.StatusOK || !bytes.Equal(body, original) {
		t.Fatalf("original authenticated object not retained: status=%d err=%v", status, err)
	}
	after, _ := os.ReadFile(files.Env)
	current, credentialErr := application.LoadObjectStorageCredentials(appFiles, "assets")
	if credentialErr != nil || current != credentials || !bytes.Equal(before, after) {
		t.Fatal("retirement/resume changed credentials or retained provider environment")
	}
	t.Log("original TLS/authenticated S3 object and credentials retained without reseeding; isolated provider runtime retirement only")
}
