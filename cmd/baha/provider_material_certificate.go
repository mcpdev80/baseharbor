package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Inspect only bounded protected material. Public certificate validity is a
// material fact; it proves neither ongoing endpoint access nor future issuance,
// renewal or revocation. Secret bytes never enter the projected result.
func inspectProviderCertificateMaterial(path string, expected os.FileInfo, material *application.ProviderMaterial) error {
	if expected.Size() > 1<<20 {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(expected, actual) || !actual.Mode().IsRegular() || !bhruntime.ProtectedStateOwner(path, actual) {
		return errors.New("provider material owner changed during inspection")
	}
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, current) {
		return errors.New("provider material path changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	defer zeroBytes(data)
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("provider material exceeds bounded inspection")
	}
	current, err = os.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(expected, current) || !bhruntime.ProtectedStateOwner(path, current) {
		return errors.New("provider material owner changed during inspection")
	}
	return describeProviderCertificateWindow(data, material)
}

func describeProviderCertificateWindow(data []byte, material *application.ProviderMaterial) error {
	count, key := 0, false
	var starts, expires time.Time
	digest := sha256.New()
	for remaining := data; len(remaining) != 0; {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest
		if block.Type != "CERTIFICATE" {
			key = key || block.Type == "PRIVATE KEY" || block.Type == "RSA PRIVATE KEY" || block.Type == "EC PRIVATE KEY" || block.Type == "ENCRYPTED PRIVATE KEY"
			zeroBytes(block.Bytes)
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !certificate.NotAfter.After(certificate.NotBefore) {
			return errors.New("provider certificate material is invalid")
		}
		if count == 0 || certificate.NotBefore.After(starts) {
			starts = certificate.NotBefore
		}
		if count == 0 || certificate.NotAfter.Before(expires) {
			expires = certificate.NotAfter
		}
		digest.Write(certificate.Raw)
		count++
	}
	if count == 0 {
		if key {
			material.Kind = "key"
			material.Evidence = "protected key material; contents not exposed; usable delivery and renewal unverified"
		}
		return nil
	}
	if !expires.After(starts) {
		return errors.New("provider certificate bundle has no common validity window")
	}
	material.Kind = "certificate"
	material.ValidFrom = starts.UTC().Format(time.RFC3339)
	material.ExpiresAt = expires.UTC().Format(time.RFC3339)
	material.Evidence = fmt.Sprintf("protected public X.509 certificate bundle; certificates=%d not_before=%s not_after=%s sha256=%s; chain trust and revocation not verified; continued access and future issuance/renewal/revocation not verified", count, material.ValidFrom, material.ExpiresAt, hex.EncodeToString(digest.Sum(nil)))
	return nil
}
