package objectstorage

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
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
	assertSeaweedFSManagementUIPublished(t, ctx, runtime, files, "after initial provider ensure")
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
	assertSeaweedFSManagementUIPublished(t, ctx, runtime, files, "after bucket provision reconcile")
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

	servicePolicy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		t.Fatal(err)
	}
	servicePolicy.ServerName = "seaweedfs"
	oldMaterial, err := serviceaccess.ExistingTLSMaterial(servicePolicy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		t.Fatal(err)
	}
	oldCA, err := os.ReadFile(oldMaterial.CA)
	if err != nil {
		t.Fatal(err)
	}
	oldCert, err := os.ReadFile(oldMaterial.ServerCertificate)
	if err != nil {
		t.Fatal(err)
	}
	oldKey, err := os.ReadFile(oldMaterial.ServerKey)
	if err != nil {
		t.Fatal(err)
	}

	rotatedIssuer := serviceissuer.New(t)
	if err := RotateProviderPKIAt(ctx, runtime, rotatedIssuer, dataDir, namespace); err != nil {
		t.Fatalf("rotate SeaweedFS provider PKI: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("verify S3 after provider PKI rotation: %v", err)
	}
	if err := VerifyManagementUIAt(ctx, dataDir, namespace); err != nil {
		t.Fatalf("verify management UI after provider PKI rotation: %v", err)
	}
	newMaterial, err := serviceaccess.ExistingTLSMaterial(servicePolicy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		t.Fatal(err)
	}
	newCA, err := os.ReadFile(newMaterial.CA)
	if err != nil {
		t.Fatal(err)
	}
	newCert, err := os.ReadFile(newMaterial.ServerCertificate)
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := os.ReadFile(newMaterial.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(oldCA, newCA) || bytes.Equal(oldCert, newCert) || bytes.Equal(oldKey, newKey) {
		t.Fatal("SeaweedFS PKI rotation did not replace CA, server certificate and server key")
	}

	oldRoots := x509.NewCertPool()
	if !oldRoots.AppendCertsFromPEM(oldCA) {
		t.Fatal("old SeaweedFS CA cannot be parsed")
	}
	oldTrustClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    oldRoots,
				ServerName: "seaweedfs",
			},
		},
		Timeout: 5 * time.Second,
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := oldTrustClient.Do(request); err == nil {
		_ = response.Body.Close()
		t.Fatal("retired SeaweedFS CA still validates the rotated service certificate")
	}
}

func waitSeaweedFSHAReady(t *testing.T, ctx context.Context, driver *Driver, resource capability.Resource, binding capability.Binding, dataDir, namespace string) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for {
		s3Err := driver.Verify(ctx, resource, binding)
		probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
		uiErr := VerifyManagementUIAt(probeCtx, dataDir, namespace)
		probeCancel()
		if s3Err == nil && uiErr == nil {
			return
		}
		if time.Now().After(deadline) {
			detail := ""
			if diagnostics, ok := driver.runtime.(runtimeDiagnostics); ok {
				diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
				if files, err := ExistingProviderFilesAt(dataDir, namespace); err == nil {
					detail = diagnostics.DiagnosticsProject(diagnosticCtx, files.Project, files.Compose, files.Env)
				}
				diagnosticCancel()
			}
			t.Fatalf("SeaweedFS HA did not become ready: s3=%v ui=%v\n%s", s3Err, uiErr, detail)
		}
		time.Sleep(time.Second)
	}
}

func assertSeaweedFSManagementUIPublished(t *testing.T, ctx context.Context, runtime interface {
	ServiceStatesProjectFilesEnv(context.Context, string, string, map[string]string, ...string) ([]bhruntime.ServiceState, error)
}, files ProviderFiles, stage string) {
	t.Helper()
	envData, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatalf("%s: read provider environment: %v", stage, err)
	}
	values, err := parseEnv(envData)
	if err != nil {
		t.Fatalf("%s: parse provider environment: %v", stage, err)
	}
	port := strings.TrimSpace(values[seaweedAdminPortEnv])
	if port == "" {
		t.Fatalf("%s: %s is not materialized", stage, seaweedAdminPortEnv)
	}
	composeData, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatalf("%s: read provider compose: %v", stage, err)
	}
	if !strings.Contains(string(composeData), "127.0.0.1:${"+seaweedAdminPortEnv+"}:9443") {
		t.Fatalf("%s: provider compose does not declare management UI host publication", stage)
	}
	publishedPort, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("%s: invalid management UI port %q: %v", stage, port, err)
	}
	states, err := runtime.ServiceStatesProjectFilesEnv(ctx, files.Project, filepath.Dir(files.Compose), values, files.Compose)
	if err != nil {
		t.Fatalf("%s: inspect provider runtime: %v", stage, err)
	}
	for _, state := range states {
		if state.Service != "seaweedfs-admin-access" || !state.Ready() {
			continue
		}
		for _, binding := range state.Publishers {
			if binding.URL == "127.0.0.1" && binding.PublishedPort == publishedPort && binding.TargetPort == 9443 && strings.EqualFold(binding.Protocol, "tcp") {
				return
			}
		}
	}
	t.Fatalf("%s: management UI container is not published as 127.0.0.1:%s->9443/tcp: %+v", stage, port, states)
}
