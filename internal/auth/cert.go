package auth

import (
	"crypto/x509"
	"encoding/pem"
)

// PEMLooksLikeCertificate reports whether data contains at least one PEM block
// of type CERTIFICATE. It never validates the certificate; it only guards
// against silently accepting a non-certificate file as a trust anchor.
func PEMLooksLikeCertificate(data []byte) bool {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return false
	}
	_, err := x509.ParseCertificate(block.Bytes)
	return err == nil
}
