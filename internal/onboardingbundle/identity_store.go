package onboardingbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PersistIdentity stages the verified workstation-owned leaf and key as
// owner-only files, then publishes the directory with one rename. It does not
// create or select a deployment Target; that is an independent authenticated
// Core connection step. Never log the returned path's contents.
func PersistIdentity(root string, b Bundle, identity EnrolledIdentity, now time.Time) (string, error) {
	if err := b.Validate(now); err != nil {
		return "", err
	}
	if identity.InstallationID != b.InstallationID || identity.Station != b.Station ||
		identity.Serial == "" || !identity.ExpiresAt.After(now) ||
		len(identity.PrivateKeyPEM) == 0 || len(identity.CertificatePEM) == 0 ||
		len(identity.TrustPEM) == 0 {
		return "", fmt.Errorf("%w: unbound or incomplete workstation identity", ErrInvalid)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%w: workstation identity directory must be private", ErrInvalid)
	}
	final := filepath.Join(root, b.InstallationID+"-"+b.Station)
	if _, err := os.Lstat(final); err == nil {
		return "", fmt.Errorf("%w: workstation identity already exists", ErrInvalid)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	// An exclusive lock prevents cooperating CLI invocations from racing the
	// rename. The staging directory and key material never become a usable
	// identity until all writes and fsyncs have succeeded.
	lock := final + ".lock"
	lockFile, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if err := lockFile.Close(); err != nil {
		return "", err
	}
	defer os.Remove(lock)
	stage, err := os.MkdirTemp(root, ".workstation-stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0o700); err != nil {
		return "", err
	}
	records := []struct {
		name string
		data []byte
	}{
		{"private-key.pem", identity.PrivateKeyPEM},
		{"certificate.pem", identity.CertificatePEM},
		{"ca.pem", identity.TrustPEM},
	}
	for _, record := range records {
		path := filepath.Join(stage, record.name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return "", err
		}
		if _, err := file.Write(record.data); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
	}
	meta, err := json.Marshal(struct {
		Version        string    `json:"contract_version"`
		CoreURL        string    `json:"core_url"`
		InstallationID string    `json:"installation_id"`
		Station        string    `json:"station"`
		Serial         string    `json:"serial"`
		ExpiresAt      time.Time `json:"expires_at"`
	}{
		Version: Contract, CoreURL: b.CoreURL,
		InstallationID: b.InstallationID, Station: b.Station,
		Serial: identity.Serial, ExpiresAt: identity.ExpiresAt,
	})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, "identity.json"), append(meta, '\n'), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(stage, final); err != nil {
		return "", err
	}
	return final, nil
}
