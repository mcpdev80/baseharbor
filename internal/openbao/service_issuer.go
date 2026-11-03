package openbao

import (
	"context"
	"errors"
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
// connection: a stable mount-derived serial that issued every service leaf.
// It lets the production selection boundary observe the existing connection
// without issuing a new leaf.
func (i *ServiceIssuer) BaseSerial(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	_, err := i.TrustBundle(ctx)
	if err != nil {
		return "", err
	}
	return serviceIssuerReference, nil
}

// BaseExpiry is the read-only freshness bound of the OpenBao service
// management connection. It reuses the base issuer reference time (there is no
// separate base cert clock), so the production selection boundary has a stable
// freshness bound without issuing a fresh leaf.
func (i *ServiceIssuer) BaseExpiry(ctx context.Context) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	_, err := i.TrustBundle(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().UTC().Add(90 * 24 * time.Hour), nil
}

var _ serviceaccess.Issuer = (*ServiceIssuer)(nil)
