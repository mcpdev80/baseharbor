package exposure

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// structCompose is an explicit zero-value Docker provider; ensureFiles does not
// mutate or invoke it, so the exposure-only branch-decision witness can run the
// file-producing step without a real runtime.

// TestExposureOnlyWorkloadGetsAServingGateway is the branch-decision witness and
// the finding-#1 regression: an application that declares ONLY an HTTP exposure
// and NO data provider still materializes a Caddy serving route under
// providers/caddy/routes/<name>/ through the single existing owner. This proves
// the bounded PKI->publication->node-serving scope of card t-20261018-r27 is
// owned by internal/exposure/caddy.go and no runtime-compose duplicate gateway
// is required (finding #1: exposure-only without gateway).
func TestExposureOnlyWorkloadGetsAServingGateway(t *testing.T) {
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	m := application.New("owned", "dev", false, false, false) // sql=false, cache=false -> exposure-only
	m.Workload.Components = []string{"web"}
	m = application.WithHTTPExposure(m, "public", "web", 8080, "https")
	runtime := application.RuntimeFilesFor(application.Store{Root: t.TempDir()}, m)
	deployment := Deployment{Hostname: "example.test", TLSMode: "existing", TLSDir: t.TempDir()}
	if err := writeServingTLS(t, deployment.TLSDir, "SERVED-CERT-A"); err != nil {
		t.Fatal(err)
	}

	driver := NewDriver(structCompose{}, m, runtime, deployment)
	binding := capability.Binding{Workload: "service/web"}
	binding.HTTPExposure = &capability.HTTPExposureBinding{Service: "web", TargetPort: 8080, Protocol: "https", Visibility: "public"}
	resource := capability.Resource{Kind: capability.ExposureHTTP, Name: "public"}
	if err := driver.Preflight(context.Background(), resource, binding); err != nil {
		t.Fatalf("preflight exposure-only managed exposure: %v", err)
	}

	state, _, err := driver.ensureFiles()
	if err != nil {
		t.Fatalf("ensureFiles exposure-only managed exposure: %v", err)
	}
	if len(state.Routes) == 0 {
		t.Fatal("finding #1 not fixed: exposure-only workload produced no serving route (still services: {} / no gateway)")
	}
	routeDir := filepath.Join(runtime.Dir, "providers", "caddy", "routes", "public")
	if _, err := os.Stat(routeDir); err != nil {
		t.Fatalf("exposure-only serving route dir not materialized under single owner: %v", err)
	}
}

// TestServingCertificateRotatesWithoutReBootstrap is the finding-#3 regression:
// node-side rotation + runtime consumption of the serving certificate. It
// provisions the existing owner with serving cert A, records the served leaf,
// rotates the deployment TLS material to cert B, reconciles the workload
// transport (the rotation path), and proves the node gateway now serves the
// replacement leaf — without a re-bootstrap (no new Provision/ProvisionAndBind).
func TestServingCertificateRotatesWithoutReBootstrap(t *testing.T) {
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	m := application.New("owned", "dev", false, false, false)
	m.Workload.Components = []string{"web"}
	m = application.WithHTTPExposure(m, "public", "web", 8080, "https")
	runtime := application.RuntimeFilesFor(application.Store{Root: t.TempDir()}, m)
	deployment := Deployment{Hostname: "example.test", TLSMode: "existing", TLSDir: t.TempDir()}
	if err := writeServingTLS(t, deployment.TLSDir, "SERVED-CERT-A"); err != nil {
		t.Fatal(err)
	}

	driver := NewDriver(structCompose{}, m, runtime, deployment)
	binding := capability.Binding{Workload: "service/web"}
	binding.HTTPExposure = &capability.HTTPExposureBinding{Service: "web", TargetPort: 8080, Protocol: "https", Visibility: "public"}
	resource := capability.Resource{Kind: capability.ExposureHTTP, Name: "public"}
	if err := driver.Preflight(context.Background(), resource, binding); err != nil {
		t.Fatalf("preflight exposure-only managed exposure: %v", err)
	}

	routeDir := filepath.Join(runtime.Dir, "providers", "caddy", "routes", "public")
	// Materialize the first (pre-rotation) serving material through the single
	// owner. ensureFiles is the file-producing step of both Provision and
	// ReconcileWorkloadTransport; the runtime orchestration layer only restarts
	// containers, so the served-cert rotation is witnessed at the file level.
	stateA, _, err := driver.ensureFiles()
	if err != nil {
		t.Fatalf("ensureFiles exposure-only managed exposure: %v", err)
	}
	if len(stateA.Routes) == 0 {
		t.Fatal("finding #1 not fixed: exposure-only workload produced no serving route (no gateway)")
	}

	servedA, err := os.ReadFile(filepath.Join(routeDir, "cert.pem"))
	if err != nil {
		t.Fatalf("served serving cert not materialized for consumption: %v", err)
	}
	if string(servedA) != "SERVED-CERT-A" {
		t.Fatalf("node served unexpected cert: %q", string(servedA))
	}

	// Rotate: swap the deployment TLS material to cert B and reconcile.
	// ReconcileWorkloadTransport re-runs ensureFiles, which re-reads TLSDir and
	// rewrites the served leaf — this is the issuer-reconcile rotation path.
	if err := writeServingTLS(t, deployment.TLSDir, "SERVED-CERT-B"); err != nil {
		t.Fatal(err)
	}
	stateB, changed, err := driver.ensureFiles()
	if err != nil {
		t.Fatalf("reconcile exposure workload transport (rotation) ensureFiles: %v", err)
	}
	if !changed {
		t.Fatal("rotation reconcile did not observe changed serving material")
	}
	_ = stateB

	servedB, err := os.ReadFile(filepath.Join(routeDir, "cert.pem"))
	if err != nil {
		t.Fatalf("served serving cert missing after rotation: %v", err)
	}
	if string(servedB) == string(servedA) {
		t.Fatal("finding #3 not fixed: node gateway still serves the rotated-away leaf; rotation did not reach the serving gateway")
	}
	if string(servedB) != "SERVED-CERT-B" {
		t.Fatalf("node did not consume the rotated serving cert: got %q", string(servedB))
	}
}

func writeServingTLS(t *testing.T, dir, cert string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte(cert), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "key.pem"), []byte("KEY:"+cert), 0o600)
}
