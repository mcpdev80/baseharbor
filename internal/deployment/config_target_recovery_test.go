package deployment

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetRecoveryRestoresOnlyOriginalEntryAndReferences(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	original := cfg.Targets["first"]
	original.OpenBao.RecoveryFile = "/operator/recovery.json"
	original.OperatorAuth = map[string]OperatorAuthEnvironmentConfig{"prod": {Provider: "external-oidc", Issuer: "https://issuer.example", ClientID: "management"}}
	cfg.Targets["first"] = original
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.Targets, "first")
	owner, err := cfg.SaveExpectedPreservingTarget(raw, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := LoadTargetDefinitionRecovery(owner, "first")
	if err != nil {
		t.Fatal(err)
	}
	current, expected, err := LoadConfigForMutation()
	if err != nil {
		t.Fatal(err)
	}
	current.Targets["other"] = TargetDefinition{Runtime: RuntimeDefinition{Provider: "podman"}, Access: TargetAccess{Reference: "local"}}
	current, err = recovery.RestoreInto(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.SaveExpected(expected); err != nil {
		t.Fatal(err)
	}
	actual, err := LoadConfig()
	if err != nil || actual.Targets["first"].OpenBao.RecoveryFile != original.OpenBao.RecoveryFile || actual.Targets["first"].OperatorAuth["prod"].Issuer != original.OperatorAuth["prod"].Issuer || actual.Targets["other"].Runtime.Provider != "podman" {
		t.Fatal("recovery lost authentication/recovery references or overwrote another Target", err)
	}
	if _, err := recovery.RestoreInto(actual); err == nil {
		t.Fatal("recovery overwrote an existing Target")
	}
}

func TestTargetRecoveryNeverOverwritesExistingMaterial(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	r, err := TargetRecoveryFromConfig(raw, "first")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := r.OwnerPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(owner), 0700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("existing operator recovery")
	if err := os.WriteFile(owner, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	delete(cfg.Targets, "first")
	if _, err := cfg.SaveExpectedPreservingTarget(raw, "first", nil); !errors.Is(err, ErrConfigurationChanged) {
		t.Fatalf("existing recovery overwrite: %v", err)
	}
	after, err := os.ReadFile(owner)
	if err != nil || !bytes.Equal(after, foreign) {
		t.Fatal("existing recovery changed", err)
	}
	after, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("Target removed despite failed recovery preservation", err)
	}
}

func TestTargetRecoveryPreparationFailureRetainsOriginalAndSupportsRetry(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	delete(cfg.Targets, "first")
	owner, err := cfg.SaveExpectedPreservingTarget(raw, "first", func() error { return errors.New("selection preparation unavailable") })
	if err == nil {
		t.Fatal("preparation failure ignored")
	}
	if _, err := LoadTargetDefinitionRecovery(owner, "first"); err != nil {
		t.Fatal("interrupted preparation lost recovery", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("failed preparation removed Target", err)
	}
	retried, err := cfg.SaveExpectedPreservingTarget(raw, "first", nil)
	if err != nil || retried != owner {
		t.Fatal("same acknowledged recovery cannot retry", err)
	}
}

func TestTargetRecoveryRejectsChangedNameDataRootAndAccess(t *testing.T) {
	cfg, _, raw := configMutationFixture(t)
	delete(cfg.Targets, "first")
	owner, err := cfg.SaveExpectedPreservingTarget(raw, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTargetDefinitionRecovery(owner, "other"); err == nil {
		t.Fatal("cross-Target recovery accepted")
	}
	r, err := LoadTargetDefinitionRecovery(owner, "first")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Access["local"] = AccessDefinition{Provider: "baseharbor-node-connector", Reference: "foreign-node"}
	if _, err := r.RestoreInto(cfg); err == nil {
		t.Fatal("recovery replaced changed access authority")
	}
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	if _, err := LoadTargetDefinitionRecovery(owner, "first"); err == nil {
		t.Fatal("recovery silently rebound another data owner")
	}
}
