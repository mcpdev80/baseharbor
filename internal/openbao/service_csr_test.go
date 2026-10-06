package openbao

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type csrPKIFake struct {
	servicePKIFake
	reply string
}

func (f *csrPKIFake) ExecProjectInput(ctx context.Context, project, compose, env string, input []byte, service string, args ...string) (string, error) {
	if strings.Contains(strings.Join(args, " "), "baseharbor-pki/sign/baseharbor-nodes") {
		f.args = append(f.args, strings.Join(args, " "))
		f.inputs = append(f.inputs, string(input))
		return f.reply, nil
	}
	return f.servicePKIFake.ExecProjectInput(ctx, project, compose, env, input, service, args...)
}

func TestManagedNodeCSRSignsClientKeyAndReturnsNoPrivateMaterial(t *testing.T) {
	identity := "spiffe://baseharbor/platform/connectors/lab/node-a"
	uri, _ := url.Parse(identity)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: identity}, URIs: []*url.URL{uri}}, key)
	if err != nil {
		t.Fatal(err)
	}
	request := serviceaccess.CSRSigningRequest{Identity: identity, CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), TTL: time.Hour}
	ca := serviceissuer.New(t)
	leaf, err := ca.SignCSR(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{"certificate": string(leaf.Certificate), "issuing_ca": string(leaf.IssuingCA), "serial_number": leaf.Serial}
	response := func() string { raw, _ := json.Marshal(map[string]any{"data": data}); return string(raw) }
	fake := &csrPKIFake{reply: response()}
	issuer := NewServiceIssuer(fake, servicePKITestFiles(t))
	cert, err := issuer.SignCSR(context.Background(), request)
	if err != nil || len(cert.PrivateKey) != 0 || len(cert.Certificate) == 0 {
		t.Fatal("CSR signing lost key-locality", err)
	}
	for _, args := range fake.args {
		if strings.Contains(args, "manager-token") || strings.Contains(args, "CERTIFICATE REQUEST") {
			t.Fatal("CSR or auth token leaked to process arguments")
		}
	}
	if !strings.Contains(managerPolicy, `path "baseharbor-pki/sign/baseharbor-nodes"`) || strings.Contains(nodePKIRoleCommand, "server_flag=true") {
		t.Fatal("node signer role is not limited to client identity")
	}
	data["private_key"] = "PRIVATE KEY MUST NOT RETURN"
	fake.reply = response()
	if _, err := issuer.SignCSR(context.Background(), request); err == nil {
		t.Fatal("issuer returned client private key")
	}
	delete(data, "private_key")
	request.Identity += "foreign"
	if _, err := issuer.SignCSR(context.Background(), request); err == nil {
		t.Fatal("foreign node scope signed")
	}
}
