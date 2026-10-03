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
	state, found, err := bhruntime.LoadControlPlaneCredentialRotation(files)
	if err != nil {
		return err
	}

	if !found {
		current, err := bhruntime.LoadControlPlaneCredentials(files)
		if err != nil {
			return err
		}
		suffix, err := controlPlaneCredentialSuffix()
		if err != nil {
			return err
		}
		next := bhruntime.ControlPlaneCredentials{
			PostgresUser:             "baseharbor_admin_" + suffix,
			PostgresPassword:         mustControlPlaneSecret(),
			PostgresInternalUser:     "baseharbor_internal_" + suffix,
			PostgresInternalPassword: mustControlPlaneSecret(),
			PostgresReplicationUser:  "baseharbor_rep_" + suffix,
			PostgresReplicationPass:  mustControlPlaneSecret(),
			OpenBaoDBUser:            "openbao_runtime_" + suffix,
			OpenBaoDBPassword:        mustControlPlaneSecret(),
		}
		if next.PostgresPassword == "" || next.PostgresInternalPassword == "" || next.PostgresReplicationPass == "" || next.OpenBaoDBPassword == "" {
			return fmt.Errorf("generate replacement control-plane credentials")
		}
		state = bhruntime.ControlPlaneCredentialRotationState{
			Version:  1,
			Phase:    bhruntime.ControlPlaneRotationPrepared,
			Previous: current,
			Next:     next,
		}
		// Persist replacement identities before the first database mutation.
		// A process crash during preparation can therefore resume with the
		// exact same secrets instead of leaving unknown orphan credentials.
		if err := bhruntime.SaveControlPlaneCredentialRotation(files, state); err != nil {
			return fmt.Errorf("persist prepared control-plane credential rotation: %w", err)
		}
	}

	current := state.Previous
	next := state.Next

	if state.Phase == bhruntime.ControlPlaneRotationPrepared {
		if err := prepareControlPlaneDatabaseCredentialOverlap(ctx, runtime, files, current, next); err != nil {
			return err
		}
		if err := bhruntime.ReplaceControlPlaneCredentials(files, next); err != nil {
			return err
		}
		if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			_ = bhruntime.ReplaceControlPlaneCredentials(files, current)
			return fmt.Errorf("validate replacement control-plane credential projection: %w", err)
		}
		state.Phase = bhruntime.ControlPlaneRotationProjected
		if err := bhruntime.SaveControlPlaneCredentialRotation(files, state); err != nil {
			return fmt.Errorf("persist projected control-plane credential rotation: %w", err)
		}
	}

	if state.Phase == bhruntime.ControlPlaneRotationProjected {
		environment, err := bhruntime.RuntimeEnvironment(files)
		if err != nil {
			return err
		}
		workdir := filepath.Dir(files.Compose)

		if err := runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, workdir, environment, []string{"postgres-admin"}, files.Compose); err != nil {
			return fmt.Errorf("reconcile PostgreSQL administration client credential: %w", err)
		}
		if err := waitForControlPlanePostgresCredential(ctx, runtime, files, next.PostgresUser, next.PostgresPassword, "postgres"); err != nil {
			return err
		}

		primary, err := controlPlanePostgresPrimary(ctx, runtime, files)
		if err != nil {
			return err
		}
		members := []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"}
		order := make([]string, 0, len(members))
		for _, member := range members {
			if member != primary {
				order = append(order, member)
			}
		}
		order = append(order, primary)
		for _, member := range order {
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

		state.Phase = bhruntime.ControlPlaneRotationVerified
		if err := bhruntime.SaveControlPlaneCredentialRotation(files, state); err != nil {
			return fmt.Errorf("persist verified control-plane credential rotation: %w", err)
		}
	}

	if state.Phase == bhruntime.ControlPlaneRotationVerified {
		retireSQL := fmt.Sprintf(
			"ALTER ROLE %s NOLOGIN; ALTER ROLE %s NOLOGIN; ALTER ROLE %s NOLOGIN; ALTER ROLE %s NOLOGIN;",
			quoteControlPlaneIdent(current.PostgresUser),
			quoteControlPlaneIdent(current.PostgresInternalUser),
			quoteControlPlaneIdent(current.PostgresReplicationUser),
			quoteControlPlaneIdent(current.OpenBaoDBUser),
		)
		if err := execControlPlanePostgresSQL(ctx, runtime, files, next.PostgresUser, next.PostgresPassword, "postgres", retireSQL); err != nil {
			return fmt.Errorf("retire previous control-plane database credentials: %w", err)
		}

		for _, old := range []struct {
			user, password, database string
		}{
			{current.PostgresUser, current.PostgresPassword, "postgres"},
			{current.PostgresInternalUser, current.PostgresInternalPassword, "postgres"},
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
		if err := verifyOpenBaoManagementUI(ctx, files); err != nil {
			return err
		}

		state.Phase = bhruntime.ControlPlaneRotationRetired
		if err := bhruntime.SaveControlPlaneCredentialRotation(files, state); err != nil {
			return fmt.Errorf("persist retired control-plane credential rotation: %w", err)
		}
	}

	if state.Phase == bhruntime.ControlPlaneRotationRetired {
		if err := bhruntime.ClearControlPlaneCredentialRotation(files); err != nil {
			return fmt.Errorf("clear completed control-plane credential rotation: %w", err)
		}
	}
	return nil
}

func prepareControlPlaneDatabaseCredentialOverlap(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, current, next bhruntime.ControlPlaneCredentials) error {
	prepareSQL := fmt.Sprintf(
		"SELECT format('CREATE ROLE %%I', %s) WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) \\gexec\n"+
			"ALTER ROLE %s WITH LOGIN SUPERUSER PASSWORD %s;\n"+
			"SELECT format('CREATE ROLE %%I', %s) WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) \\gexec\n"+
			"ALTER ROLE %s WITH LOGIN SUPERUSER PASSWORD %s;\n"+
			"SELECT format('CREATE ROLE %%I', %s) WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) \\gexec\n"+
			"ALTER ROLE %s WITH LOGIN REPLICATION PASSWORD %s;\n"+
			"SELECT format('CREATE ROLE %%I', %s) WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) \\gexec\n"+
			"ALTER ROLE %s WITH LOGIN PASSWORD %s;\n"+
			"GRANT %s TO %s;\n",
		quoteControlPlaneLiteral(next.PostgresUser), quoteControlPlaneLiteral(next.PostgresUser),
		quoteControlPlaneIdent(next.PostgresUser), quoteControlPlaneLiteral(next.PostgresPassword),
		quoteControlPlaneLiteral(next.PostgresInternalUser), quoteControlPlaneLiteral(next.PostgresInternalUser),
		quoteControlPlaneIdent(next.PostgresInternalUser), quoteControlPlaneLiteral(next.PostgresInternalPassword),
		quoteControlPlaneLiteral(next.PostgresReplicationUser), quoteControlPlaneLiteral(next.PostgresReplicationUser),
		quoteControlPlaneIdent(next.PostgresReplicationUser), quoteControlPlaneLiteral(next.PostgresReplicationPass),
		quoteControlPlaneLiteral(next.OpenBaoDBUser), quoteControlPlaneLiteral(next.OpenBaoDBUser),
		quoteControlPlaneIdent(next.OpenBaoDBUser), quoteControlPlaneLiteral(next.OpenBaoDBPassword),
		quoteControlPlaneIdent(current.OpenBaoDBUser), quoteControlPlaneIdent(next.OpenBaoDBUser),
	)
	if err := execControlPlanePostgresSQL(ctx, runtime, files, current.PostgresUser, current.PostgresPassword, "postgres", prepareSQL); err != nil {
		return fmt.Errorf("prepare replacement control-plane database credentials: %w", err)
	}
	for _, probe := range []struct {
		user, password, database string
	}{
		{next.PostgresUser, next.PostgresPassword, "postgres"},
		{next.PostgresInternalUser, next.PostgresInternalPassword, "postgres"},
		{next.PostgresReplicationUser, next.PostgresReplicationPass, "postgres"},
		{next.OpenBaoDBUser, next.OpenBaoDBPassword, "openbao"},
	} {
		if err := probeControlPlanePostgresCredential(ctx, runtime, files, probe.user, probe.password, probe.database); err != nil {
			return fmt.Errorf("verify prepared control-plane database credential for %s: %w", probe.user, err)
		}
	}
	return nil
}

func controlPlanePostgresPrimary(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files) (string, error) {
	const probe = "import urllib.request,sys;\ntry:\n r=urllib.request.urlopen('http://127.0.0.1:8008/primary', timeout=2); sys.exit(0 if r.status == 200 else 1)\nexcept Exception:\n sys.exit(1)"
	for _, member := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
		if _, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, member, "python3", "-c", probe); err == nil {
			return member, nil
		}
	}
	return "", fmt.Errorf("no Patroni PostgreSQL primary found")
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
