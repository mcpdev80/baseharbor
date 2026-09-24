package openbao

import (
	"context"
	"errors"

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

var _ serviceaccess.Issuer = (*ServiceIssuer)(nil)
