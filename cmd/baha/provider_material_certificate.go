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
	if !os.SameFile(expected, actual) || !actual.Mode().IsRegular() || actual.Mode().Perm()&0077 != 0 {
		return errors.New("provider material owner changed during inspection")
	}
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, current) {
		return errors.New("provider material path changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("provider material exceeds bounded inspection")
	}
	defer zeroBytes(data)
	block, _ := pem.Decode(data)
	if block == nil {
		return nil
	}
	if block.Type != "CERTIFICATE" {
		return nil
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return errors.New("provider certificate material is invalid")
	}
	digest := sha256.Sum256(certificate.Raw)
	material.Kind = "certificate"
	material.ExpiresAt = certificate.NotAfter.UTC().Format(time.RFC3339)
	material.Evidence = fmt.Sprintf("protected public X.509 certificate; not_before=%s not_after=%s sha256=%s; continued access and future issuance/renewal/revocation not verified", certificate.NotBefore.UTC().Format(time.RFC3339), material.ExpiresAt, hex.EncodeToString(digest[:]))
	return nil
}
