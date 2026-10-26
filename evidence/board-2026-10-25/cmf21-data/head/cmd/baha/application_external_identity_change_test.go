package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func registeredIdentityConsumerFixture(t *testing.T, root, id, credential string) (string, *atomic.Int64) {
	t.Helper()
	count := &atomic.Int64{}
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "userinfo_endpoint": server.URL + "/userinfo", "jwks_uri": server.URL + "/jwks"})
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("client_id") != id || r.Form.Get("client_secret") != credential {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			count.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	secret := filepath.Join(t.TempDir(), "client-secret")
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(secret, []byte(credential), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0644); err != nil {
		t.Fatal(err)
	}
	reg := externalprovider.Registration{ID: id, ProviderID: "company/oidc", Endpoint: server.URL, Provider: capability.ExternalOIDC, CredentialRef: "file://" + secret, Trust: externalprovider.Trust{Mode: externalprovider.TrustCustomCA, CAReference: ca}, Metadata: map[string]string{"client-id": id}}
	if err := application.RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatal(err)
	}
	return secret, count
}

func verifyDeliveredIdentityConsumer(t *testing.T, files application.RuntimeFiles) {
	t.Helper()
	dir := filepath.Join(application.WorkloadServiceBindingProjectionDir(files), application.IdentityBindingName)
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return bytes.TrimSpace(data)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(read("ca.crt")) {
		t.Fatal("delivered consumer trust is invalid")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.PostForm(string(read("oidc.token-endpoint")), url.Values{"client_id": {string(read("oidc.client-id"))}, "client_secret": {string(read("client-secret"))}})
	if err != nil {
		t.Fatal("delivered OIDC consumer failed actual HTTPS authentication")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("delivered OIDC credential did not authenticate with its selected receiver")
	}
}

func TestRegisteredOIDCReplacementRequiresCurrentConsentAndDeliversDestinationMaterial(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.FromRepository = false
	r.Store = application.Store{Root: t.TempDir(), Namespace: r.Target.Name}
	r.Manifest.Services.Identity = true
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	t.Setenv("BASEHARBOR_EXTERNAL_OIDC_ISSUER", "")
	for _, name := range []string{identityprovider.ExternalClientIDEnv, identityprovider.ExternalClientSecretFileEnv, identityprovider.ExternalCAFileEnv} {
		t.Setenv(name, "")
	}
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	oldSecret, oldCount := registeredIdentityConsumerFixture(t, root, "original-login", "fixture-private-original")
	_, newCount := registeredIdentityConsumerFixture(t, root, "destination-login", "fixture-private-destination")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "original-login")
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	f := application.RuntimeFilesFor(r.Store, r.Manifest)
	if err := os.MkdirAll(f.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{f.Env, f.ApplicationEnv} {
		if err := os.WriteFile(p, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepareManagedIdentity(t.Context(), nil, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := provisionAndVerifyManagedIdentity(t.Context(), io.Discard, prepared, nil); err != nil {
		t.Fatal(err)
	}
	verifyDeliveredIdentityConsumer(t, f)
	before := map[string][]byte{}
	for _, p := range []string{f.Env, f.ApplicationEnv, filepath.Join(r.TargetStateRoot, "provider-registry.json")} {
		before[p], _ = os.ReadFile(p)
	}
	globalBefore, _ := os.ReadFile(filepath.Join(root, "external-providers.json"))
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "destination-login")
	plan, err := buildResolvedApplicationPlan(t.Context(), r)
	if err != nil || plan.ProviderPlan.Impact == nil {
		t.Fatalf("registered OIDC replacement lacked consequence plan: %v", err)
	}
	impact := plan.ProviderPlan.Impact
	if impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
		t.Fatal("unverified login/session continuation lacked current two-stage acknowledgement")
	}
	public, _ := json.Marshal(plan)
	if strings.Contains(string(public), "fixture-private") {
		t.Fatal("plan disclosed confidential client material")
	}
	for _, consent := range []*application.ProviderChangeConsent{nil, {PlanID: impact.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}, {PlanID: impact.PlanID, First: true, Second: true}} {
		err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), consent), r, nil, io.Discard)
		if consent != nil && consent.PlanID == impact.PlanID && consent.Second {
			if err != nil {
				t.Fatalf("supported explicit replacement was permanently refused: %v", err)
			}
		} else {
			var failure *machine.Error
			if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired && failure.Code != machine.ErrorConflict {
				t.Fatalf("missing/current consent was bypassed: %v", err)
			}
		}
		for p, data := range before {
			after, _ := os.ReadFile(p)
			if !bytes.Equal(data, after) {
				t.Fatal("planning/admission mutated original material or canonical binding")
			}
		}
	}
	prepared, err = prepareManagedIdentity(t.Context(), nil, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := provisionAndVerifyManagedIdentity(t.Context(), io.Discard, prepared, nil); err != nil {
		t.Fatal(err)
	}
	verifyDeliveredIdentityConsumer(t, f)
	if oldCount.Load() != 1 || newCount.Load() != 1 {
		t.Fatal("actual consumer authentication did not follow source and destination binding")
	}
	if data, err := os.ReadFile(oldSecret); err != nil || string(data) != "fixture-private-original" {
		t.Fatal("original external credential owner was modified or destroyed")
	}
	if data, _ := os.ReadFile(filepath.Join(root, "external-providers.json")); !bytes.Equal(globalBefore, data) {
		t.Fatal("provider replacement changed foreign registration owners")
	}
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(r.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil || len(registry.Bindings) != 1 || len(registry.Instances) != 2 {
		t.Fatal("verified destination binding did not preserve the original external offer")
	}
	for _, i := range registry.Instances {
		if i.ID == registry.Bindings[0].ProviderInstanceID && i.Reference != "destination-login" {
			t.Fatal("canonical consumer binding did not follow verified destination")
		}
	}
}

func TestManagedToRegisteredIdentityReplacementRequiresConsentAndVerifiesDeliveredConsumer(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.Store = application.Store{Root: t.TempDir(), Namespace: "identity-handoff"}
	r.FromRepository = false
	r.Manifest.Services.Identity = true
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "keycloak")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), "application")
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(r.TargetStateRoot, "provider-registry.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	_, requests := registeredIdentityConsumerFixture(t, root, "replacement-login", "replacement-private-value")
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "replacement-login")
	for _, name := range []string{identityprovider.ExternalClientIDEnv, identityprovider.ExternalClientSecretFileEnv, identityprovider.ExternalCAFileEnv} {
		t.Setenv(name, "")
	}
	plan, err := buildResolvedApplicationPlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	impact := plan.ProviderPlan.Impact
	if impact == nil || impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
		t.Fatal("authority replacement bypassed current consequence plan")
	}
	for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: impact.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}, {PlanID: impact.PlanID, First: true, Second: true}} {
		err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), ack), r, nil, io.Discard)
		valid := ack != nil && ack.PlanID == impact.PlanID && ack.Second
		if valid && err != nil || !valid && err == nil {
			t.Fatal("authority handoff did not enforce current two-stage consent", err)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("admission changed original managed authority owner")
		}
	}
	files := application.RuntimeFilesFor(r.Store, r.Manifest)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{files.Env, files.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepareManagedIdentity(t.Context(), nil, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := provisionAndVerifyManagedIdentity(t.Context(), io.Discard, prepared, nil); err != nil {
		t.Fatal(err)
	}
	verifyDeliveredIdentityConsumer(t, files)
	if requests.Load() != 1 {
		t.Fatal("destination was not authenticated with actual delivered material")
	}
	// This proves delivery/admission, not native managed retirement or user migration.
}
