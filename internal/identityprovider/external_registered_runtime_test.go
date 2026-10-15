package identityprovider

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRegisteredExternalOIDCNativeConsumerHandoff(t *testing.T) {
	if os.Getenv("BASEHARBOR_EXTERNAL_PROVIDER_HANDOFF_ACCEPTANCE") != "1" {
		t.Skip("set BASEHARBOR_EXTERNAL_PROVIDER_HANDOFF_ACCEPTANCE=1 for isolated native OIDC consumer handoff")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), "application")
	producerApp := application.WithIdentity(application.New("external-handoff-source", "test", false, false, false))
	producerFiles := nativeExternalApplicationFiles(t, producerApp)
	producer := NewKeycloakDriver(runtime, producerApp, producerFiles, serviceissuer.New(t), root, "external-handoff-native")
	r := capability.Resource{Kind: capability.Identity, Provider: capability.ProviderKeycloak}
	b := capability.Binding{Identity: &capability.IdentityBinding{Scopes: []string{"openid", "profile"}}}
	if err := producer.Preflight(ctx, r, b); err != nil {
		t.Fatal(err)
	}
	if err := producer.Provision(ctx, r, b); err != nil {
		t.Fatal(err)
	}
	files, err := ExistingKeycloakFilesAt(producerApp, root, "external-handoff-native")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 90*time.Second)
		defer stop()
		if err := runtime.DestroyProject(cleanup, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("owned OIDC producer cleanup failed: %v", err)
			return
		}
		t.Log("[native-external-oidc] owned producer cleanup verified")
	}()
	if err := producer.Bind(ctx, r, b); err != nil {
		t.Fatal(err)
	}
	if err := producer.Verify(ctx, r, b); err != nil {
		t.Fatal(err)
	}
	admin, err := producer.adminClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sourceRealm, destinationRealm := producer.realm, "bh-external-handoff-destination"
	if err := admin.reconcileRealm(ctx, keycloakRealm{Realm: destinationRealm, Enabled: true, Attributes: map[string]string{"frontendUrl": producer.instance.PublicBaseURL, "baseharbor.owner": "external-handoff-native"}}); err != nil {
		t.Fatal(err)
	}
	oldSecret := registerNativeExternalIdentity(t, ctx, admin, producer, root, sourceRealm, "native-source-client", files.PublicAccess.Material.CA)
	_ = registerNativeExternalIdentity(t, ctx, admin, producer, root, destinationRealm, "native-destination-client", files.PublicAccess.Material.CA)
	consumer := application.WithIdentity(application.New("external-handoff-consumer", "test", false, false, false))
	consumerFiles := nativeExternalApplicationFiles(t, consumer)
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	t.Setenv("BASEHARBOR_EXTERNAL_OIDC_ISSUER", "")
	for _, name := range []string{ExternalClientIDEnv, ExternalClientSecretFileEnv, ExternalCAFileEnv} {
		t.Setenv(name, "")
	}
	target, err := deployment.TargetStateRoot("native-consumer")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "native-source-client")
	bindNativeRegisteredIdentity(t, ctx, consumer, consumerFiles, target, producer)
	sourceToken, sourceJWKS := nativeDeliveredIdentityToken(t, ctx, consumerFiles, producer, "native-source-client")
	if err := application.ReconcileReferenceProviderRegistryAt(target, consumer); err != nil {
		t.Fatal(err)
	}
	globalBefore, _ := os.ReadFile(filepath.Join(root, "external-providers.json"))
	oldBytes, _ := os.ReadFile(oldSecret)
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "native-destination-client")
	change, err := InspectRegisteredExternalBindingChange(consumer, consumerFiles, target)
	if err != nil || !change.Changed || !change.ClientChanged || !change.CredentialChanged || !change.InputsVerified {
		t.Fatal("actual native issuer/client/material replacement was not disclosed")
	}
	bindNativeRegisteredIdentity(t, ctx, consumer, consumerFiles, target, producer)
	_, destinationJWKS := nativeDeliveredIdentityToken(t, ctx, consumerFiles, producer, "native-destination-client")
	if err := verifyRS256JWTWithJWKS(sourceToken, sourceJWKS); err != nil {
		t.Fatal("original token unexpectedly lost its original signature validity")
	}
	if err := verifyRS256JWTWithJWKS(sourceToken, destinationJWKS); err == nil {
		t.Fatal("old issuer token was falsely migrated to destination trust")
	}
	if err := application.ReconcileReferenceProviderRegistryAt(target, consumer); err != nil {
		t.Fatal(err)
	}
	if current, _ := os.ReadFile(oldSecret); !bytes.Equal(oldBytes, current) {
		t.Fatal("handoff changed original external credential owner")
	}
	if current, _ := os.ReadFile(filepath.Join(root, "external-providers.json")); !bytes.Equal(globalBefore, current) {
		t.Fatal("handoff changed foreign registration owners")
	}
	// Dropping the consumer reference never destroys either foreign realm.
	consumer.Services.Identity = false
	consumer.Identity = application.IdentityRequirements{}
	if err := application.ReconcileReferenceProviderRegistryAt(target, consumer); err != nil {
		t.Fatal(err)
	}
	for _, choice := range []struct{ realm, client string }{{sourceRealm, "native-source-client"}, {destinationRealm, "native-destination-client"}} {
		reg, err := application.InspectExternalProvider(choice.client)
		if err != nil {
			t.Fatal(err)
		}
		path, err := externalLocalMaterialFile(reg.CredentialRef)
		if err != nil {
			t.Fatal(err)
		}
		secret, err := readExternalProtectedMaterial(path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = admin.mintSigningProbeToken(ctx, choice.realm, choice.client, strings.TrimSpace(string(secret)))
		zeroExternalMaterial(secret)
		if err != nil {
			t.Fatal("foreign identity stopped after consumer reference release")
		}
	}
	t.Log("[native-external-oidc] source and destination consumer TLS, client authentication, JWT issuer/client/expiry and JWKS signature verified; old token rejected by destination; original owners and foreign token issuance retained")
}

func nativeExternalApplicationFiles(t *testing.T, app application.Manifest) application.RuntimeFiles {
	t.Helper()
	f := application.RuntimeFilesFor(application.Store{Root: t.TempDir()}, app)
	if err := os.MkdirAll(f.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{f.Env, f.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func registerNativeExternalIdentity(t *testing.T, ctx context.Context, admin *keycloakAdmin, producer *KeycloakDriver, root, realm, clientID, ca string) string {
	t.Helper()
	secret, err := randomIdentitySecret(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ensureSigningRotationProbeClient(ctx, realm, clientID, secret); err != nil {
		t.Fatal("create owned native consumer client failed")
	}
	path := filepath.Join(t.TempDir(), "client-secret")
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	reg := externalprovider.Registration{ID: clientID, ProviderID: "native/keycloak-oidc", Provider: capability.ExternalOIDC, Endpoint: strings.TrimRight(producer.instance.PublicBaseURL, "/") + "/realms/" + realm, CredentialRef: "file://" + path, Trust: externalprovider.Trust{Mode: externalprovider.TrustCustomCA, CAReference: ca}, Metadata: map[string]string{"client-id": clientID}}
	if err := application.RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatal(err)
	}
	return path
}

func bindNativeRegisteredIdentity(t *testing.T, ctx context.Context, app application.Manifest, files application.RuntimeFiles, target string, producer *KeycloakDriver) {
	t.Helper()
	d, err := NewRegisteredExternalDriver(app, files, target)
	if err != nil {
		t.Fatal(err)
	}
	// Map the fixture's logical native hostname to its owned loopback port.
	// TLS still uses the exact captured registration CA and expected server name.
	transport := producer.instance.PublicHTTPClient.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(d.material.trust) {
		t.Fatal("captured native issuer trust is invalid")
	}
	transport.TLSClientConfig.RootCAs = pool
	transport.TLSClientConfig.Certificates = nil
	d.client = &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer d.client.CloseIdleConnections()
	r := capability.Resource{Kind: capability.Identity, Provider: capability.ProviderExternalOIDC}
	b := capability.Binding{Identity: &capability.IdentityBinding{Scopes: []string{"openid"}}}
	if err := d.Preflight(ctx, r, b); err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(ctx, r, b); err != nil {
		t.Fatal(err)
	}
	if err := d.Verify(ctx, r, b); err != nil {
		t.Fatal(err)
	}
}

func nativeDeliveredIdentityToken(t *testing.T, ctx context.Context, files application.RuntimeFiles, producer *KeycloakDriver, expectedClient string) (string, keycloakJWKS) {
	t.Helper()
	dir := filepath.Join(application.WorkloadServiceBindingProjectionDir(files), application.IdentityBindingName)
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return bytes.TrimSpace(data)
	}
	transport := producer.instance.PublicHTTPClient.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(read("ca.crt")) {
		t.Fatal("delivered native consumer CA is invalid")
	}
	transport.TLSClientConfig.RootCAs, transport.TLSClientConfig.Certificates = pool, nil
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	secret := read("client-secret")
	defer zeroExternalMaterial(secret)
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {string(read("oidc.client-id"))}, "client_secret": {string(secret)}}
	response, err := client.PostForm(string(read("oidc.token-endpoint")), form)
	if err != nil {
		t.Fatal("actual native consumer TLS/token request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal("actual native delivered client authentication failed")
	}
	var reply struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&reply); err != nil || reply.AccessToken == "" {
		t.Fatal("native token response is invalid")
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, string(read("oidc.jwks-uri")), nil)
	keys, err := client.Do(request)
	if err != nil {
		t.Fatal("actual native consumer JWKS request failed")
	}
	defer keys.Body.Close()
	var jwks keycloakJWKS
	if keys.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(keys.Body, 1<<20)).Decode(&jwks) != nil || verifyRS256JWTWithJWKS(reply.AccessToken, jwks) != nil {
		t.Fatal("native consumer could not verify actual signed token")
	}
	parts := strings.Split(reply.AccessToken, ".")
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal("native token claims are invalid")
	}
	defer zeroExternalMaterial(claimsBytes)
	var claims struct {
		Issuer string `json:"iss"`
		Client string `json:"azp"`
		Expiry int64  `json:"exp"`
	}
	if json.Unmarshal(claimsBytes, &claims) != nil || claims.Issuer != string(read("oidc.issuer")) || claims.Client != expectedClient || claims.Expiry <= time.Now().Unix() {
		t.Fatal("native token does not match delivered issuer/client/current validity")
	}
	return reply.AccessToken, jwks
}
