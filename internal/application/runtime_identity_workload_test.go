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

func TestRuntimeIdentityWorkloadOverrideRequiresSecureAPIURLForConsumer(t *testing.T) {
	m := New("demo", "dev", true, false, true)
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	workload := WorkloadFiles{Services: []string{"api"}}
	plan := WorkloadBindingPlan{RuntimeIdentityServices: []string{"api"}}

	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "")
	if _, _, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files, plan); !errors.Is(err, ErrRuntimeAPIURL) {
		t.Fatalf("missing runtime API URL error = %v", err)
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
		"BASEHARBOR_RUNTIME_TOKEN_FILE: \"/run/baseharbor/runtime/token\"",
		":/run/baseharbor/runtime/token:ro",
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
	overrideInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if overrideInfo.Mode().Perm() != 0o600 {
		t.Fatalf("override mode = %o, want 600", overrideInfo.Mode().Perm())
	}
}
