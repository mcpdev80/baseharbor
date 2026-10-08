package coreupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type snapshotRuntime struct {
	data     []byte
	restored []byte
	exports  int
	restores int
}

func (r *snapshotRuntime) ExportOwnedVolume(_ context.Context, _, _ string) ([]byte, error) {
	r.exports++
	return append([]byte(nil), r.data...), nil
}
func (r *snapshotRuntime) RestoreOwnedVolume(_ context.Context, _, _ string, data []byte) error {
	r.restores++
	r.restored = append([]byte(nil), data...)
	return nil
}

func TestVolumeRecoveryCaptureRestoreAndTamperRefusal(t *testing.T) {
	rt := &snapshotRuntime{data: []byte("pg-safe-backup")}
	delta := Delta{Installed: Realization{Kind: SQL, Installation: "core", Scope: "shared", Instance: "postgres-member-1", Owner: "baseharbor"}, Desired: Desired{Kind: SQL, Image: "postgres:18", Version: "18.2", Digest: digestA}}
	recovery := VolumeRecovery{Runtime: rt, Directory: filepath.Join(t.TempDir(), "artifacts"), Project: "owned-core", Volume: "postgres-data-1"}
	ctx := context.Background()
	if err := recovery.Capture(ctx, delta); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Capture(ctx, delta); err == nil {
		t.Fatal("overwrote original backup")
	}
	if rt.exports != 1 {
		t.Fatalf("unexpected second export: %d", rt.exports)
	}
	path, err := recovery.archivePath(delta)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe backup permissions %v", info.Mode())
	}
	if err := recovery.Recover(ctx, delta); err != nil {
		t.Fatal(err)
	}
	if string(rt.restored) != "pg-safe-backup" {
		t.Fatal("wrong restored bytes")
	}
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Recover(ctx, delta); err == nil {
		t.Fatal("tampered recovery accepted")
	}
	if rt.restores != 1 {
		t.Fatal("runtime volume touched after checksum mismatch")
	}
}
func TestVolumeRecoveryRejectsMissingOwnershipAndSymlinks(t *testing.T) {
	rt := &snapshotRuntime{data: []byte("openbao")}
	delta := Delta{Installed: Realization{Kind: Secrets, Installation: "core", Scope: "shared", Instance: "bao", Owner: "foreign"}, Desired: Desired{Kind: Secrets, Image: "openbao", Version: "2.7.0", Digest: digestA}}
	recovery := VolumeRecovery{Runtime: rt, Directory: t.TempDir(), Project: "owned", Volume: "bao"}
	if err := recovery.Capture(context.Background(), delta); err == nil {
		t.Fatal("foreign volume capture authorized")
	}
	delta.Installed.Owner = "baseharbor"
	if err := recovery.Capture(context.Background(), delta); err != nil {
		t.Fatal(err)
	}
	path, err := recovery.archivePath(delta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(destination, []byte("operator"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destination, path); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Recover(context.Background(), delta); err == nil {
		t.Fatal("symlink recovery accepted")
	}
	if rt.restores != 0 {
		t.Fatal("foreign path restored")
	}
}
func TestVolumeRecoveryRefusesFailedExport(t *testing.T) {
	rt := &snapshotRuntime{}
	delta := Delta{Installed: Realization{Kind: SQL, Installation: "core", Scope: "shared", Instance: "pg", Owner: "baseharbor"}, Desired: Desired{Kind: SQL, Image: "postgres", Version: "18", Digest: digestA}}
	r := VolumeRecovery{Runtime: rt, Directory: t.TempDir(), Project: "p", Volume: "v"}
	if err := r.Capture(context.Background(), delta); !errors.Is(err, errors.New("empty")) && err == nil {
		t.Fatal("empty snapshot accepted")
	}
}
