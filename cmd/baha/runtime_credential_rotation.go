package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func rotateControlPlaneDatabaseCredentials(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, recoveryFile string) error {
	current, err := bhruntime.LoadControlPlaneCredentials(files)
	if err != nil {
		return err
	}
	suffix, err := controlPlaneCredentialSuffix()
	if err != nil {
		return err
	}
	next := bhruntime.ControlPlaneCredentials{
		PostgresUser:            "baseharbor_admin_" + suffix,
		PostgresPassword:        mustControlPlaneSecret(),
		PostgresReplicationUser: "baseharbor_rep_" + suffix,
		PostgresReplicationPass: mustControlPlaneSecret(),
		OpenBaoDBUser:           "openbao_runtime_" + suffix,
		OpenBaoDBPassword:       mustControlPlaneSecret(),
	}
	if next.PostgresPassword == "" || next.PostgresReplicationPass == "" || next.OpenBaoDBPassword == "" {
		return fmt.Errorf("generate replacement control-plane credentials")
	}

	prepareSQL := fmt.Sprintf(
		"CREATE ROLE %s WITH LOGIN SUPERUSER PASSWORD %s;\n"+
			"CREATE ROLE %s WITH LOGIN REPLICATION PASSWORD %s;\n"+
			"CREATE ROLE %s WITH LOGIN PASSWORD %s IN ROLE openbao;\n",
		quoteControlPlaneIdent(next.PostgresUser), quoteControlPlaneLiteral(next.PostgresPassword),
		quoteControlPlaneIdent(next.PostgresReplicationUser), quoteControlPlaneLiteral(next.PostgresReplicationPass),
		quoteControlPlaneIdent(next.OpenBaoDBUser), quoteControlPlaneLiteral(next.OpenBaoDBPassword),
	)
	if err := execControlPlanePostgresSQL(ctx, runtime, files, current.PostgresUser, current.PostgresPassword, "postgres", prepareSQL); err != nil {
		return fmt.Errorf("prepare replacement control-plane database credentials: %w", err)
	}

	cleanupPrepared := true
	defer func() {
		if !cleanupPrepared {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cleanupSQL := fmt.Sprintf(
			"DROP ROLE IF EXISTS %s; DROP ROLE IF EXISTS %s; DROP ROLE IF EXISTS %s;",
			quoteControlPlaneIdent(next.OpenBaoDBUser),
			quoteControlPlaneIdent(next.PostgresReplicationUser),
			quoteControlPlaneIdent(next.PostgresUser),
		)
		_ = execControlPlanePostgresSQL(cleanupCtx, runtime, files, current.PostgresUser, current.PostgresPassword, "postgres", cleanupSQL)
	}()

	for _, probe := range []struct {
		user, password, database string
	}{
		{next.PostgresUser, next.PostgresPassword, "postgres"},
		{next.PostgresReplicationUser, next.PostgresReplicationPass, "postgres"},
		{next.OpenBaoDBUser, next.OpenBaoDBPassword, "openbao"},
	} {
		if err := probeControlPlanePostgresCredential(ctx, runtime, files, probe.user, probe.password, probe.database); err != nil {
			return fmt.Errorf("verify prepared control-plane database credential for %s: %w", probe.user, err)
		}
	}

	if err := bhruntime.ReplaceControlPlaneCredentials(files, next); err != nil {
		return err
	}
	environment, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		return err
	}
	workdir := filepath.Dir(files.Compose)
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate replacement control-plane credential projection: %w", err)
	}

	if err := runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, workdir, environment, []string{"postgres-admin"}, files.Compose); err != nil {
		return fmt.Errorf("reconcile PostgreSQL administration client credential: %w", err)
	}
	if err := waitForControlPlanePostgresCredential(ctx, runtime, files, next.PostgresUser, next.PostgresPassword, "postgres"); err != nil {
		return err
	}

	for _, member := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
		if err := runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, workdir, environment, []string{member}, files.Compose); err != nil {
			return fmt.Errorf("roll PostgreSQL member %s: %w", member, err)
		}
		if err := waitForControlPlanePostgresCredential(ctx, runtime, files, next.PostgresUser, next.PostgresPassword, "postgres"); err != nil {
			return fmt.Errorf("verify PostgreSQL after rolling %s: %w", member, err)
		}
	}

	for _, member := range []string{"openbao-member-1", "openbao-member-2", "openbao-member-3"} {
		if err := runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, workdir, environment, []string{member}, files.Compose); err != nil {
			return fmt.Errorf("roll OpenBao member %s: %w", member, err)
		}
		if err := waitForOpenBaoUnsealAfterCredentialRotation(ctx, runtime, files, recoveryFile); err != nil {
			return fmt.Errorf("restore OpenBao HA member after rolling %s: %w", member, err)
		}
		if err := platformopenbao.CheckManager(ctx, runtime, files); err != nil {
			return fmt.Errorf("verify OpenBao manager after rolling %s: %w", member, err)
		}
		if err := verifyOpenBaoManagementUI(ctx, files); err != nil {
			return fmt.Errorf("verify OpenBao management UI after rolling %s: %w", member, err)
		}
	}

	retireSQL := fmt.Sprintf(
		"ALTER ROLE %s NOLOGIN; ALTER ROLE %s NOLOGIN; ALTER ROLE %s NOLOGIN;",
		quoteControlPlaneIdent(current.PostgresUser),
		quoteControlPlaneIdent(current.PostgresReplicationUser),
		quoteControlPlaneIdent(current.OpenBaoDBUser),
	)
	if err := execControlPlanePostgresSQL(ctx, runtime, files, next.PostgresUser, next.PostgresPassword, "postgres", retireSQL); err != nil {
		return fmt.Errorf("retire previous control-plane database credentials: %w", err)
	}
	cleanupPrepared = false

	for _, old := range []struct {
		user, password, database string
	}{
		{current.PostgresUser, current.PostgresPassword, "postgres"},
		{current.PostgresReplicationUser, current.PostgresReplicationPass, "postgres"},
		{current.OpenBaoDBUser, current.OpenBaoDBPassword, "openbao"},
	} {
		if err := probeControlPlanePostgresCredential(ctx, runtime, files, old.user, old.password, old.database); err == nil {
			return fmt.Errorf("previous control-plane credential for %s still authenticates after retirement", old.user)
		}
	}
	if err := platformopenbao.CheckManager(ctx, runtime, files); err != nil {
		return fmt.Errorf("verify OpenBao after control-plane credential retirement: %w", err)
	}
	return verifyOpenBaoManagementUI(ctx, files)
}

func execControlPlanePostgresSQL(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, user, password, database, sql string) error {
	const script = "IFS= read -r PGPASSWORD\nexport PGPASSWORD\nexec psql -h postgres -p 5432 -U \"$1\" -d \"$2\" -v ON_ERROR_STOP=1"
	input := []byte(password + "\n" + sql + "\n")
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, input, "postgres-admin", "sh", "-ec", script, "--", user, database)
	return err
}

func probeControlPlanePostgresCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, user, password, database string) error {
	const script = "IFS= read -r PGPASSWORD\nexport PGPASSWORD\nexec psql -h postgres -p 5432 -U \"$1\" -d \"$2\" -v ON_ERROR_STOP=1 -Atqc 'SELECT 1'"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(password+"\n"), "postgres-admin", "sh", "-ec", script, "--", user, database)
	return err
}

func waitForControlPlanePostgresCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, user, password, database string) error {
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		last = probeControlPlanePostgresCredential(probeCtx, runtime, files, user, password, database)
		cancel()
		if last == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("stable PostgreSQL endpoint did not authenticate replacement credential: %w", last)
}

func waitForOpenBaoUnsealAfterCredentialRotation(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, recoveryFile string) error {
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		last = platformopenbao.Unseal(probeCtx, runtime, files, recoveryFile)
		cancel()
		if last == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("OpenBao HA members did not become unsealed after rolling credential change: %w", last)
}

func controlPlaneCredentialSuffix() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToLower(base64.RawURLEncoding.EncodeToString(buf)), nil
}

func mustControlPlaneSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func quoteControlPlaneIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteControlPlaneLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
