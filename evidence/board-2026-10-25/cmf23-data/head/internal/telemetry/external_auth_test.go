package telemetry

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func externalAuthFixture(t *testing.T) (application.Manifest, application.RuntimeFiles) {
	t.Helper()
	dir := t.TempDir()
	f := application.RuntimeFiles{Dir: dir, Env: filepath.Join(dir, "runtime.env"), ApplicationEnv: filepath.Join(dir, "application.env"), Bindings: filepath.Join(dir, "bindings")}
	for _, path := range []string{f.Env, f.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := application.WithOTLPTelemetry(application.Manifest{Version: 1, Name: "demo", Environment: "test", Workload: application.WorkloadConfig{Components: []string{"api"}}}, "traces")
	return m, f
}

func TestExternalOTLPConsumerAndVerifierUseDeliveredAuthorization(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" || r.Header.Get("Authorization") != "Bearer fixture-original" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	m, files := externalAuthFixture(t)
	t.Setenv(application.OTLPEndpointEnv, server.URL)
	t.Setenv(ExternalHeadersEnv, "Authorization=Bearer fixture-original")
	d := NewDriver(nil, m, files, nil)
	d.client = server.Client()
	r := capability.Resource{Kind: capability.TelemetryOTLP, Provider: capability.ProviderExternalOTLP}
	b := capability.Binding{TelemetryOTLP: &capability.OTLPTelemetryBinding{Direction: "export", Protocol: "http/protobuf", Signals: []string{"traces"}}}
	if err := d.Preflight(t.Context(), r, b); err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), r, b); err != nil {
		t.Fatal(err)
	}
	// Ambient credentials no longer override either consumer or verifier.
	t.Setenv(ExternalHeadersEnv, "Authorization=Bearer fixture-other-authority")
	if err := d.Verify(t.Context(), r, b); err != nil {
		t.Fatal("prepared verification did not use delivered authorization")
	}
	consumer, ok, err := application.ExistingRuntimeOTLPBinding(m, files)
	if err != nil || !ok {
		t.Fatal("delivered consumer binding unavailable")
	}
	headers, err := parseExternalHeaders(consumer.Headers)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, consumer.ContainerEndpoint+"/v1/traces", bytes.NewReader(probeTracePayload(m)))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || requests != 2 {
		t.Fatal("delivered consumer did not authenticate and deliver a real trace")
	}
	env, err := application.ManagedWorkloadEnvironment(m, files)
	if err != nil || env["OTEL_EXPORTER_OTLP_HEADERS"] != consumer.Headers {
		t.Fatal("workload authorization diverged from the verified consumer")
	}
	for _, path := range []string{files.Env, files.ApplicationEnv} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("authorization projection was not owner-only")
		}
	}
}

func TestExternalOTLPChangedOrMalformedAuthorizationPreservesDelivery(t *testing.T) {
	for _, change := range []string{"changed", "malformed", "duplicate", "newline"} {
		t.Run(change, func(t *testing.T) {
			m, files := externalAuthFixture(t)
			t.Setenv(application.OTLPEndpointEnv, "https://otel.example.test")
			t.Setenv(ExternalHeadersEnv, "Authorization=Bearer fixture-retained")
			d := NewDriver(nil, m, files, nil)
			r := capability.Resource{Kind: capability.TelemetryOTLP, Provider: capability.ProviderExternalOTLP}
			if err := d.Bind(t.Context(), r, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(files.Env)
			appBefore, _ := os.ReadFile(files.ApplicationEnv)
			switch change {
			case "changed":
				t.Setenv(ExternalHeadersEnv, "Authorization=Bearer changed")
			case "malformed":
				t.Setenv(ExternalHeadersEnv, "credential-value-without-key")
				d = NewDriver(nil, m, files, nil)
			case "duplicate":
				t.Setenv(ExternalHeadersEnv, "Authorization=one,authorization=two")
				d = NewDriver(nil, m, files, nil)
			case "newline":
				t.Setenv(ExternalHeadersEnv, "Authorization=hidden-secret\nInjected=value")
				d = NewDriver(nil, m, files, nil)
			}
			err := d.Bind(t.Context(), r, capability.Binding{})
			if err == nil || strings.Contains(err.Error(), "hidden-secret") {
				t.Fatal("invalid authorization admitted or disclosed")
			}
			after, _ := os.ReadFile(files.Env)
			appAfter, _ := os.ReadFile(files.ApplicationEnv)
			if !bytes.Equal(before, after) || !bytes.Equal(appBefore, appAfter) {
				t.Fatal("denied handoff changed original delivery")
			}
		})
	}
}

func TestExternalOTLPReapplyRetainsAuthorizationOnlyAtSameAuthority(t *testing.T) {
	m, files := externalAuthFixture(t)
	t.Setenv(application.OTLPEndpointEnv, "https://original.example.test")
	t.Setenv(ExternalHeadersEnv, "Authorization=Bearer original")
	r := capability.Resource{Kind: capability.TelemetryOTLP, Provider: capability.ProviderExternalOTLP}
	d := NewDriver(nil, m, files, nil)
	if err := d.Bind(t.Context(), r, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv(ExternalHeadersEnv); err != nil {
		t.Fatal(err)
	}
	d = NewDriver(nil, m, files, nil)
	if err := d.Bind(t.Context(), r, capability.Binding{}); err != nil {
		t.Fatal("same-authority reapply lost retained authorization")
	}
	bound, _, err := application.ExistingRuntimeOTLPBinding(m, files)
	if err != nil || bound.Headers != "Authorization=Bearer original" {
		t.Fatal("same-authority authorization was not retained")
	}
	before, _ := os.ReadFile(files.Env)
	t.Setenv(application.OTLPEndpointEnv, "https://destination.example.test")
	d = NewDriver(nil, m, files, nil)
	if err := d.Bind(t.Context(), r, capability.Binding{}); err == nil {
		t.Fatal("old credential was silently delivered to a new authority")
	}
	after, _ := os.ReadFile(files.Env)
	if !bytes.Equal(before, after) {
		t.Fatal("denied authority change rewrote retained authorization")
	}
	// Explicitly selecting anonymous access is allowed; missing alternatives
	// are not a permanent prohibition against an intentional replacement.
	t.Setenv(ExternalHeadersEnv, "")
	d = NewDriver(nil, m, files, nil)
	if err := d.Bind(t.Context(), r, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	bound, _, err = application.ExistingRuntimeOTLPBinding(m, files)
	if err != nil || bound.Headers != "" || bound.ContainerEndpoint != "https://destination.example.test" {
		t.Fatal("explicit anonymous destination inherited old authorization")
	}
}

func TestExternalOTLPDoesNotRedirectAuthorization(t *testing.T) {
	forwarded := false
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/v1/traces", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	m, files := externalAuthFixture(t)
	t.Setenv(application.OTLPEndpointEnv, origin.URL)
	t.Setenv(ExternalHeadersEnv, "X-Private-Access=fixture-secret")
	d := NewDriver(nil, m, files, nil)
	d.client = origin.Client()
	if err := d.Verify(t.Context(), capability.Resource{Provider: capability.ProviderExternalOTLP}, capability.Binding{}); err == nil || forwarded {
		t.Fatal("receiver redirect forwarded confidential authorization or was reported as successful delivery")
	}
}

func TestExternalOTLPProtectedOwnerRequiredForRetainedAuthorization(t *testing.T) {
	for _, change := range []string{"public", "symlink", "hardlink", "missing"} {
		t.Run(change, func(t *testing.T) {
			m, files := externalAuthFixture(t)
			t.Setenv(application.OTLPEndpointEnv, "https://otel.example.test")
			t.Setenv(ExternalHeadersEnv, "Authorization=Bearer retained")
			if err := NewDriver(nil, m, files, nil).Bind(t.Context(), capability.Resource{Provider: capability.ProviderExternalOTLP}, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "public":
				if err := os.Chmod(files.Env, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(files.Env, files.Env+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(files.Env+".original", files.Env); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(files.Env, files.Env+".linked"); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(files.Env); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := application.ExistingRuntimeOTLPBinding(m, files); change != "missing" && err == nil {
				t.Fatal("unprotected retained authorization was delivered")
			}
			if _, err := application.ManagedWorkloadEnvironment(m, files); err == nil {
				t.Fatal("unverifiable retained authorization entered workload environment")
			}
			if err := VerifyApplication(t.Context(), m, files); err == nil {
				t.Fatal("unverifiable protected owner was reported as verified")
			}
			if err := os.Unsetenv(ExternalHeadersEnv); err != nil {
				t.Fatal(err)
			}
			if err := NewDriver(nil, m, files, nil).Bind(t.Context(), capability.Resource{Provider: capability.ProviderExternalOTLP}, capability.Binding{}); err == nil {
				t.Fatal("damaged retained delivery was silently replaced")
			}
		})
	}
}
