package coreupdate

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func physicalRestoreFixture(t *testing.T, extra *tar.Header) StreamRecoveryPoint {
	t.Helper()
	point := StreamRecoveryPoint{Directory: filepath.Join(t.TempDir(), "backup"), Name: "pg"}
	if err := point.Capture(context.Background(), func(_ context.Context, w io.Writer) error {
		writer := tar.NewWriter(w)
		for name, data := range map[string]string{"PG_VERSION": "18\n", "backup_label": "label", "global/pg_control": "control", "base/5/123": "preserved SQL data", "pg_wal/000000010000000000000001": "WAL"} {
			if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
				return err
			}
			if _, err := io.WriteString(writer, data); err != nil {
				return err
			}
		}
		if extra != nil {
			if err := writer.WriteHeader(extra); err != nil {
				return err
			}
			if extra.Size > 0 {
				if _, err := io.CopyN(writer, strings.NewReader(strings.Repeat("x", int(extra.Size))), extra.Size); err != nil {
					return err
				}
			}
		}
		return writer.Close()
	}); err != nil {
		t.Fatal(err)
	}
	return point
}

func TestPhysicalPostgresRestorePreservesDataWALAndForeignDestination(t *testing.T) {
	point := physicalRestoreFixture(t, nil)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "restored")
	if err := RestorePostgresBasebackup(context.Background(), point, "18", destination); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPostgresBasebackupRestore(context.Background(), point, "18", destination); err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string]string{"base/5/123": "preserved SQL data", "pg_wal/000000010000000000000001": "WAL", "PG_VERSION": "18\n"} {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil || string(data) != expected {
			t.Fatalf("restore %s: %q %v", name, data, err)
		}
		st, err := os.Stat(filepath.Join(destination, name))
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatalf("restore permission %s: %v", name, err)
		}
	}
	if err := RestorePostgresBasebackup(context.Background(), point, "18", destination); err == nil {
		t.Fatal("existing physical data overwritten")
	}
	if err := os.WriteFile(filepath.Join(destination, "base/5/123"), []byte("changed SQL data!"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPostgresBasebackupRestore(context.Background(), point, "18", destination); err == nil {
		t.Fatal("changed restore reused")
	}
}

func TestPhysicalPostgresRestoreReplayRejectsExtraPartialAndSymlink(t *testing.T) {
	for _, mutation := range []string{"extra", "partial", "symlink", "public"} {
		t.Run(mutation, func(t *testing.T) {
			point := physicalRestoreFixture(t, nil)
			parent := t.TempDir()
			if err := os.Chmod(parent, 0700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(parent, "restore")
			if err := RestorePostgresBasebackup(context.Background(), point, "18", destination); err != nil {
				t.Fatal(err)
			}
			var err error
			switch mutation {
			case "extra":
				err = os.WriteFile(filepath.Join(destination, "foreign"), []byte("foreign"), 0600)
			case "partial":
				err = os.Remove(filepath.Join(destination, "global/pg_control"))
			case "symlink":
				if err = os.Remove(filepath.Join(destination, "PG_VERSION")); err == nil {
					err = os.Symlink(filepath.Join(destination, "backup_label"), filepath.Join(destination, "PG_VERSION"))
				}
			case "public":
				err = os.Chmod(filepath.Join(destination, "base"), 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := VerifyPostgresBasebackupRestore(context.Background(), point, "18", destination); err == nil {
				t.Fatal("unsafe restore reused")
			}
		})
	}
}

func TestPhysicalPostgresRestoreRejectsUnsafeArchiveBeforeCreatingDestination(t *testing.T) {
	for _, entry := range []*tar.Header{
		{Name: "../outside", Typeflag: tar.TypeReg},
		{Name: "/absolute", Typeflag: tar.TypeReg},
		{Name: "base/../../outside", Typeflag: tar.TypeReg},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"},
		{Name: "hardlink", Typeflag: tar.TypeLink, Linkname: "PG_VERSION"},
		{Name: "fifo", Typeflag: tar.TypeFifo},
		{Name: "setuid", Typeflag: tar.TypeReg, Mode: 04700},
		{Name: "PG_VERSION", Typeflag: tar.TypeReg},
	} {
		t.Run(entry.Name, func(t *testing.T) {
			point := physicalRestoreFixture(t, entry)
			parent := t.TempDir()
			if err := os.Chmod(parent, 0700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(parent, "restore")
			if err := RestorePostgresBasebackup(context.Background(), point, "18", destination); err == nil {
				t.Fatal("unsafe physical archive accepted")
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatal("unsafe archive created restore destination")
			}
		})
	}
}

func TestPhysicalPostgresRestoreRejectsChecksumMajorAndParent(t *testing.T) {
	point := physicalRestoreFixture(t, nil)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "restore")
	if err := RestorePostgresBasebackup(context.Background(), point, "17", destination); err == nil {
		t.Fatal("wrong major restored")
	}
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := RestorePostgresBasebackup(context.Background(), point, "18", destination); err == nil {
		t.Fatal("public destination parent accepted")
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	archive, _, err := point.paths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestorePostgresBasebackup(context.Background(), point, "18", destination); err == nil {
		t.Fatal("corrupt archive restored")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("failed preflight created restore destination")
	}
}
