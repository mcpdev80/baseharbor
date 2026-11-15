package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func saveLocalOwnerTestConfig(t *testing.T, target string, definition TargetDefinition, access AccessDefinition) {
	t.Helper()
	config := Config{
		Version:       ConfigVersion,
		DefaultTarget: target,
		Access:        map[string]AccessDefinition{"local": access},
		Targets:       map[string]TargetDefinition{target: definition},
	}
	if err := config.Save(); err != nil {
		t.Fatalf("save target config: %v", err)
	}
}

func localOwnerTestDefinition(scope, runtime string, tenantID string) TargetDefinition {
	return TargetDefinition{
		TenantID: tenantID,
		Runtime:  RuntimeDefinition{Provider: runtime},
		Access:   TargetAccess{Reference: "local"},
		Scope:    scope,
	}
}

func TestExplicitLocalTargetOwnsCorelessDeploymentNamespace(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data, config := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", config)

	target := "local-workload"
	saveLocalOwnerTestConfig(t, target, localOwnerTestDefinition("local", "docker", ""), AccessDefinition{Provider: "local", Reference: "local"})
	runtimeRoot := filepath.Join(data, "baseharbor", "targets", target, "runtime")
	if err := VerifyDeploymentOwner(target, runtimeRoot, "11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatalf("explicit local-scope workload target was not authorized: %v", err)
	}

	// The authorization persists for later read/write calls after the deployment
	// namespace exists, while the Core runtime root remains absent.
	identity := testDeploymentIdentity(t, target, "local-workload-app", "dev")
	record := DeploymentRecord{Version: DeploymentRecordVersion, Identity: identity, Source: DeploymentSource{Kind: "repository"}}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("save local workload deployment: %v", err)
	}
	if _, err := LoadDeploymentRecord(identity); err != nil {
		t.Fatalf("load local workload deployment: %v", err)
	}
	if _, err := os.Lstat(runtimeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local workload owner created Core runtime state: %v", err)
	}
}

func TestLocalTargetOwnerAuthorizationIsNarrowAndFailClosed(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		access   AccessDefinition
		tenantID string
		runtime  string
		artifact string
	}{
		{name: "non-local-scope", scope: "team", access: AccessDefinition{Provider: "local", Reference: "local"}, runtime: "docker"},
		{name: "remote-access", scope: "local", access: AccessDefinition{Provider: "docker", Reference: "remote"}, runtime: "docker"},
		{name: "tenant-local-access", scope: "local", access: AccessDefinition{Provider: "local", Reference: "local"}, tenantID: "11111111-1111-4111-8111-111111111111", runtime: "docker"},
		{name: "unsupported-runtime", scope: "local", access: AccessDefinition{Provider: "local", Reference: "local"}, runtime: "kubernetes"},
		{name: "orphaned-core-runtime", scope: "local", access: AccessDefinition{Provider: "local", Reference: "local"}, runtime: "docker", artifact: "runtime.env"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", "")
			data, config := t.TempDir(), t.TempDir()
			t.Setenv("XDG_DATA_HOME", data)
			t.Setenv("XDG_CONFIG_HOME", config)
			target := "candidate"
			saveLocalOwnerTestConfig(t, target, localOwnerTestDefinition(test.scope, test.runtime, test.tenantID), test.access)
			runtimeRoot := filepath.Join(data, "baseharbor", "targets", target, "runtime")
			if test.artifact != "" {
				if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(runtimeRoot, test.artifact), []byte("orphaned"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := VerifyDeploymentOwner(target, runtimeRoot, "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerMissing) {
				t.Fatalf("ineligible local target was not refused as missing owner: %v", err)
			}
		})
	}
}

func TestLocalTargetOwnerDoesNotOverrideCoreOwnerVerdict(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data, config := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", config)
	target := "local-managed"
	saveLocalOwnerTestConfig(t, target, localOwnerTestDefinition("local", "docker", ""), AccessDefinition{Provider: "local", Reference: "local"})
	runtimeRoot := filepath.Join(data, "baseharbor", "targets", target, "runtime")
	writeInstallationAuthoritative(t, target, runtimeRoot)
	if err := VerifyDeploymentOwner(target, runtimeRoot, "11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatalf("valid Core owner did not remain authoritative: %v", err)
	}
	makeInstallationForeign(t, runtimeRoot)
	if err := VerifyDeploymentOwner(target, runtimeRoot, "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("local scope overrode a foreign Core owner: %v", err)
	}
}

func TestLocalTargetOwnerDoesNotOverrideDamagedCoreState(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data, config := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", config)
	target := "local-damaged"
	saveLocalOwnerTestConfig(t, target, localOwnerTestDefinition("local", "docker", ""), AccessDefinition{Provider: "local", Reference: "local"})
	runtimeRoot := filepath.Join(data, "baseharbor", "targets", target, "runtime")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "installation.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDeploymentOwner(target, runtimeRoot, "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerMalformedState) {
		t.Fatalf("damaged Core state fell through to local authorization: %v", err)
	}
}

func TestLocalTargetOwnerRejectsPartialCoreRuntime(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data, config := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", config)
	target := "local-partial"
	saveLocalOwnerTestConfig(t, target, localOwnerTestDefinition("local", "docker", ""), AccessDefinition{Provider: "local", Reference: "local"})
	runtimeRoot := filepath.Join(data, "baseharbor", "targets", target, "runtime")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "runtime.env"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDeploymentOwner(target, runtimeRoot, "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("partial Core runtime was adopted by local authorization: %v", err)
	}
}

func TestLocalTargetOwnerRejectsSymlinkedTargetStateRoot(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data, config := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", config)
	target := "local-symlink"
	saveLocalOwnerTestConfig(t, target, localOwnerTestDefinition("local", "docker", ""), AccessDefinition{Provider: "local", Reference: "local"})
	targetsRoot := filepath.Join(data, "baseharbor", "targets")
	if err := os.MkdirAll(targetsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	externalRoot := t.TempDir()
	if err := os.Symlink(externalRoot, filepath.Join(targetsRoot, target)); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(targetsRoot, target, "runtime")
	if err := VerifyDeploymentOwner(target, runtimeRoot, "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("symlinked target state root was adopted as a coreless local target: %v", err)
	}
}
