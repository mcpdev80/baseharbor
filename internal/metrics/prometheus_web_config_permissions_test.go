package metrics

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestWritePrometheusWebConfigOverridesRestrictiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	dir := t.TempDir()
	path := filepath.Join(dir, "web-config.yml")
	if err := writePrometheusWebConfig(path, serviceaccess.Policy{}, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("web config mode = %04o, want 0644 for the numeric Prometheus user", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("provider directory mode = %04o, want 0700 to keep host-side config private", got)
	}
}

func TestPrometheusConsumerProjectionsOverrideRestrictiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	root := t.TempDir()
	targets := filepath.Join(root, "targets")
	source := observability.MetricsSource{
		ID: "runtime-broker:demo:dev", Target: "baseharbor-runtime:8443", Path: "/metrics",
		Security: observability.Security{
			TLSRequired:       true,
			TrustFile:         writeMetricsPermissionFixture(t, root, "source-ca.pem"),
			ClientCertificate: writeMetricsPermissionFixture(t, root, "source-client.pem"),
			ClientKey:         writeMetricsPermissionFixture(t, root, "source-client-key.pem"),
		},
	}
	if err := syncProviderTargets(targets, []observability.MetricsSource{source}); err != nil {
		t.Fatal(err)
	}
	security := filepath.Join(root, "provider-security")
	if err := syncProviderSecurity(security, []observability.MetricsSource{source}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targets, providerTargetFileName(source))
	token := providerSourceToken(source.ID)
	for path, want := range map[string]os.FileMode{
		targets: 0o755, target: 0o644,
		security:                                         0o755,
		filepath.Join(security, token+"-ca.pem"):         0o644,
		filepath.Join(security, token+"-client.pem"):     0o644,
		filepath.Join(security, token+"-client-key.pem"): 0o644,
		root: 0o700,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %04o, want %04o", path, got, want)
		}
	}
}

func writeMetricsPermissionFixture(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
