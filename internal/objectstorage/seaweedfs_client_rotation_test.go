package objectstorage

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestSeaweedFSDriverRefreshesTrustAfterProviderCARotation(t *testing.T) {
	ctx := context.Background()
	policy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		t.Fatal(err)
	}
	policy.ServerName = "seaweedfs"
	var certificate atomic.Pointer[tls.Certificate]
	clientForIssuer := func() *http.Client {
		material, err := serviceaccess.EnsureTLSMaterial(ctx, serviceissuer.New(t), policy, t.TempDir(), "seaweedfs")
		if err != nil {
			t.Fatal(err)
		}
		cert, err := tls.LoadX509KeyPair(material.ServerCertificate, material.ServerKey)
		if err != nil {
			t.Fatal(err)
		}
		certificate.Store(&cert)
		client, err := serviceaccess.NewHTTPClient(material, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.CloseIdleConnections)
		return client
	}
	oldClient := clientForIssuer()
	var payload []byte
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPut:
			payload, _ = io.ReadAll(req.Body)
		case http.MethodGet:
			_, _ = w.Write(payload)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	server.TLS = &tls.Config{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		return certificate.Load(), nil
	}}
	server.StartTLS()
	defer server.Close()

	m := application.Manifest{
		Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(),
		Name: "ca-rotation", Environment: "dev",
		Services: application.Services{ObjectStorage: true},
	}
	files, err := application.EnsureRuntime(ctx, nil, application.Store{Root: t.TempDir()}, m)
	if err != nil {
		t.Fatal(err)
	}
	realization := &rotationTestRealization{instance: SeaweedFSInstance{Endpoint: server.URL, HTTPClient: oldClient}}
	driver := NewDriverWithRealization(realization, m, files)
	resource := capability.Resource{Name: "default"}
	if err := driver.Verify(ctx, resource, capability.Binding{}); err != nil {
		t.Fatalf("verify before CA rotation: %v", err)
	}

	realization.instance.HTTPClient = clientForIssuer()
	server.CloseClientConnections()
	oldClient.CloseIdleConnections()
	if response, err := oldClient.Get(server.URL); err == nil {
		response.Body.Close()
		t.Fatal("old client unexpectedly trusts the replacement CA")
	}
	if err := driver.Verify(ctx, resource, capability.Binding{}); err != nil {
		t.Fatalf("verify with current realization trust after CA rotation: %v", err)
	}
	credentials, err := application.LoadObjectStorageCredentials(files, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.waitBucketIdentityReady(ctx, PhysicalBucketName(m, "default"), credentials); err != nil {
		t.Fatalf("verify identity with current realization trust: %v", err)
	}
}
