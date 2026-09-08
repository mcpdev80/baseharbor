package application

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeIdentityBinding = "baseharbor-runtime"
const runtimeIdentityTokenFile = "token"
const runtimeIdentityRevokedFile = "revoked"

func EnsureRuntimeIdentity(m Manifest, files RuntimeFiles) (string, error) {
	if !m.Services.Secrets {
		return "", nil
	}
	binding := filepath.Join(files.Bindings, runtimeIdentityBinding)
	if err := os.MkdirAll(binding, 0o700); err != nil {
		return "", fmt.Errorf("create application runtime identity binding: %w", err)
	}
	if err := os.Chmod(binding, 0o700); err != nil {
		return "", fmt.Errorf("secure application runtime identity binding: %w", err)
	}
	path := filepath.Join(binding, runtimeIdentityTokenFile)
	if data, err := os.ReadFile(path); err == nil {
		if err := ownerOnlyRuntimeIdentity(path); err != nil {
			return "", err
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", errors.New("application runtime identity token is empty")
		}
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read application runtime identity token: %w", err)
	}

	token, err := newRuntimeIdentityToken()
	if err != nil {
		return "", err
	}
	if err := writeOwnerOnlyFile(path, []byte(token+"\n")); err != nil {
		return "", fmt.Errorf("write application runtime identity token: %w", err)
	}
	return path, nil
}

func RotateRuntimeIdentity(m Manifest, files RuntimeFiles) error {
	path, err := EnsureRuntimeIdentity(m, files)
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("application does not enable managed secrets")
	}
	if err := ownerOnlyRuntimeIdentity(path); err != nil {
		return err
	}
	token, err := newRuntimeIdentityToken()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open application runtime identity token for rotation: %w", err)
	}
	_, writeErr := file.WriteString(token + "\n")
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("rotate application runtime identity token: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close rotated application runtime identity token: %w", closeErr)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure rotated application runtime identity token: %w", err)
	}
	if err := os.Remove(RuntimeIdentityRevokedPath(files)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear application runtime identity revocation: %w", err)
	}
	return nil
}

func RevokeRuntimeIdentity(m Manifest, files RuntimeFiles) error {
	path, err := EnsureRuntimeIdentity(m, files)
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("application does not enable managed secrets")
	}
	if err := ownerOnlyRuntimeIdentity(path); err != nil {
		return err
	}
	if err := writeOwnerOnlyFile(RuntimeIdentityRevokedPath(files), []byte("revoked\n")); err != nil {
		return fmt.Errorf("revoke application runtime identity: %w", err)
	}
	return nil
}

func RuntimeIdentityTokenPath(files RuntimeFiles) string {
	return filepath.Join(files.Bindings, runtimeIdentityBinding, runtimeIdentityTokenFile)
}

func RuntimeIdentityRevokedPath(files RuntimeFiles) string {
	return filepath.Join(files.Bindings, runtimeIdentityBinding, runtimeIdentityRevokedFile)
}

func RuntimeIdentityRevoked(files RuntimeFiles) bool {
	info, err := os.Stat(RuntimeIdentityRevokedPath(files))
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0
}

func newRuntimeIdentityToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate application runtime identity token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func ownerOnlyRuntimeIdentity(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("application runtime identity token %s is not an owner-only regular file (%o)", path, info.Mode().Perm())
	}
	return nil
}
