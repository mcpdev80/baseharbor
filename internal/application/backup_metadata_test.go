package application

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupMetadataRoundTrip(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	createdAt := time.Date(2026, 9, 11, 0, 30, 0, 0, time.UTC)
	metadata := BackupMetadata{
		Version:           LastBackupMetadataVersion,
		Application:       "mailflow",
		Environment:       "production",
		CreatedAt:         createdAt,
		ArchivePath:       "/backups/mailflow-production.bhbackup",
		PostgresResources: []string{"default"},
		IncludesSecrets:   true,
	}
	if err := store.RecordLastBackup(metadata); err != nil {
		t.Fatal(err)
	}
	got, err := store.LastBackup("mailflow")
	if err != nil {
		t.Fatal(err)
	}
	if got.Application != metadata.Application || got.Environment != metadata.Environment || !got.CreatedAt.Equal(createdAt) || got.ArchivePath != metadata.ArchivePath || len(got.PostgresResources) != 1 || got.PostgresResources[0] != "default" || !got.IncludesSecrets {
		t.Fatalf("unexpected metadata: %#v", got)
	}
	info, err := os.Stat(filepath.Join(store.Root, "mailflow", lastBackupMetadataName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup metadata permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLastBackupMissingIsExplicit(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	_, err := store.LastBackup("mailflow")
	if !errors.Is(err, ErrNoBackupMetadata) {
		t.Fatalf("expected ErrNoBackupMetadata, got %v", err)
	}
}

func TestBackupMetadataRejectsInvalidIdentity(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	err := store.RecordLastBackup(BackupMetadata{
		Version:     LastBackupMetadataVersion,
		Application: "../escape",
		Environment: "production",
		CreatedAt:   time.Now().UTC(),
		ArchivePath: "/tmp/backup.bhbackup",
	})
	if err == nil {
		t.Fatal("expected invalid application identity to be rejected")
	}
}
