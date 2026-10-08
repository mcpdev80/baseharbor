package coreupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VolumeSnapshotRuntime provides the existing Docker/Podman owned-volume
// export/restore contract without allowing this package to shell out.
type VolumeSnapshotRuntime interface {
	ExportOwnedVolume(context.Context, string, string) ([]byte, error)
	RestoreOwnedVolume(context.Context, string, string, []byte) error
}

type VolumeRecovery struct {
	Runtime   VolumeSnapshotRuntime
	Directory string
	Project   string
	Volume    string
	// VerifyQuiesced proves no database writer can mutate this volume during an archive/restore.
	VerifyQuiesced func(context.Context, string, string) error
}

func (v VolumeRecovery) archivePath(delta Delta) (string, error) {
	if v.Directory == "" || v.Project == "" || v.Volume == "" {
		return "", errors.New("volume recovery requires directory/project/volume")
	}
	if delta.Installed.Owner != "baseharbor" || delta.Installed.Installation == "" || delta.Installed.Scope == "" || delta.Installed.Instance == "" || !validDigest(delta.Desired.Digest) {
		return "", errors.New("volume recovery refuses unowned or unpinned realization")
	}
	key := sha256.Sum256([]byte(JournalKey(delta) + "\x00" + v.Project + "\x00" + v.Volume))
	return filepath.Join(v.Directory, hex.EncodeToString(key[:])+".tar"), nil
}

// Capture persists an owner-only recovery point and its SHA-256 checksum.
// It deliberately rejects pre-existing recovery points rather than overwriting
// a backup from an earlier interrupted migration.
func (v VolumeRecovery) Capture(ctx context.Context, delta Delta) error {
	if v.Runtime == nil {
		return errors.New("no owned-volume runtime")
	}
	path, err := v.archivePath(delta)
	if err != nil {
		return err
	}
	if v.VerifyQuiesced == nil {
		return errors.New("provider volume recovery requires quiescence verification")
	}
	if err := v.VerifyQuiesced(ctx, v.Project, v.Volume); err != nil {
		return fmt.Errorf("provider volume is not quiesced: %w", err)
	}
	if err := os.MkdirAll(v.Directory, 0700); err != nil {
		return err
	}
	if err := os.Chmod(v.Directory, 0700); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("recovery point already exists for %s", delta.Installed.Instance)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := v.Runtime.ExportOwnedVolume(ctx, v.Project, v.Volume)
	if err != nil {
		return fmt.Errorf("export owned volume: %w", err)
	}
	if len(data) == 0 {
		return errors.New("empty provider recovery archive")
	}
	checksum := sha256.Sum256(data)
	tmp, err := os.CreateTemp(v.Directory, ".recovery-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	if err := os.WriteFile(path+".sha256", []byte(hex.EncodeToString(checksum[:])), 0600); err != nil {
		return err
	}
	directory, err := os.Open(v.Directory)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// Recover validates the immutable recovery archive before restoring the
// *owned* runtime volume; if verification fails it never touches a volume.
func (v VolumeRecovery) Recover(ctx context.Context, delta Delta) error {
	if v.Runtime == nil {
		return errors.New("no owned-volume runtime")
	}
	path, err := v.archivePath(delta)
	if err != nil {
		return err
	}
	for _, p := range []string{path, path + ".sha256"} {
		st, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("unsafe recovery artifact %s", p)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	expected, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return err
	}
	checksum := sha256.Sum256(data)
	if len(data) == 0 || strings.TrimSpace(string(expected)) != hex.EncodeToString(checksum[:]) {
		return errors.New("provider recovery archive checksum mismatch")
	}
	return v.Runtime.RestoreOwnedVolume(ctx, v.Project, v.Volume, data)
}
