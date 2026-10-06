package openbao

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const corePKIRoleCommand = `exec bao write baseharbor-pki/roles/baseharbor-core allow_any_name=true allow_localhost=false allow_ip_sans=false allowed_uri_sans="spiffe://baseharbor/platform/core/*" enforce_hostnames=false key_type=any key_usage=DigitalSignature client_flag=false server_flag=true use_csr_common_name=false use_csr_sans=true ttl=24h max_ttl=24h generate_lease=true`

const nodePKIRoleCommand = `exec bao write baseharbor-pki/roles/baseharbor-nodes allow_any_name=true allow_localhost=false allow_ip_sans=false allowed_uri_sans="spiffe://baseharbor/platform/connectors/*" enforce_hostnames=false key_type=any key_usage=DigitalSignature client_flag=true server_flag=false use_csr_common_name=false use_csr_sans=true ttl=24h max_ttl=24h generate_lease=true`

// SignCSR uses the signing endpoint: the client key never enters Core/OpenBao.
func (i *ServiceIssuer) SignCSR(ctx context.Context, request serviceaccess.CSRSigningRequest) (serviceaccess.IssuedCertificate, error) {
	return i.signScopedCSR(ctx, request, "connectors", "baseharbor-nodes", x509.ExtKeyUsageClientAuth)
}

// SignCoreCSR signs a Core-owned key with server-only usage. It is deliberately
// separate from the node enrollment issuer and cannot authorize node identities.
func (i *ServiceIssuer) SignCoreCSR(ctx context.Context, request serviceaccess.CSRSigningRequest) (serviceaccess.IssuedCertificate, error) {
	if !regexp.MustCompile(`^spiffe://baseharbor/platform/core/[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`).MatchString(request.Identity) {
		return serviceaccess.IssuedCertificate{}, errors.New("Core CSR requires one stable authority identity")
	}
	return i.signScopedCSR(ctx, request, "core", "baseharbor-core", x509.ExtKeyUsageServerAuth)
}

func (i *ServiceIssuer) signScopedCSR(ctx context.Context, request serviceaccess.CSRSigningRequest, namespace, role string, usage x509.ExtKeyUsage) (serviceaccess.IssuedCertificate, error) {
	if i == nil || i.executor == nil {
		return serviceaccess.IssuedCertificate{}, errors.New("OpenBao CSR issuer is not configured")
	}
	csr, err := request.Validate()
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	if !strings.HasPrefix(request.Identity, "spiffe://baseharbor/platform/"+namespace+"/") {
		return serviceaccess.IssuedCertificate{}, errors.New("node CSR is outside the managed connector identity namespace")
	}
	token, err := managerToken(ctx, i.executor, i.files)
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	payload, err := json.Marshal(map[string]any{"csr": string(request.CSRPEM), "common_name": request.Identity, "uri_sans": request.Identity, "ttl": request.TTL.String(), "format": "pem", "exclude_cn_from_sans": true})
	if err != nil {
		return serviceaccess.IssuedCertificate{}, errors.New("cannot encode node CSR")
	}
	out, err := execWithTokenPayload(ctx, i.executor, i.files, token, `exec bao write -format=json baseharbor-pki/sign/`+role+` -`, string(payload))
	if err != nil {
		return serviceaccess.IssuedCertificate{}, errors.New("managed authority could not sign the node CSR")
	}
	var reply struct {
		Data struct {
			Certificate string   `json:"certificate"`
			PrivateKey  string   `json:"private_key"`
			IssuingCA   string   `json:"issuing_ca"`
			CAChain     []string `json:"ca_chain"`
			Serial      string   `json:"serial_number"`
			Expiration  int64    `json:"expiration"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(out), &reply) != nil || reply.Data.PrivateKey != "" || reply.Data.IssuingCA == "" || reply.Data.Serial == "" {
		return serviceaccess.IssuedCertificate{}, errors.New("managed CSR signing returned invalid public material")
	}
	block, rest := pem.Decode([]byte(reply.Data.Certificate))
	if block == nil || block.Type != "CERTIFICATE" || strings.TrimSpace(string(rest)) != "" {
		return serviceaccess.IssuedCertificate{}, errors.New("managed CSR signing returned an invalid certificate")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.IsCA || leaf.Subject.CommonName != request.Identity || leaf.NotBefore.After(time.Now()) || !leaf.NotAfter.After(time.Now()) || leaf.NotAfter.After(time.Now().Add(request.TTL+time.Minute)) || len(leaf.URIs) != 1 || leaf.URIs[0].String() != request.Identity || len(leaf.DNSNames) != 0 || len(leaf.IPAddresses) != 0 || len(leaf.EmailAddresses) != 0 {
		return serviceaccess.IssuedCertificate{}, errors.New("signed node certificate differs from authorized scope")
	}
	publicKey, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil || string(publicKey) != string(csr.RawSubjectPublicKeyInfo) {
		return serviceaccess.IssuedCertificate{}, errors.New("signed certificate does not bind the submitted node key")
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != usage {
		return serviceaccess.IssuedCertificate{}, errors.New("signed node certificate has unapproved extended key usage")
	}
	chain := make([][]byte, 0, len(reply.Data.CAChain))
	for _, part := range reply.Data.CAChain {
		chain = append(chain, []byte(part))
	}
	return serviceaccess.IssuedCertificate{IssuerReference: serviceIssuerReference, Certificate: []byte(reply.Data.Certificate), IssuingCA: []byte(reply.Data.IssuingCA), CAChain: chain, Serial: reply.Data.Serial, ExpiresAt: leaf.NotAfter}, nil
}

var _ serviceaccess.CSRIssuer = (*ServiceIssuer)(nil)

var _ serviceaccess.CoreCSRIssuer = (*ServiceIssuer)(nil)
