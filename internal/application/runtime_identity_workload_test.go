package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeIdentityWorkloadOverrideIsOptionalWithoutAPIURL(t *testing.T) {
	m := New("demo", "dev", true, false, true)
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	workload := WorkloadFiles{Services: []string{"api"}}

	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "")
	path, enabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files)
	if err != nil {
		t.Fatalf("missing runtime API URL should keep runtime identity optional: %v", err)
	}
	if enabled || path != "" {
		t.Fatalf("runtime identity path=%q enabled=%v, want disabled without configured API", path, enabled)
	}
	if _, err := os.Stat(RuntimeIdentityTokenPath(files)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime token should not be materialized without configured API: %v", err)
	}
}

func TestRuntimeIdentityWorkloadOverrideRejectsInsecureAPIURL(t *testing.T) {
	m := New("demo", "dev", true, false, true)
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	workload := WorkloadFiles{Services: []string{"api"}}

	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "http://baseharbor.example")
	if _, _, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files); !errors.Is(err, ErrRuntimeAPIURL) {
		t.Fatalf("insecure runtime API URL error = %v", err)
	}
}

func TestRuntimeIdentityWorkloadOverrideMountsOwnerOnlyTokenAndIsReusable(t *testing.T) {
	m := New("demo", "dev", true, false, true)
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	files.Bindings = filepath.Join(files.Dir, "bindings")
	workload := WorkloadFiles{Services: []string{"worker", "api"}}
	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "https://baseharbor.example/runtime/")

	path, enabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files)
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
	for _, want := range []string{
		"BASEHARBOR_RUNTIME_API_URL: \"https://baseharbor.example/runtime\"",
		"BASEHARBOR_RUNTIME_TOKEN_FILE: \"/run/baseharbor/runtime/token\"",
		":/run/baseharbor/runtime/token:ro",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("override missing %q:\n%s", want, text)
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

	t.Setenv("BASEHARBOR_RUNTIME_API_URL", "")
	reused, reusedEnabled, err := MaterializeRuntimeIdentityWorkloadOverride(m, workload, files)
	if err != nil {
		t.Fatalf("reuse without operator env failed: %v", err)
	}
	if !reusedEnabled || reused != path {
		t.Fatalf("reused override = %q enabled=%v, want %q true", reused, reusedEnabled, path)
	}
}
