package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// This native test exercises real Caddy TLS provisioning without a container
// engine. The HA acceptance separately covers both collectors and runtimes.
func TestOTelPKIRotationReloadsNativeGatewayTrust(t *testing.T) {
	binary := os.Getenv("BASEHARBOR_TEST_CADDY_BINARY")
	if binary == "" {
		t.Skip("native TLS regression requires BASEHARBOR_TEST_CADDY_BINARY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	issuer := serviceissuer.New(t)
	app := application.Manifest{Name: "rotation", Environment: "test"}
	runtime := &nativeGatewayRuntime{binary: binary, root: root}
	driver := NewDriverAt(runtime, app, application.RuntimeFiles{}, issuer, root, "rotation")
	files, err := driver.ensureProviderFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := serviceaccess.Resolve("test", "opentelemetry-collector", serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	policy.ServerName = "otel-collector"
	oldMaterial, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		t.Fatal(err)
	}
	oldCA := mustReadOTelPKIFile(t, oldMaterial.CA)
	oldCert := mustReadOTelPKIFile(t, oldMaterial.ClientCertificate)
	oldKey := mustReadOTelPKIFile(t, oldMaterial.ClientKey)
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if runtime.process != nil {
			_ = runtime.process.Process.Kill()
			_ = runtime.process.Wait()
		}
	})
	client, err := managedOTLPHTTPClient(app.Environment, files)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitOTLP(ctx, client, endpoint); err != nil {
		t.Fatal(err)
	}
	issuer.Rotate(t)
	if err := driver.RotatePKI(ctx); err != nil {
		t.Fatal(err)
	}
	if runtime.reloads != 2 {
		t.Fatalf("reloads = %d, want overlap and retirement", runtime.reloads)
	}
	current, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		t.Fatal(err)
	}
	assertOTelRetiredMaterialRejected(t, ctx, policy, endpoint, current, oldCA, nil, nil, "retired CA")
	assertOTelRetiredMaterialRejected(t, ctx, policy, endpoint, current, nil, oldCert, oldKey, "retired client")
}

type nativeGatewayRuntime struct {
	noopRuntime
	binary, root, config string
	process              *exec.Cmd
	reloads              int
}

func (r *nativeGatewayRuntime) UpProject(ctx context.Context, project, compose, env string) error {
	if r.process != nil {
		return nil
	}
	files, err := ExistingProviderFilesAt(r.root, "rotation")
	if err != nil {
		return err
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	// The process context closes before the temporary test directory is removed.
	go func() { <-ctx.Done(); upstream.Close() }()
	tlsDir := filepath.Join(files.Dir, "service-access", "runtime")
	r.config = filepath.Join(r.root, "Caddyfile.native")
	config := fmt.Sprintf("{\n admin unix/%s\n auto_https off\n}\n%s {\n tls %s %s {\n client_auth {\n mode require_and_verify\n trust_pool file {\n pem_file %s\n }\n }\n }\n reverse_proxy %s\n}\n", filepath.Join(r.root, "admin.sock"), strings.TrimPrefix(endpoint, "https://127.0.0.1"), filepath.Join(tlsDir, "server.pem"), filepath.Join(tlsDir, "server-key.pem"), filepath.Join(tlsDir, "ca.pem"), upstream.URL)
	if err := os.WriteFile(r.config, []byte(config), 0600); err != nil {
		return err
	}
	r.process = exec.CommandContext(ctx, r.binary, "run", "--config", r.config, "--adapter", "caddyfile")
	return r.process.Start()
}

func (r *nativeGatewayRuntime) ExecProject(ctx context.Context, project, compose, env, service string, args ...string) (string, error) {
	if service != "otel-collector-access" || strings.Join(args, " ") != "/run/baseharbor/caddy reload --force --config /etc/caddy/Caddyfile --adapter caddyfile" {
		return "", fmt.Errorf("unexpected gateway reload: %s %v", service, args)
	}
	r.reloads++
	out, err := exec.CommandContext(ctx, r.binary, "reload", "--force", "--config", r.config, "--adapter", "caddyfile", "--address", "unix/"+filepath.Join(r.root, "admin.sock")).CombinedOutput()
	return string(out), err
}
