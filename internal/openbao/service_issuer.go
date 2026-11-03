package openbao

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const serviceIssuerReference = "openbao://baseharbor-pki/baseharbor-services"

// ServiceIssuer adapts the managed OpenBao PKI engine to the provider-neutral
// service-access issuer contract.
type ServiceIssuer struct {
	executor Executor
	files    bhruntime.Files
}

func NewServiceIssuer(executor Executor, files bhruntime.Files) *ServiceIssuer {
	return &ServiceIssuer{executor: executor, files: files}
}

// CoreRuntimeFiles exposes the selected physical dependency, never its secrets.
func (i *ServiceIssuer) CoreRuntimeFiles() (bhruntime.Files, error) {
	if i == nil || i.executor == nil || i.files.Compose == "" || i.files.Env == "" || i.files.Project == "" {
		return bhruntime.Files{}, errors.New("shared Core dependency is not configured")
	}
	return i.files, nil
}

func (i *ServiceIssuer) TrustBundle(ctx context.Context) (serviceaccess.TrustBundle, error) {
	if i == nil || i.executor == nil {
		return serviceaccess.TrustBundle{}, errors.New("OpenBao service issuer is not configured")
	}
	ca, err := ServiceCA(ctx, i.executor, i.files)
	if err != nil {
		return serviceaccess.TrustBundle{}, err
	}
	return serviceaccess.TrustBundle{
		IssuerReference: serviceIssuerReference,
		PEM:             ca,
	}, nil
}

func (i *ServiceIssuer) Issue(ctx context.Context, request serviceaccess.CertificateRequest) (serviceaccess.IssuedCertificate, error) {
	if i == nil || i.executor == nil {
		return serviceaccess.IssuedCertificate{}, errors.New("OpenBao service issuer is not configured")
	}
	cert, err := IssueServiceCertificate(ctx, i.executor, i.files, ServiceCertificateRequest{
		CommonName:  request.CommonName,
		DNSNames:    request.DNSNames,
		IPAddresses: request.IPAddresses,
		URIs:        request.URIs,
		TTL:         request.TTL,
	})
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	return serviceaccess.IssuedCertificate{
		IssuerReference: serviceIssuerReference,
		Certificate:     cert.Certificate,
		PrivateKey:      cert.PrivateKey,
		IssuingCA:       cert.IssuingCA,
		CAChain:         cert.CAChain,
		Serial:          cert.Serial,
		ExpiresAt:       cert.ExpiresAt,
	}, nil
}

func (i *ServiceIssuer) Renew(ctx context.Context, _ serviceaccess.IssuedCertificate, request serviceaccess.CertificateRequest) (serviceaccess.IssuedCertificate, error) {
	// Renewal issues replacement material first. Revocation is deliberately a
	// separate operation so the caller can roll out and verify the replacement
	// before invalidating the previous certificate.
	return i.Issue(ctx, request)
}

func (i *ServiceIssuer) Revoke(ctx context.Context, serial string) error {
	if i == nil || i.executor == nil {
		return errors.New("OpenBao service issuer is not configured")
	}
	return RevokeServiceCertificate(ctx, i.executor, i.files, serial)
}

// Revoked is the coarse read-only revocation predicate for verification paths:
// it reports whether the observed serial is revoked by listing the PKI
// revocation output without writing. It reuses the read-only revocation-status
// lookup and never publishes a new revocation. A missing, foreign, unreadable
// or malformed revocation status fails closed by returning false.
func (i *ServiceIssuer) Revoked(ctx context.Context, serial string) bool {
	revoked, _ := i.RevokedStatus(ctx, serial)
	return revoked
}

// RevokedStatus is the tri-state read-only counterpart of
// RevokeServiceCertificate. It reports whether the observed serial is revoked by
// listing the PKI revocation output without writing. A missing, foreign,
// unreadable, malformed or empty revocation status reports err != nil, so the
// fail-closed production path rejects the connection instead of treating an
// unknown status as safe.
func (i *ServiceIssuer) RevokedStatus(ctx context.Context, serial string) (bool, error) {
	serial, err := canonicalServiceRevocationSerial(serial)
	if err != nil {
		return false, err
	}
	token, err := managerToken(ctx, i.executor, i.files)
	if err != nil {
		return false, err
	}
	const command = `exec bao read -format=json baseharbor-pki/list-revoked`
	out, execErr := execWithToken(ctx, i.executor, i.files, token, command)
	if execErr != nil {
		return false, execErr
	}
	return revokedSerialInResponse(out, serial)
}

func (i *ServiceIssuer) Status(ctx context.Context) (serviceaccess.IssuerStatus, error) {
	if _, err := i.TrustBundle(ctx); err != nil {
		return serviceaccess.IssuerStatus{
			IssuerReference: serviceIssuerReference,
			Ready:           false,
		}, err
	}
	return serviceaccess.IssuerStatus{
		IssuerReference: serviceIssuerReference,
		Ready:           true,
	}, nil
}

// BaseSerial is the read-only identity of the OpenBao service management
// connection: the real CA serial number (hex, no colon) that issued every
// service leaf. RevokedStatus compares against this on the CRL, so it must carry
// the genuine serial, not the mount URI.
func (i *ServiceIssuer) BaseSerial(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ca, err := ServiceCA(ctx, i.executor, i.files)
	if err != nil {
		return "", err
	}
	serial, err := caSerialHex(ca)
	if err != nil {
		return "", err
	}
	return serial, nil
}

// BaseExpiry is the read-only expiry bound of the OpenBao service management
// connection. It reads the genuine CA certificate NotAfter, so the production
// selection boundary sees the real certificate lifetime instead of an invented
// window.
func (i *ServiceIssuer) BaseExpiry(ctx context.Context) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	ca, err := ServiceCA(ctx, i.executor, i.files)
	if err != nil {
		return time.Time{}, err
	}
	return caNotAfter(ca)
}

var _ serviceaccess.Issuer = (*ServiceIssuer)(nil)

// caSerialHex parses the PEM service CA and returns its serial as lowercase hex
// without colons, matching the format RevokedStatus compares the CRL against.
func caSerialHex(caPEM []byte) (string, error) {
	block, rest := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 {
		return "", errors.New("OpenBao service CA is not a single CERTIFICATE block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse OpenBao service CA: %w", err)
	}
	return cert.SerialNumber.Text(16), nil
}

// caNotAfter parses the PEM service CA and returns its real NotAfter in UTC.
func caNotAfter(caPEM []byte) (time.Time, error) {
	block, rest := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 {
		return time.Time{}, errors.New("OpenBao service CA is not a single CERTIFICATE block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse OpenBao service CA: %w", err)
	}
	return cert.NotAfter.UTC(), nil
}
