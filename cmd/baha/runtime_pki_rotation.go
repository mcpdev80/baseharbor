package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func waitForOpenBaoReplacementCA(ctx context.Context, material serviceaccess.TLSMaterial, endpoint string, activeCA []byte) error {
	if len(activeCA) == 0 {
		return errors.New("active OpenBao service CA is empty")
	}
	file, err := os.CreateTemp("", "baseharbor-active-openbao-ca-*.pem")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(activeCA); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	material.CA = file.Name()
	client, err := serviceaccess.NewHTTPClient(material, false)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	verifyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return serviceaccess.WaitHTTPS(verifyCtx, client, endpoint, "/v1/sys/health")
}

func rotateControlPlaneServiceCA(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, recoveryFile string) error {
	stateDir := filepath.Dir(files.Compose)
	oldOpenBaoCA, err := os.ReadFile(filepath.Join(stateDir, "providers", "openbao", "service-access", "pki", "ca.pem"))
	if err != nil {
		return fmt.Errorf("read previous OpenBao service CA: %w", err)
	}
	oldPostgresCA, err := os.ReadFile(filepath.Join(stateDir, "providers", "postgresql", "service-access", "pki", "ca.pem"))
	if err != nil {
		return fmt.Errorf("read previous PostgreSQL service CA: %w", err)
	}

	if err := platformopenbao.RotateServiceCA(ctx, runtime, files); err != nil {
		return err
	}
	issuer := platformopenbao.NewServiceIssuer(runtime, files)
	if err := reconcileControlPlaneServiceAccess(ctx, runtime, files, recoveryFile); err != nil {
		return fmt.Errorf("reconcile replacement control-plane PKI with overlap: %w", err)
	}

	credentials, err := bhruntime.LoadControlPlaneCredentials(files)
	if err != nil {
		return err
	}
	if err := probeControlPlanePostgresTLS(ctx, runtime, files, credentials.PostgresUser, credentials.PostgresPassword, "postgres"); err != nil {
		return fmt.Errorf("verify PostgreSQL with replacement service CA before retirement: %w", err)
	}
	if err := verifyOpenBaoManagementUI(ctx, files); err != nil {
		return fmt.Errorf("verify OpenBao management UI with replacement service CA before retirement: %w", err)
	}

	if err := bhruntime.RetireControlPlaneServiceAccessOverlap(ctx, issuer, files); err != nil {
		return err
	}

	if err := probeControlPlanePostgresTLS(ctx, runtime, files, credentials.PostgresUser, credentials.PostgresPassword, "postgres"); err != nil {
		return fmt.Errorf("verify PostgreSQL after old CA retirement: %w", err)
	}
	if err := verifyOpenBaoManagementUI(ctx, files); err != nil {
		return fmt.Errorf("verify OpenBao management UI after old CA retirement: %w", err)
	}

	if err := verifyOldOpenBaoCARejected(ctx, files, oldOpenBaoCA); err != nil {
		return err
	}
	if err := verifyOldPostgresCARejected(ctx, runtime, files, credentials, oldPostgresCA); err != nil {
		return err
	}
	return nil
}

func probeControlPlanePostgresTLS(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, user, password, database string) error {
	const script = "IFS= read -r PGPASSWORD\nexport PGPASSWORD\nPGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem exec psql -h postgres -p 5432 -U \"$1\" -d \"$2\" -v ON_ERROR_STOP=1 -Atqc 'SELECT 1'"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(password+"\n"), "postgres-admin", "sh", "-ec", script, "--", user, database)
	return err
}

func verifyOldOpenBaoCARejected(ctx context.Context, files bhruntime.Files, oldCA []byte) error {
	if len(oldCA) == 0 {
		return errors.New("previous OpenBao service CA is empty")
	}
	path := filepath.Join(filepath.Dir(files.Env), ".old-openbao-ca.pem")
	if err := os.WriteFile(path, oldCA, 0o600); err != nil {
		return err
	}
	defer os.Remove(path)

	cfg, err := bhruntime.LoadConfig(files.Env)
	if err != nil {
		return err
	}
	client, err := serviceaccess.NewHTTPClient(serviceaccess.TLSMaterial{CA: path, ServerName: "openbao"}, false)
	if err != nil {
		return err
	}
	client.Timeout = 5 * time.Second
	endpoint, err := serviceaccess.LoopbackHTTPSURL(cfg.OpenBaoPort)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v1/sys/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		return errors.New("previous OpenBao service CA still validates the rotated endpoint")
	}
	return nil
}

func verifyOldPostgresCARejected(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, credentials bhruntime.ControlPlaneCredentials, oldCA []byte) error {
	if len(oldCA) == 0 {
		return errors.New("previous PostgreSQL service CA is empty")
	}
	encoded := base64.StdEncoding.EncodeToString(oldCA)
	input := []byte(encoded + "\n" + credentials.PostgresPassword + "\n")
	const script = "IFS= read -r OLD_CA_B64\nIFS= read -r PGPASSWORD\nexport PGPASSWORD\nprintf '%s' \"$OLD_CA_B64\" | base64 -d >/tmp/baseharbor-old-postgres-ca.pem\ntrap 'rm -f /tmp/baseharbor-old-postgres-ca.pem' EXIT\nPGSSLMODE=verify-full PGSSLROOTCERT=/tmp/baseharbor-old-postgres-ca.pem exec psql -h postgres -p 5432 -U \"$1\" -d \"$2\" -v ON_ERROR_STOP=1 -Atqc 'SELECT 1'"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, input, "postgres-admin", "sh", "-ec", script, "--", credentials.PostgresUser, "postgres")
	if err == nil {
		return errors.New("previous PostgreSQL service CA still validates the rotated endpoint")
	}
	if strings.Contains(strings.ToLower(err.Error()), "base64") && strings.Contains(strings.ToLower(err.Error()), "not found") {
		return fmt.Errorf("cannot verify previous PostgreSQL CA rejection: %w", err)
	}
	return nil
}
