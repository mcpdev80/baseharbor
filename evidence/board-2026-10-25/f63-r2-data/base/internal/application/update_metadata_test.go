package application

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAndLoadLastUpdateMetadata(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	now := time.Now().UTC().Truncate(time.Second)
	backupAt := now.Add(-time.Minute)
	metadata := UpdateMetadata{
		Version:         LastUpdateMetadataVersion,
		Application:     "mailflow",
		Environment:     "production",
		UpdatedAt:       now,
		Branch:          "main",
		Upstream:        "origin/main",
		FromRevision:    "1111111111111111111111111111111111111111",
		ToRevision:      "2222222222222222222222222222222222222222",
		Result:          "ready",
		BackupPath:      "/safe/mailflow-pre-update.bhbackup",
		BackupCreatedAt: backupAt,
	}
	if err := store.RecordLastUpdate(metadata); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LastUpdate("mailflow")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Result != "ready" || loaded.FromRevision != metadata.FromRevision || loaded.ToRevision != metadata.ToRevision || loaded.BackupPath != metadata.BackupPath {
		t.Fatalf("unexpected update metadata: %#v", loaded)
	}
	path := filepath.Join(store.Root, "mailflow", lastUpdateMetadataName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("update metadata permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestUpdateMetadataRejectsUnsafeShapes(t *testing.T) {
	base := UpdateMetadata{
		Version:      LastUpdateMetadataVersion,
		Application:  "mailflow",
		Environment:  "production",
		UpdatedAt:    time.Now().UTC(),
		Branch:       "main",
		Upstream:     "origin/main",
		FromRevision: "from",
		ToRevision:   "to",
		Result:       "ready",
	}
	badResult := base
	badResult.Result = "unknown"
	if err := badResult.Validate(); err == nil {
		t.Fatal("expected unsupported result rejection")
	}
	badBackup := base
	badBackup.BackupPath = "/tmp/backup"
	if err := badBackup.Validate(); err == nil {
		t.Fatal("expected incomplete backup linkage rejection")
	}
}
