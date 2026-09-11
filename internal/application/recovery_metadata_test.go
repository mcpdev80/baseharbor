package application

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecoveryMetadataRoundTripAndPermissions(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	restoredAt := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	backupCreatedAt := time.Date(2026, 9, 11, 11, 0, 0, 0, time.UTC)
	metadata := RecoveryMetadata{
		Version:         LastRecoveryMetadataVersion,
		Application:     "mailflow",
		Environment:     "production",
		RestoredAt:      restoredAt,
		BackupCreatedAt: backupCreatedAt,
		ArchivePath:     "/backups/mailflow.bhbackup",
	}
	if err := store.RecordLastRecovery(metadata); err != nil {
		t.Fatal(err)
	}
	got, err := store.LastRecovery("mailflow")
	if err != nil {
		t.Fatal(err)
	}
	if got != metadata {
		t.Fatalf("recovery metadata=%+v want %+v", got, metadata)
	}
	info, err := os.Stat(filepath.Join(store.Root, "mailflow", lastRecoveryMetadataName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("recovery metadata mode=%o want 600", info.Mode().Perm())
	}
}

func TestLastRecoveryMissing(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	_, err := store.LastRecovery("mailflow")
	if !errors.Is(err, ErrNoRecoveryMetadata) {
		t.Fatalf("LastRecovery error=%v want ErrNoRecoveryMetadata", err)
	}
}

func TestLastRecoveryRejectsInvalidApplicationName(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	if _, err := store.LastRecovery("../escape"); err == nil {
		t.Fatal("LastRecovery accepted invalid application name")
	}
}
