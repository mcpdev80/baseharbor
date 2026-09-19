package objectstorage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const providerAdminCredentialsFile = "runtime-admin.env"

type AdminCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

func EnsureAdminCredentials(files ProviderFiles) (AdminCredentials, string, error) {
	path := filepath.Join(files.Dir, providerAdminCredentialsFile)
	if credentials, err := LoadAdminCredentials(path); err == nil {
		return credentials, path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return AdminCredentials{}, "", err
	}

	access, err := randomHex(20)
	if err != nil {
		return AdminCredentials{}, "", fmt.Errorf("generate SeaweedFS runtime admin access key: %w", err)
	}
	secret, err := randomHex(32)
	if err != nil {
		return AdminCredentials{}, "", fmt.Errorf("generate SeaweedFS runtime admin secret key: %w", err)
	}
	credentials := AdminCredentials{
		AccessKeyID:     "BHADMIN" + strings.ToUpper(access),
		SecretAccessKey: secret,
	}
	data := []byte("ACCESS_KEY_ID=" + credentials.AccessKeyID + "\nSECRET_ACCESS_KEY=" + credentials.SecretAccessKey + "\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return AdminCredentials{}, "", fmt.Errorf("write SeaweedFS runtime admin credentials: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return AdminCredentials{}, "", fmt.Errorf("protect SeaweedFS runtime admin credentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return AdminCredentials{}, "", fmt.Errorf("install SeaweedFS runtime admin credentials: %w", err)
	}
	return credentials, path, nil
}

func LoadAdminCredentials(path string) (AdminCredentials, error) {
	return loadAdminCredentials(path, true)
}

func LoadContainerAdminCredentials(path string) (AdminCredentials, error) {
	return loadAdminCredentials(path, false)
}

func loadAdminCredentials(path string, ownerOnly bool) (AdminCredentials, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return AdminCredentials{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return AdminCredentials{}, errors.New("SeaweedFS runtime admin credentials must be a regular file")
	}
	if ownerOnly {
		if info.Mode().Perm()&0o077 != 0 {
			return AdminCredentials{}, fmt.Errorf("SeaweedFS runtime admin credentials are accessible by group or others (%o)", info.Mode().Perm())
		}
	} else if info.Mode().Perm()&0o022 != 0 {
		return AdminCredentials{}, fmt.Errorf("SeaweedFS runtime admin credentials are writable by group or others (%o)", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AdminCredentials{}, err
	}
	values, err := parseEnv(data)
	if err != nil {
		return AdminCredentials{}, errors.New("SeaweedFS runtime admin credentials are invalid")
	}
	credentials := AdminCredentials{
		AccessKeyID:     strings.TrimSpace(values["ACCESS_KEY_ID"]),
		SecretAccessKey: strings.TrimSpace(values["SECRET_ACCESS_KEY"]),
	}
	if credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
		return AdminCredentials{}, errors.New("SeaweedFS runtime admin credentials are incomplete")
	}
	return credentials, nil
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
