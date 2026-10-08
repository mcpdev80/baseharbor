package coreupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamRecoveryPointCaptureVerifyResumeAndTamper(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p := StreamRecoveryPoint{Directory: filepath.Join(dir, "backups"), Name: "postgres-ha"}
	calls := 0
	source := func(_ context.Context, out io.Writer) error {
		calls++
		_, err := io.Copy(out, strings.NewReader(strings.Repeat("basebackup-WAL-", 8192)))
		return err
	}
	if err := p.Capture(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(); err != nil {
		t.Fatal(err)
	}
	if err := p.Capture(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("resumed backup source invoked %d times", calls)
	}
	path, _, err := p.paths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.Capture(context.Background(), source); err == nil {
		t.Fatal("tampered recovery accepted")
	}
	if calls != 1 {
		t.Fatal("tampered source overwritten")
	}
}

func TestStreamRecoveryPointRejectsUnsafeAndPartialEvidence(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../foreign", "/absolute", ".", ".hidden", "slash/name"} {
		if err := (StreamRecoveryPoint{Directory: dir, Name: name}).Capture(context.Background(), func(context.Context, io.Writer) error { return nil }); err == nil {
			t.Fatalf("unsafe name %q accepted", name)
		}
	}
	p := StreamRecoveryPoint{Directory: filepath.Join(dir, "backups"), Name: "pg"}
	if err := p.Capture(context.Background(), func(_ context.Context, w io.Writer) error {
		_, _ = w.Write([]byte("partial"))
		return errors.New("interrupted")
	}); err == nil {
		t.Fatal("failed backup published")
	}
	path, checksum, _ := p.paths()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial backup published: %v", err)
	}
	if err := os.MkdirAll(p.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checksum, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.Capture(context.Background(), func(_ context.Context, w io.Writer) error { _, e := w.Write([]byte("healthy")); return e }); err == nil {
		t.Fatal("unpaired checksum overwritten")
	}
	if err := os.Remove(checksum); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "foreign"), path); err != nil {
		t.Fatal(err)
	}
	if err := p.Capture(context.Background(), func(context.Context, io.Writer) error { t.Fatal("symlink target touched"); return nil }); err == nil {
		t.Fatal("symlink backup accepted")
	}
}
