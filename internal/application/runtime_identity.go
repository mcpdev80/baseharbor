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

	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate application runtime identity token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	if err := writeOwnerOnlyFile(path, []byte(token+"\n")); err != nil {
		return "", fmt.Errorf("write application runtime identity token: %w", err)
	}
	return path, nil
}

func RuntimeIdentityTokenPath(files RuntimeFiles) string {
	return filepath.Join(files.Bindings, runtimeIdentityBinding, runtimeIdentityTokenFile)
}

func ownerOnlyRuntimeIdentity(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("application runtime identity token %s is accessible by group or others (%o)", path, info.Mode().Perm())
	}
	return nil
}
