package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestParseAppBackupArgsRequiresPasswordFile(t *testing.T) {
	_, _, _, err := parseAppBackupArgs([]string{"mailflow"})
	if err == nil || !strings.Contains(err.Error(), "--password-file") {
		t.Fatalf("expected password-file error, got %v", err)
	}
}

func TestParseAppRestoreArgsRejectsPasswordInArgv(t *testing.T) {
	_, _, _, err := parseAppRestoreArgs([]string{"backup.bhbackup", "--password", "secret"})
	if err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("expected argv password rejection, got %v", err)
	}
}

func TestReadBackupPasswordFileRequiresOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("correct horse battery staple\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readBackupPasswordFile(path)
	if err == nil || !strings.Contains(err.Error(), "accessible by group or others") {
		t.Fatalf("expected permission rejection, got %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	password, err := readBackupPasswordFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zeroBytes(password)
	if string(password) != "correct horse battery staple" {
		t.Fatalf("password = %q", password)
	}
}

func TestWriteBackupArchiveRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.bhbackup")
	if err := writeBackupArchive(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeBackupArchive(path, []byte("second")); err == nil {
		t.Fatal("expected overwrite rejection")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first" {
		t.Fatalf("existing backup changed to %q", data)
	}
}

func TestRestoreNeedsDeploymentInitializationOnlyForMissingDevExposureState(t *testing.T) {
	devExposure := application.Manifest{
		Environment: "dev",
		Exposures:   []application.HTTPExposureRequirement{{Name: "web", Service: "app", Port: 8080, Protocol: "http", Visibility: "public"}},
	}
	if !restoreNeedsDeploymentInitialization(devExposure, repositoryInitState{}) {
		t.Fatal("missing DEV exposure state must be initialized during restore")
	}
	if restoreNeedsDeploymentInitialization(devExposure, repositoryInitState{Hostname: "app.baha.localhost"}) {
		t.Fatal("existing DEV exposure state must be reused")
	}

	prodExposure := devExposure
	prodExposure.Environment = "prod"
	if restoreNeedsDeploymentInitialization(prodExposure, repositoryInitState{}) {
		t.Fatal("production restore must not invent development deployment initialization")
	}

	noExposure := devExposure
	noExposure.Exposures = nil
	if restoreNeedsDeploymentInitialization(noExposure, repositoryInitState{}) {
		t.Fatal("restore without exposure must not initialize deployment access")
	}
}
