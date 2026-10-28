package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// streamPatroniBasebackup performs PostgreSQL's native physical online backup,
// with WAL fetched into the same tar stream. The credential is supplied solely
// over stdin, never in argv, logs, or receipts. The archive must be separately
// verified and retained before any member is restarted.
func streamPatroniBasebackup(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, leader, user, password string, dest io.Writer) error {
	if rt == nil || !files.HA || files.Project == "" || files.Compose == "" || leader == "" ||
		user == "" || password == "" || dest == nil {
		return errors.New("native Patroni backup requires owned HA runtime and replication credentials")
	}
	allowed := false
	for _, member := range files.PostgresMembers() {
		if member == leader {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("native PostgreSQL backup leader is not an owned member")
	}
	environment, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		return fmt.Errorf("native PostgreSQL backup environment: %w", err)
	}
	const script = "IFS= read -r PGPASSWORD || exit 1\nexport PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem PGCONNECT_TIMEOUT=10\nexec pg_basebackup -h \"$1\" -U \"$2\" -D - -Ft -X fetch -c fast --no-password"
	if err := rt.RunProjectFilesEnv(ctx, files.Project, filepath.Dir(files.Compose), environment,
		strings.NewReader(password+"\n"), dest, io.Discard,
		[]string{files.Compose}, "exec", "-T", "postgres-admin", "sh", "-ec", script, "--", leader, user); err != nil {
		return fmt.Errorf("native Patroni physical backup failed: %w", err)
	}
	return nil
}

func prepareOwnedPatroniPhysicalRestore(ctx context.Context, journalDir string) error {
	point := coreupdate.StreamRecoveryPoint{Directory: filepath.Join(journalDir, "patroni-recovery"), Name: "core-spilo-basebackup"}
	destination := filepath.Join(journalDir, "patroni-isolated-restore")
	if _, err := os.Lstat(destination); errors.Is(err, os.ErrNotExist) {
		if err := coreupdate.RestorePostgresBasebackup(ctx, point, "18", destination); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return coreupdate.VerifyPostgresBasebackupRestore(ctx, point, "18", destination)
}

// captureOwnedPatroniBackup never runs on an unverifiable cluster; it writes a
// streaming, checksum-protected and immutable physical backup into Core state.
// A physical basebackup is a recoverable *data* point, not an etcd DCS snapshot,
// and does NOT authorize an unsupported Spilo image migration by itself.
func captureOwnedPatroniBackup(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, directory string) error {
	return captureOwnedPatroniBackupFromLeader(ctx, rt, files, directory, "")
}

func awaitOwnedPatroniQuorum(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, expectedLeader string) (string, error) {
	quorumCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return coreupdate.AwaitPatroniQuorum(quorumCtx, expectedLeader, 2*time.Second, func(ctx context.Context) ([]coreupdate.PatroniMemberState, error) {
		return inspectPatroniMembers(ctx, rt, files)
	})
}

func captureOwnedPatroniBackupFromLeader(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, directory, expectedLeader string) error {
	leader, err := awaitOwnedPatroniQuorum(ctx, rt, files, expectedLeader)
	if err != nil {
		return fmt.Errorf("native backup Patroni quorum: %w", err)
	}
	creds, err := bhruntime.LoadControlPlaneCredentials(files)
	if err != nil {
		return fmt.Errorf("native backup replication credentials unavailable: %w", err)
	}
	recovery := coreupdate.StreamRecoveryPoint{Directory: directory, Name: "core-spilo-basebackup"}
	if err := recovery.Capture(ctx, func(ctx context.Context, dest io.Writer) error {
		return streamPatroniBasebackup(ctx, rt, files, leader, creds.PostgresReplicationUser, creds.PostgresReplicationPass, dest)
	}); err != nil {
		return err
	}
	return coreupdate.VerifyPostgresBasebackup(recovery, "18")
}
