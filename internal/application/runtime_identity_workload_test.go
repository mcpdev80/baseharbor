package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeIdentityWorkloadOverrideIsOptionalWithoutConsumers(t *testing.T) {
	m := New("demo", "dev", true, false, true)
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	workload := WorkloadFiles{Services: []string{"api"}}

	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "")
	path, enabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, WorkloadBindingPlan{})
	if err != nil {
		t.Fatalf("unused runtime API should remain optional: %v", err)
	}
	if enabled || path != "" {
		t.Fatalf("runtime identity path=%q enabled=%v, want disabled without consumers", path, enabled)
	}
	if _, err := os.Stat(RuntimeIdentityTokenPath(files)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime token should not be materialized without consumers: %v", err)
	}
}

func TestRuntimeIdentityWorkloadOverrideDefaultsSecureAPIURLForConsumer(t *testing.T) {
	m := New("demo", "dev", true, false, true)
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	workload := WorkloadFiles{Services: []string{"api"}}
	plan := WorkloadBindingPlan{RuntimeIdentityServices: []string{"api"}}
	materializeRuntimeMTLSTestFiles(t, files)

	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "")
	path, enabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, plan)
	if err != nil {
		t.Fatalf("default runtime API URL should be automatic: %v", err)
	}
	if !enabled {
		t.Fatal("runtime identity override was not enabled")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "BASEHARBOR_RUNTIME_API_URL: \"https://baseharbor-runtime:8443\"") {
		t.Fatalf("override does not use automatic broker URL:\n%s", data)
	}

	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "http://baseharbor.example")
	if _, _, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, plan); !errors.Is(err, ErrRuntimeAPIURL) {
		t.Fatalf("insecure runtime API URL error = %v", err)
	}
}

func TestRuntimeIdentityWorkloadOverrideScopesTokenAndFileSecrets(t *testing.T) {
	m := WithRequiredSecrets(New("demo", "dev", true, false, true), "TLS_KEY_FILE")
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	if err := os.MkdirAll(SecretFileHostDir(files), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SecretFileHostPath(files, "TLS_KEY_FILE"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	materializeRuntimeMTLSTestFiles(t, files)
	workload := WorkloadFiles{Services: []string{"web", "worker", "api"}}
	plan := WorkloadBindingPlan{
		RuntimeIdentityServices: []string{"api"},
		FileSecretsByService: map[string][]string{
			"api": {"TLS_KEY_FILE"},
		},
	}
	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "https://baseharbor.example/runtime/")

	path, enabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("runtime binding override was not enabled")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"api:",
		"BASEHARBOR_RUNTIME_API_URL: \"https://baseharbor.example/runtime\"",
		"BASEHARBOR_RUNTIME_TOKEN_FILE: \"/run/secrets/baseharbor-runtime-token\"",
		"BASEHARBOR_RUNTIME_CA_FILE: \"/run/secrets/baseharbor-runtime-ca\"",
		"BASEHARBOR_RUNTIME_CLIENT_CERT_FILE: \"/run/secrets/baseharbor-runtime-client-cert\"",
		"BASEHARBOR_RUNTIME_CLIENT_KEY_FILE: \"/run/secrets/baseharbor-runtime-client-key\"",
		"baseharbor-runtime-token:",
		"baseharbor-runtime-client-key:",
		":/run/baseharbor/bindings/secrets/TLS_KEY_FILE:ro",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("override missing %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"web:", "worker:"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("non-consuming service received protected binding %q:\n%s", unwanted, text)
		}
	}
	info, err := os.Stat(RuntimeIdentityTokenPath(files))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", info.Mode().Perm())
	}
	projectionDir := filepath.Join(files.Bindings, "runtime-workload")
	projectionDirInfo, err := os.Stat(projectionDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectionDirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("runtime workload projection directory mode = %o, want 700", got)
	}
	for _, name := range []string{
		"baseharbor-runtime-token",
		"baseharbor-runtime-ca",
		"baseharbor-runtime-client-cert",
		"baseharbor-runtime-client-key",
		"baseharbor-tls-cert",
		"baseharbor-tls-key",
	} {
		projectedInfo, err := os.Stat(filepath.Join(projectionDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := projectedInfo.Mode().Perm(); got != 0o644 {
			t.Fatalf("runtime workload projection %s mode = %o, want 644", name, got)
		}
	}
	overrideInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if overrideInfo.Mode().Perm() != 0o600 {
		t.Fatalf("override mode = %o, want 600", overrideInfo.Mode().Perm())
	}
}

func materializeRuntimeMTLSTestFiles(t *testing.T, files RuntimeFiles) {
	t.Helper()
	dir := RuntimeMTLSHostDir(files)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ca.pem", "client-cert.pem", "client-key.pem", "workload-cert.pem", "workload-key.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuntimeIdentityWorkloadOverrideUsesServiceScopedTokenForExplicitPermission(t *testing.T) {
	m := New("demo", "dev", false, false, true)
	m = WithRuntimePermission(m, "object-storage.s3/v1", []string{"api"}, "runtime.create")
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	materializeRuntimeMTLSTestFiles(t, files)
	workload := WorkloadFiles{Services: []string{"api"}}
	plan := WorkloadBindingPlan{RuntimeIdentityServices: []string{"api"}}

	path, enabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("runtime identity override was not enabled")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	serviceSecret := runtimeServiceIdentitySecretName("api")
	if !strings.Contains(text, "source: "+serviceSecret) {
		t.Fatalf("explicit runtime permission did not use service-scoped token %q:\n%s", serviceSecret, text)
	}
	if strings.Contains(text, "source: baseharbor-runtime-token\n") {
		t.Fatalf("explicit runtime permission unexpectedly fell back to legacy app token:\n%s", text)
	}
	if !strings.Contains(text, serviceSecret+":") {
		t.Fatalf("service-scoped token secret definition missing:\n%s", text)
	}
}
