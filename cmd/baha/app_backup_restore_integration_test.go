package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestAppBackupRestoreCLIRealDisasterRecovery(t *testing.T) {
	if os.Getenv("BASEHARBOR_APP_BACKUP_ACCEPTANCE") != "true" {
		t.Skip("application backup CLI acceptance is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := runtimeUp(ctx, io.Discard); err != nil {
		t.Fatalf("runtime up: %v", err)
	}
	defer func() { _ = runtimeDown(context.Background(), io.Discard) }()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatal(err)
	}
	platformFiles, err := bhruntime.ExistingFiles("")
	if err != nil {
		t.Fatal(err)
	}
	recoveryPath := filepath.Join(root, "openbao-recovery.json")
	if err := openbao.Bootstrap(ctx, compose, platformFiles, recoveryPath); err != nil {
		t.Fatalf("OpenBao bootstrap: %v", err)
	}

	manifest := `version: 1
app:
  name: backup-cli
  environment: dev
services:
  postgres:
    enabled: true
  redis:
    enabled: false
  secrets:
    enabled: true
`
	if err := os.WriteFile("baseharbor.yaml", []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "apply"}, &output, &output); err != nil {
		t.Fatalf("app apply: %v\n%s", err, output.String())
	}

	store := application.DefaultStore()
	resolved, err := resolveApplication(store, nil, "backup acceptance")
	if err != nil {
		t.Fatal(err)
	}
	m := resolved.Manifest
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compose.ExecProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env, "postgres", "psql", "-U", "baseharbor", "-d", "backup_cli_dev", "-v", "ON_ERROR_STOP=1", "-c", "CREATE TABLE recovery_probe(value text); INSERT INTO recovery_probe(value) VALUES ('survived');"); err != nil {
		t.Fatalf("seed postgres: %v", err)
	}
	secretService := applicationsecret.New(resolved.Store)
	if err := secretService.Set(ctx, m.Name, "API_TOKEN", []byte("provider-key-v1")); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	tokenBefore, err := os.ReadFile(application.RuntimeIdentityTokenPath(files))
	if err != nil {
		t.Fatal(err)
	}
	clientCertPath := filepath.Join(files.Bindings, "runtime-identity", "client-cert.pem")
	certBefore, err := os.ReadFile(clientCertPath)
	if err != nil {
		t.Fatal(err)
	}

	passwordPath := filepath.Join(root, "backup-password")
	if err := os.WriteFile(passwordPath, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(root, "backup-cli.bhbackup")
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "backup", "--password-file", passwordPath, "--output", backupPath}, &output, &output); err != nil {
		t.Fatalf("app backup: %v\n%s", err, output.String())
	}
	metadata, err := resolved.Store.LastBackup(m.Name)
	if err != nil {
		t.Fatalf("load recorded backup metadata: %v", err)
	}
	if metadata.Application != m.Name || metadata.Environment != m.Environment || metadata.CreatedAt.IsZero() || metadata.ArchivePath != backupPath || len(metadata.PostgresResources) != 1 || metadata.PostgresResources[0] != "default" || !metadata.IncludesSecrets {
		t.Fatalf("unexpected recorded backup metadata: %#v", metadata)
	}
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "show"}, &output, &output); err != nil {
		t.Fatalf("app show after backup: %v\n%s", err, output.String())
	}
	showOutput := output.String()
	if !strings.Contains(showOutput, "Last backup") || !strings.Contains(showOutput, backupPath) || !strings.Contains(showOutput, "managed secrets      included") {
		t.Fatalf("app show did not report last backup metadata:\n%s", showOutput)
	}
	if strings.Contains(showOutput, "provider-key-v1") || strings.Contains(showOutput, "API_TOKEN") {
		t.Fatalf("app show exposed secret-bearing backup detail:\n%s", showOutput)
	}

	output.Reset()
	if err := runWithIO(ctx, []string{"app", "destroy", "--yes"}, &output, &output); err != nil {
		t.Fatalf("app destroy: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "restore", backupPath, "--password-file", passwordPath}, &output, &output); err != nil {
		t.Fatalf("app restore: %v\n%s", err, output.String())
	}

	resolved, err = resolveApplication(application.DefaultStore(), nil, "backup acceptance verify")
	if err != nil {
		t.Fatal(err)
	}
	files, err = application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	query, err := compose.ExecProject(ctx, application.RuntimeProjectName(resolved.Manifest), files.Compose, files.Env, "postgres", "psql", "-U", "baseharbor", "-d", "backup_cli_dev", "-tAc", "SELECT value FROM recovery_probe")
	if err != nil {
		t.Fatalf("verify postgres: %v", err)
	}
	if string(bytes.TrimSpace([]byte(query))) != "survived" {
		t.Fatalf("restored postgres value = %q", query)
	}
	secretService = applicationsecret.New(resolved.Store)
	secretValue, err := secretService.Get(ctx, resolved.Manifest.Name, "API_TOKEN")
	if err != nil {
		t.Fatalf("verify restored secret: %v", err)
	}
	if !bytes.Equal(secretValue, []byte("provider-key-v1")) {
		t.Fatalf("restored secret = %q", secretValue)
	}
	tokenAfter, err := os.ReadFile(application.RuntimeIdentityTokenPath(files))
	if err != nil {
		t.Fatal(err)
	}
	certAfter, err := os.ReadFile(filepath.Join(files.Bindings, "runtime-identity", "client-cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(tokenBefore, tokenAfter) {
		t.Fatal("runtime token was replayed instead of regenerated")
	}
	if bytes.Equal(certBefore, certAfter) {
		t.Fatal("runtime mTLS client certificate was replayed instead of regenerated")
	}
}
