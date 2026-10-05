package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplacementCredentialsKeepPreviousReplicationIdentityInProjection(t *testing.T) {
	files, err := EnsureFilesWithPorts(t.TempDir(), Ports{Postgres: 15432, OpenBao: 18200})
	if err != nil {
		t.Fatal(err)
	}
	previous, err := LoadControlPlaneCredentials(files)
	if err != nil {
		t.Fatal(err)
	}
	next := previous
	next.PostgresReplicationUser = "replacement_replication"
	next.PostgresReplicationPass = "replacement-password"
	if err := SaveControlPlaneCredentialRotation(files, ControlPlaneCredentialRotationState{
		Version: 1, Phase: ControlPlaneRotationPrepared, Previous: previous, Next: next,
	}); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceControlPlaneCredentials(files, next); err != nil {
		t.Fatal(err)
	}
	// Simulate resuming after projection but before the phase was persisted.
	if err := ReplaceControlPlaneCredentials(files, next); err != nil {
		t.Fatal(err)
	}
	environment, err := RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	if environment["BASEHARBOR_POSTGRES_REPLICATION_PREVIOUS_USER"] != previous.PostgresReplicationUser ||
		environment["BASEHARBOR_POSTGRES_REPLICATION_USER"] != next.PostgresReplicationUser {
		t.Fatal("rolling member projection does not admit both replication identities")
	}
}

func TestControlPlaneCredentialRotationStateSurvivesRestartOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	files := Files{Env: filepath.Join(dir, "runtime.env"), Compose: filepath.Join(dir, "compose.yaml")}
	state := ControlPlaneCredentialRotationState{
		Version: 1,
		Phase:   ControlPlaneRotationPrepared,
		Previous: ControlPlaneCredentials{
			PostgresUser: "old-admin", PostgresPassword: "old-admin-pass",
			PostgresInternalUser: "postgres", PostgresInternalPassword: "old-internal-pass",
			PostgresReplicationUser: "old-repl", PostgresReplicationPass: "old-repl-pass",
			OpenBaoDBUser: "old-openbao", OpenBaoDBPassword: "old-openbao-pass",
		},
		Next: ControlPlaneCredentials{
			PostgresUser: "new-admin", PostgresPassword: "new-admin-pass",
			PostgresInternalUser: "new-internal", PostgresInternalPassword: "new-internal-pass",
			PostgresReplicationUser: "new-repl", PostgresReplicationPass: "new-repl-pass",
			OpenBaoDBUser: "new-openbao", OpenBaoDBPassword: "new-openbao-pass",
		},
	}
	if err := SaveControlPlaneCredentialRotation(files, state); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, controlPlaneCredentialRotationStateName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("rotation state mode = %o, want 600", got)
	}

	got, found, err := LoadControlPlaneCredentialRotation(files)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got.Phase != ControlPlaneRotationPrepared ||
		got.Previous.PostgresPassword != state.Previous.PostgresPassword ||
		got.Previous.PostgresInternalPassword != state.Previous.PostgresInternalPassword ||
		got.Next.OpenBaoDBPassword != state.Next.OpenBaoDBPassword {
		t.Fatalf("reloaded rotation state = %#v", got)
	}

	got.Phase = ControlPlaneRotationVerified
	if err := SaveControlPlaneCredentialRotation(files, got); err != nil {
		t.Fatal(err)
	}
	reloaded, found, err := LoadControlPlaneCredentialRotation(files)
	if err != nil {
		t.Fatal(err)
	}
	if !found || reloaded.Phase != ControlPlaneRotationVerified {
		t.Fatalf("resumed phase = %q, found=%v", reloaded.Phase, found)
	}

	if err := ClearControlPlaneCredentialRotation(files); err != nil {
		t.Fatal(err)
	}
	if err := ClearControlPlaneCredentialRotation(files); err != nil {
		t.Fatal(err)
	}
	_, found, err = LoadControlPlaneCredentialRotation(files)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("completed rotation state still exists")
	}
}
