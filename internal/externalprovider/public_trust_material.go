package externalprovider

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

// Public trust is a certificate-only delivery surface. Never copy a private
// key or arbitrary accompanying text through a CA/workload projection.
func ValidatePublicTrustMaterial(data []byte) error {
	remaining := bytes.TrimSpace(data)
	certificates := 0
	for len(remaining) > 0 {
		if !bytes.HasPrefix(remaining, []byte("-----BEGIN CERTIFICATE-----")) {
			return errors.New("external public trust requires certificate-only PEM material")
		}
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return errors.New("external public trust requires certificate-only PEM material")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return errors.New("external public trust contains an invalid certificate")
		}
		certificates++
		remaining = bytes.TrimSpace(rest)
	}
	if certificates == 0 {
		return errors.New("external public trust contains no certificates")
	}
	return nil
}
