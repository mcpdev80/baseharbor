package openbao

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestRuntimeMTLSIdentityValidReusesMatchingIdentity(t *testing.T) {
	ctx := context.Background()
	issuer := serviceissuer.New(t)
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	identity := ApplicationIdentity{Name: "demo", Environment: "dev"}
	broker, client, workload := issueTestRuntimeIdentity(t, ctx, issuer, identity, []string{"bhm-test"})

	files := writeTestRuntimeIdentity(t, trust.PEM, broker, client, workload)
	valid, err := runtimeMTLSIdentityValid(files, trust.PEM, identity, []string{"bhm-test"})
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("matching runtime identity should be reusable")
	}

	valid, err = runtimeMTLSIdentityValid(files, trust.PEM, identity, []string{"bhm-other"})
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("runtime identity missing a required workload DNS alias must rotate")
	}

	valid, err = runtimeMTLSIdentityValid(files, trust.PEM, ApplicationIdentity{Name: "other", Environment: "dev"}, []string{"bhm-test"})
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("runtime identity for a different application must not be reused")
	}
}

func TestRuntimeMTLSIdentityValidRejectsDifferentCA(t *testing.T) {
	ctx := context.Background()
	issuer := serviceissuer.New(t)
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	identity := ApplicationIdentity{Name: "demo", Environment: "dev"}
	broker, client, workload := issueTestRuntimeIdentity(t, ctx, issuer, identity, []string{"bhm-test"})
	files := writeTestRuntimeIdentity(t, trust.PEM, broker, client, workload)

	otherTrust, err := serviceissuer.New(t).TrustBundle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := runtimeMTLSIdentityValid(files, otherTrust.PEM, identity, []string{"bhm-test"})
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("identity signed by another issuer must not be reused")
	}
}

func issueTestRuntimeIdentity(t *testing.T, ctx context.Context, issuer serviceaccess.Issuer, identity ApplicationIdentity, workloadDNS []string) (serviceaccess.IssuedCertificate, serviceaccess.IssuedCertificate, serviceaccess.IssuedCertificate) {
	t.Helper()
	broker, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName:  "baseharbor-runtime",
		DNSNames:    []string{"baseharbor-runtime", "baseharbor-secrets", "localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		TTL:         30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	identityURI, err := url.Parse("spiffe://baseharbor/apps/" + identity.Name + "/" + identity.Environment)
	if err != nil {
		t.Fatal(err)
	}
	client, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: "baseharbor-" + identity.Name + "-" + identity.Environment + "-client",
		URIs:       []*url.URL{identityURI},
		TTL:        30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	commonName := identity.Name + "." + identity.Environment + ".baseharbor"
	workload, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: commonName,
		DNSNames:   uniqueRuntimeDNSNames(append([]string{"localhost", identity.Name, commonName}, workloadDNS...)),
		TTL:        30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return broker, client, workload
}

func writeTestRuntimeIdentity(t *testing.T, trust []byte, broker, client, workload serviceaccess.IssuedCertificate) RuntimeMTLSFiles {
	t.Helper()
	dir := t.TempDir()
	files := RuntimeMTLSFiles{
		CA:           filepath.Join(dir, "ca.pem"),
		BrokerCert:   filepath.Join(dir, "broker-cert.pem"),
		BrokerKey:    filepath.Join(dir, "broker-key.pem"),
		ClientCert:   filepath.Join(dir, "client-cert.pem"),
		ClientKey:    filepath.Join(dir, "client-key.pem"),
		WorkloadCert: filepath.Join(dir, "workload-cert.pem"),
		WorkloadKey:  filepath.Join(dir, "workload-key.pem"),
	}
	values := map[string][]byte{
		files.CA:           trust,
		files.BrokerCert:   broker.Certificate,
		files.BrokerKey:    broker.PrivateKey,
		files.ClientCert:   client.Certificate,
		files.ClientKey:    client.PrivateKey,
		files.WorkloadCert: workload.Certificate,
		files.WorkloadKey:  workload.PrivateKey,
	}
	for path, value := range values {
		if err := os.WriteFile(path, value, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return files
}
