package traces_test

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
	"github.com/mcpdev80/baseharbor/internal/traces"
)

// This native test exercises real Caddy TLS provisioning without a container
// engine. The HA acceptance separately covers Tempo members and runtimes.
func TestTempoPKIRotationReloadsNativeGatewayTrust(t *testing.T) {
	binary := os.Getenv("BASEHARBOR_TEST_CADDY_BINARY")
	if binary == "" {
		t.Skip("native TLS regression requires BASEHARBOR_TEST_CADDY_BINARY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	issuer := serviceissuer.New(t)
	app := application.Manifest{Name: "rotation", Environment: "test"}
	runtime := &nativeTempoGatewayRuntime{binary: binary, root: root}
	driver := traces.NewDriverAt(runtime, app, issuer, root, "rotation")
	files, _, err := traces.EnsureProviderFilesAt(ctx, issuer, root, "rotation", app)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := serviceaccess.Resolve("test", "tempo", serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	oldMaterial, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		t.Fatal(err)
	}
	oldCA := mustReadTempoPKIFile(t, oldMaterial.CA)
	oldCert := mustReadTempoPKIFile(t, oldMaterial.ClientCertificate)
	oldKey := mustReadTempoPKIFile(t, oldMaterial.ClientKey)
	if err := runtime.UpProject(ctx, "native-tempo", files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if runtime.process != nil {
			_ = runtime.process.Process.Kill()
			_ = runtime.process.Wait()
		}
	})
	client, err := serviceaccess.NewHTTPClientForPolicy(oldMaterial, policy)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := traces.ProviderEndpoint(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceaccess.WaitHTTPS(ctx, client, endpoint, "/ready"); err != nil {
		t.Fatal(err)
	}
	issuer.Rotate(t)
	if err := driver.RotateAccessPKI(ctx); err != nil {
		t.Fatal(err)
	}
	if runtime.reloads != 2 {
		t.Fatalf("reloads = %d, want overlap and retirement", runtime.reloads)
	}
	current, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		t.Fatal(err)
	}
	assertTempoRetiredMaterialRejected(t, ctx, policy, endpoint, current, oldCA, nil, nil, "retired CA")
	assertTempoRetiredMaterialRejected(t, ctx, policy, endpoint, current, nil, oldCert, oldKey, "retired client")
}

type nativeTempoGatewayRuntime struct {
	binary, root, config string
	process              *exec.Cmd
	reloads              int
}

func (r *nativeTempoGatewayRuntime) UpProject(ctx context.Context, project, compose, env string) error {
	if r.process != nil {
		return nil
	}
	files, _, err := traces.ExistingProviderFilesAt(r.root, "rotation", application.Manifest{Name: "rotation", Environment: "test"})
	if err != nil {
		return err
	}
	endpoint, err := traces.ProviderEndpoint(files)
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

func (r *nativeTempoGatewayRuntime) ExecProject(ctx context.Context, project, compose, env, service string, args ...string) (string, error) {
	if service != "tempo-access" || strings.Join(args, " ") != "/run/baseharbor/caddy reload --force --config /etc/caddy/Caddyfile --adapter caddyfile" {
		return "", fmt.Errorf("unexpected gateway reload: %s %v", service, args)
	}
	r.reloads++
	out, err := exec.CommandContext(ctx, r.binary, "reload", "--force", "--config", r.config, "--adapter", "caddyfile", "--address", "unix/"+filepath.Join(r.root, "admin.sock")).CombinedOutput()
	return string(out), err
}

func (r *nativeTempoGatewayRuntime) ConfigProject(context.Context, string, string, string) error {
	return nil
}
func (r *nativeTempoGatewayRuntime) StopProject(context.Context, string, string, string) error {
	return nil
}
func (r *nativeTempoGatewayRuntime) DestroyProject(context.Context, string, string, string) error {
	return nil
}
