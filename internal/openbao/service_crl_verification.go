package openbao

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// revokedSerialInSignedCRL is the fail-closed fallback when OpenBao returns
// HTTP 404 for a fresh PKI's empty LIST endpoint. A missing list is never
// treated as proof of non-revocation: only a valid signed CRL can prove it.
func revokedSerialInSignedCRL(response, canonicalSerial string, ca *x509.Certificate, now time.Time) (bool, error) {
	if ca == nil || !ca.IsCA || now.IsZero() {
		return false, errors.New("OpenBao CRL requires a trusted CA and an explicit verification time")
	}
	var payload struct {
		Data struct {
			Certificate string `json:"certificate"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(response), &payload); err != nil || payload.Data.Certificate == "" {
		return false, errors.New("OpenBao CRL response is missing signed material")
	}
	block, rest := pem.Decode([]byte(payload.Data.Certificate))
	if block == nil || block.Type != "X509 CRL" || strings.TrimSpace(string(rest)) != "" {
		return false, errors.New("OpenBao CRL is not one PEM revocation list")
	}
	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("parse signed OpenBao CRL: %w", err)
	}
	if err := crl.CheckSignatureFrom(ca); err != nil {
		return false, fmt.Errorf("OpenBao CRL is not signed by the selected CA: %w", err)
	}
	if crl.ThisUpdate.After(now.Add(30*time.Second)) || !crl.NextUpdate.After(now) ||
		!crl.NextUpdate.After(crl.ThisUpdate) {
		return false, errors.New("OpenBao CRL is outside its valid publication window")
	}
	serial, err := canonicalServiceRevocationSerial(canonicalSerial)
	if err != nil {
		return false, err
	}
	number := new(big.Int)
	if _, ok := number.SetString(strings.ReplaceAll(serial, ":", ""), 16); !ok {
		return false, errors.New("invalid CRL serial lookup")
	}
	for _, revoked := range crl.RevokedCertificateEntries {
		if revoked.SerialNumber != nil && revoked.SerialNumber.Cmp(number) == 0 {
			return true, nil
		}
	}
	return false, nil
}
