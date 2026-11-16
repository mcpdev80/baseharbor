package application

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestNativeBackendWithdrawalRequiresExistingExecutableCanonicalOwner(t *testing.T) {
	prior := WithDocumentDatabaseInstances(WithWorkloadComponents(New("retired", "dev", false, false, false), "api"), "records")
	withdrawn := prior
	withdrawn.Services = Services{}
	withdrawn.Workload = WorkloadConfig{}
	if withdrawn.ValidateIntent() != nil || withdrawn.Validate() == nil {
		t.Fatal("fixture does not distinguish portable withdrawal from executable intent")
	}
	files := RuntimeFilesFor(Store{Root: t.TempDir(), Namespace: "isolated"}, prior)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	environment := []byte("retained=private-recovery-material\n")
	if err := os.WriteFile(files.Env, environment, 0600); err != nil {
		t.Fatal(err)
	}
	canonical, err := RuntimeComposeYAMLForProject(prior, files.ResourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if err := CheckExistingNativeBackendInventory(files, withdrawn, prior); err != nil {
			t.Fatal("withdrawal obscures authoritative prior renderer inventory", err)
		}
	}
	if err := CheckExistingNativeBackendInventory(files, withdrawn); !errors.Is(err, ErrRuntimeDefinitionChanged) {
		t.Fatal("empty withdrawal alone invented native ownership", err)
	}
	damaged := withdrawn
	damaged.Name = "../foreign"
	if err := CheckExistingNativeBackendInventory(files, prior, damaged); err == nil {
		t.Fatal("matching prior hid invalid withdrawal")
	}
	after, err := os.Stat(files.Compose)
	data, readErr := os.ReadFile(files.Compose)
	env, envErr := os.ReadFile(files.Env)
	if err != nil || readErr != nil || envErr != nil || !before.ModTime().Equal(after.ModTime()) || !bytes.Equal(data, []byte(canonical)) || !bytes.Equal(env, environment) {
		t.Fatal("read-only inventory mutated native owner or recovery material")
	}
	if err := os.WriteFile(files.Compose, []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckExistingNativeBackendInventory(files, withdrawn, prior); !errors.Is(err, ErrRuntimeDefinitionChanged) {
		t.Fatal("withdrawal fabricated an empty canonical owner", err)
	}
}
