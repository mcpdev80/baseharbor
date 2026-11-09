package onboardingbundle

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// PersistIdentity stages the verified workstation-owned leaf and key as
// owner-only files, then publishes the directory with one rename. It does not
// create or select a deployment Target; that is an independent authenticated
// Core connection step. Never log the returned path's contents.
func PersistIdentity(root string, b Bundle, identity EnrolledIdentity, now time.Time) (string, error) {
	if err := b.Validate(now); err != nil {
		return "", err
	}
	if err := validateEnrolledMaterial(b, identity, now); err != nil {
		return "", err
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

func validateEnrolledMaterial(b Bundle, identity EnrolledIdentity, now time.Time) error {
	if identity.InstallationID != b.InstallationID || identity.Station != b.Station ||
		identity.Serial == "" || !identity.ExpiresAt.After(now) ||
		len(identity.PrivateKeyPEM) == 0 || len(identity.CertificatePEM) == 0 ||
		!bytes.Equal(identity.TrustPEM, []byte(b.CAPEM)) {
		return fmt.Errorf("%w: unbound or incomplete workstation identity", ErrInvalid)
	}
	keyBlock, rest := pem.Decode(identity.PrivateKeyPEM)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return fmt.Errorf("%w: invalid private workstation key", ErrInvalid)
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("%w: invalid private workstation key", ErrInvalid)
	}
	leafBlock, rest := pem.Decode(identity.CertificatePEM)
	if leafBlock == nil || leafBlock.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return fmt.Errorf("%w: invalid workstation leaf", ErrInvalid)
	}
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil {
		return fmt.Errorf("%w: invalid workstation leaf", ErrInvalid)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(b.CAPEM)) {
		return fmt.Errorf("%w: missing workstation trust", ErrInvalid)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return fmt.Errorf("%w: invalid workstation trust chain", ErrInvalid)
	}
	scope := ws.Scope{TenantID: b.InstallationID, Station: b.Station}
	if len(leaf.URIs) != 1 || leaf.URIs[0].String() != scope.Identity() || leaf.IsCA {
		return fmt.Errorf("%w: foreign workstation identity", ErrInvalid)
	}
	keyPub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	certPub, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil || !bytes.Equal(keyPub, certPub) {
		return fmt.Errorf("%w: workstation private key does not match certificate", ErrInvalid)
	}
	serial, ok := new(big.Int).SetString(strings.ReplaceAll(strings.ToLower(identity.Serial), ":", ""), 16)
	if !ok || serial.Cmp(leaf.SerialNumber) != 0 ||
		leaf.NotAfter.Sub(identity.ExpiresAt) > time.Second ||
		identity.ExpiresAt.Sub(leaf.NotAfter) > time.Second {
		return fmt.Errorf("%w: workstation serial or expiry mismatch", ErrInvalid)
	}
	return nil
}
