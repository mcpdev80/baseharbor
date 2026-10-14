package telemetry

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func registeredOTLPFixture(t *testing.T, endpoint string, ca, cert, key []byte) (application.Manifest, application.RuntimeFiles, string, string) {
	t.Helper()
	m, f := externalAuthFixture(t)
	f.Bindings = filepath.Join(f.Dir, "bindings#owned?100%")
	m.ApplicationID = "80ce2925-2ec1-4c43-893c-bb881ee6cbb9"
	root := t.TempDir()
	t.Setenv(application.OTLPEndpointEnv, "")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "receiver-alias")
	t.Setenv(ExternalHeadersEnv, "")
	if err := os.Unsetenv(ExternalHeadersEnv); err != nil {
		t.Fatal(err)
	}
	credential := filepath.Join(root, "credential.json")
	if err := os.WriteFile(credential, []byte(`{"token":"original-receiver-token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	reg := externalprovider.Registration{ID: "receiver-alias", ProviderID: "company/receiver", Provider: capability.ExternalOTLP, Endpoint: endpoint, CredentialRef: "file://" + credential, Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem}}
	write := func(name string, data []byte, mode os.FileMode) string {
		if len(data) == 0 {
			return ""
		}
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if len(ca) > 0 {
		reg.Trust.Mode = externalprovider.TrustCustomCA
		reg.Trust.CAReference = write("source-ca.pem", ca, 0644)
	}
	if len(cert) > 0 {
		reg.Trust.Mode = externalprovider.TrustMTLS
		reg.Trust.ClientCertificate = write("source-client.pem", cert, 0644)
		reg.Trust.ClientKey = write("source-key.pem", key, 0600)
	}
	if err := application.RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	return m, f, root, credential
}

func TestRegisteredOTLPActualConsumerTokenTrustAndClientIdentity(t *testing.T) {
	issuer := serviceissuer.New(t)
	serverID, err := issuer.Issue(t.Context(), serviceaccess.CertificateRequest{CommonName: "receiver", IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}})
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := issuer.Issue(t.Context(), serviceaccess.CertificateRequest{CommonName: "original-consumer"})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(serverID.Certificate, serverID.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(serverID.IssuingCA)
	requests := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/traces" || r.Header.Get("Authorization") != "Bearer original-receiver-token" || len(r.TLS.PeerCertificates) != 1 || r.TLS.PeerCertificates[0].Subject.CommonName != "original-consumer" || !bytes.Contains(body, []byte("baseharbor.otlp.verify")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	m, f, root, credential := registeredOTLPFixture(t, server.URL, serverID.IssuingCA, clientID.Certificate, clientID.PrivateKey)
	originals := map[string][]byte{}
	for _, p := range []string{credential, filepath.Join(root, "source-key.pem"), filepath.Join(root, "external-providers.json")} {
		originals[p], _ = os.ReadFile(p)
	}
	d := NewDriverAt(nil, m, f, nil, root, "test")
	resource := capability.Resource{Kind: capability.TelemetryOTLP, Provider: capability.ProviderExternalOTLP}
	binding := capability.Binding{TelemetryOTLP: &capability.OTLPTelemetryBinding{Direction: "export", Protocol: "http/protobuf", Signals: []string{"traces"}}}
	if err := d.Preflight(t.Context(), resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := d.Verify(t.Context(), resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := VerifyApplicationAt(t.Context(), m, f, root, "test"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatal("actual consumer traces were not received")
	}
	env, err := application.ManagedWorkloadEnvironment(m, f)
	if err != nil || env["OTEL_EXPORTER_OTLP_CLIENT_KEY"] != application.OTLPTLSContainerClientKey {
		t.Fatal("workload has no registered client identity")
	}
	for p, before := range originals {
		after, _ := os.ReadFile(p)
		if !bytes.Equal(before, after) {
			t.Fatal("foreign registration or source material changed")
		}
	}
	for _, p := range []string{f.Env, f.ApplicationEnv} {
		info, _ := os.Stat(p)
		if info.Mode().Perm()&0077 != 0 {
			t.Fatal("private authorization became public")
		}
	}
}

func TestRegisteredOTLPDriftBeforeAndDuringDeliveryNeverVerifies(t *testing.T) {
	for _, phase := range []string{"before-bind", "before-verify", "during-verify", "host-projection", "public-projection-owner", "foreign-projection-path"} {
		t.Run(phase, func(t *testing.T) {
			requests := 0
			var credential string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if phase == "during-verify" {
					if err := os.WriteFile(credential, []byte(`{"token":"changed-private-token"}`), 0600); err != nil {
						t.Error(err)
					}
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
			m, f, root, path := registeredOTLPFixture(t, server.URL, ca, nil, nil)
			credential = path
			d := NewDriverAt(nil, m, f, nil, root, "")
			r := capability.Resource{Provider: capability.ProviderExternalOTLP}
			if phase == "before-bind" {
				before, _ := os.ReadFile(f.Env)
				os.WriteFile(credential, []byte(`{"token":"changed-private-token"}`), 0600)
				if err := d.Bind(t.Context(), r, capability.Binding{}); err == nil || strings.Contains(err.Error(), "changed-private-token") {
					t.Fatal("changed source admitted or disclosed")
				}
				after, _ := os.ReadFile(f.Env)
				if !bytes.Equal(before, after) {
					t.Fatal("denied preparation changed delivery")
				}
				return
			}
			if err := d.Bind(t.Context(), r, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			switch phase {
			case "before-verify":
				os.WriteFile(credential, []byte(`{"token":"changed-private-token"}`), 0600)
			case "host-projection":
				data, _ := os.ReadFile(f.ApplicationEnv)
				os.WriteFile(f.ApplicationEnv, bytes.ReplaceAll(data, []byte("original-receiver-token"), []byte("changed-private-token")), 0600)
			case "public-projection-owner":
				os.Chmod(filepath.Join(f.Bindings, "telemetry"), 0755)
			case "foreign-projection-path":
				other := filepath.Join(root, "unrelated-ca.pem")
				os.WriteFile(other, ca, 0644)
				data, _ := os.ReadFile(f.Env)
				os.WriteFile(f.Env, bytes.ReplaceAll(data, []byte(filepath.Join(f.Bindings, "telemetry", "ca.pem")), []byte(other)), 0600)
			}
			if err := d.Verify(t.Context(), r, capability.Binding{}); err == nil || strings.Contains(err.Error(), "changed-private-token") {
				t.Fatal("unverified material drift passed or disclosed")
			}
			expected := 0
			if phase == "during-verify" {
				expected = 1
			}
			if requests != expected {
				t.Fatal("unverified delivery reached receiver")
			}
		})
	}
}

func TestRegisteredOTLPRebindingNeverInheritsAnotherRegistrationAuthorization(t *testing.T) {
	m, f, root, _ := registeredOTLPFixture(t, "https://receiver.example.test", nil, nil, nil)
	r := capability.Resource{Provider: capability.ProviderExternalOTLP}
	if err := NewDriverAt(nil, m, f, nil, root, "").Bind(t.Context(), r, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	// A second anonymous registration may share the network endpoint while
	// representing another authority. Its alias is still an explicit selection.
	if err := application.RegisterExternalProviderAt(root, externalprovider.Registration{ID: "anonymous-alias", ProviderID: "company/anonymous", Provider: capability.ExternalOTLP, Endpoint: "https://receiver.example.test", Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "anonymous-alias")
	d := NewDriverAt(nil, m, f, nil, root, "")
	if err := d.Bind(t.Context(), r, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	bound, _, err := application.ExistingRuntimeOTLPBinding(m, f)
	if err != nil || bound.Headers != "" {
		t.Fatal("new registration inherited old credential at same endpoint")
	}
	// Reapply of the same applied registration can reuse its private explicit
	// delivery, and any subsequent change to that owner must fail before bind.
	if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ExternalHeadersEnv, "Authorization=Bearer own-retained-token")
	if err := NewDriverAt(nil, m, f, nil, root, "").Bind(t.Context(), r, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv(ExternalHeadersEnv)
	d = NewDriverAt(nil, m, f, nil, root, "")
	if err := d.Bind(t.Context(), r, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	d = NewDriverAt(nil, m, f, nil, root, "")
	if err := application.MaterializeExternalOTLPBinding(m, f, "https://receiver.example.test", "Authorization=Bearer changed-token"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(f.Env)
	if err := d.Bind(t.Context(), r, capability.Binding{}); err == nil {
		t.Fatal("prepared retained credential drift admitted")
	}
	after, _ := os.ReadFile(f.Env)
	if !bytes.Equal(before, after) {
		t.Fatal("denied reapply overwrote actual changed owner")
	}
}

func TestRegisteredOTLPPrivateOrInvalidPublicTrustNeverProjects(t *testing.T) {
	for _, invalid := range []string{"private-key", "arbitrary-text"} {
		t.Run(invalid, func(t *testing.T) {
			issuer := serviceissuer.New(t)
			cert, err := issuer.Issue(t.Context(), serviceaccess.CertificateRequest{CommonName: "private-client"})
			if err != nil {
				t.Fatal(err)
			}
			ca := []byte("invalid-public-material")
			if invalid == "private-key" {
				ca = append(cert.IssuingCA, cert.PrivateKey...)
			}
			m, f, root, _ := registeredOTLPFixture(t, "https://receiver.example.test", ca, nil, nil)
			before, _ := os.ReadFile(f.Env)
			d := NewDriverAt(nil, m, f, nil, root, "")
			if err := d.Bind(t.Context(), capability.Resource{Provider: capability.ProviderExternalOTLP}, capability.Binding{}); err == nil {
				t.Fatal("private or invalid trust was admitted")
			}
			after, _ := os.ReadFile(f.Env)
			if !bytes.Equal(before, after) {
				t.Fatal("invalid public trust changed original delivery")
			}
			if _, err := os.Stat(filepath.Join(f.Bindings, "telemetry")); !os.IsNotExist(err) {
				t.Fatal("invalid public trust was projected")
			}
		})
	}
}
