package openbao

import (
	"context"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestServiceIssuerImplementsProviderNeutralContract(t *testing.T) {
	files := servicePKITestFiles(t)
	fake := &servicePKIFake{}
	issuer := NewServiceIssuer(fake, files)

	status, err := issuer.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.IssuerReference != serviceIssuerReference {
		t.Fatalf("unexpected issuer status: %#v", status)
	}

	cert, err := issuer.Issue(context.Background(), serviceaccess.CertificateRequest{
		CommonName: "prometheus.baseharbor",
		DNSNames:   []string{"prometheus-access", "localhost"},
		TTL:        24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cert.IssuerReference != serviceIssuerReference || cert.Serial == "" || len(cert.PrivateKey) == 0 {
		t.Fatalf("unexpected issued certificate: %#v", cert)
	}
}

func TestConfigureServicePKIReusesExistingRoot(t *testing.T) {
	files := servicePKITestFiles(t)
	fake := &servicePKIFake{}

	if err := configureServicePKI(context.Background(), fake, files, "root-token"); err != nil {
		t.Fatal(err)
	}

	foundGuardedRoot := false
	for _, args := range fake.args {
		if containsAll(args, "baseharbor-pki/cert/ca", "root/generate/internal") {
			foundGuardedRoot = true
			if containsAll(args, "exec bao write", "root/generate/internal") {
				t.Fatalf("root generation must be guarded by an existing-CA check: %q", args)
			}
		}
	}
	if !foundGuardedRoot {
		t.Fatal("managed service PKI root reconciliation was not executed")
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !contains(value, part) {
			return false
		}
	}
	return true
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
