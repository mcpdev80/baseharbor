package openbao

import (
	"context"
	"strings"
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

// revocationListFake returns a configurable JSON revocation-list response for
// the list-revoked command so the tri-state parse/validate behaviour can be
// exercised: a valid empty list, a list containing the canonical serial, and a
// malformed response.
type revocationListFake struct {
	servicePKIFake
	response string
}

func (f *revocationListFake) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "auth/approle/login"):
		return `{"auth":{"client_token":"manager-token"}}`, nil
	case strings.Contains(joined, "baseharbor-pki/list-revoked"):
		return f.response, nil
	default:
		return "", nil
	}
}

func TestRevokedStatusParsesAndValidatesRevocationList(t *testing.T) {
	files := servicePKITestFiles(t)

	// A valid, empty revocation list reports not revoked (no error): a readable
	// status, not an unreadable one.
	t.Run("valid empty list is not revoked", func(t *testing.T) {
		fake := &revocationListFake{response: `{"data":{"serials":[]}}`}
		issuer := NewServiceIssuer(fake, files)
		revoked, err := issuer.RevokedStatus(context.Background(), "01:02:03")
		if err != nil {
			t.Fatalf("valid empty list errored: %v", err)
		}
		if revoked {
			t.Fatal("empty list reported the serial as revoked")
		}
	})

	// The canonical serial is present: revoked.
	t.Run("canonical serial is revoked", func(t *testing.T) {
		fake := &revocationListFake{response: `{"data":{"serials":["01:02:03"]}}`}
		issuer := NewServiceIssuer(fake, files)
		revoked, err := issuer.RevokedStatus(context.Background(), "01:02:03")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if !revoked {
			t.Fatal("canonical serial was not reported as revoked")
		}
	})

	// A different serial is not revoked.
	t.Run("unrelated serial is not revoked", func(t *testing.T) {
		fake := &revocationListFake{response: `{"data":{"serials":["aa:bb:cc"]}}`}
		issuer := NewServiceIssuer(fake, files)
		revoked, err := issuer.RevokedStatus(context.Background(), "01:02:03")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if revoked {
			t.Fatal("unrelated serial reported as revoked")
		}
	})

	// A malformed response fails closed (err != nil), so the caller rejects
	// rather than treating an unreadable status as not-revoked.
	t.Run("malformed response fails closed", func(t *testing.T) {
		fake := &revocationListFake{response: `not-json at all`}
		issuer := NewServiceIssuer(fake, files)
		if _, err := issuer.RevokedStatus(context.Background(), "01:02:03"); err == nil {
			t.Fatal("malformed revocation status did not fail closed")
		}
	})

	// An empty/blank body fails closed: the provider did not publish a valid
	// parseable revocation list, so the connection is treated as revoked
	// rather than not-revoked (fail-open window).
	t.Run("empty body fails closed", func(t *testing.T) {
		fake := &revocationListFake{response: `   `}
		issuer := NewServiceIssuer(fake, files)
		revoked, err := issuer.RevokedStatus(context.Background(), "01:02:03")
		if err == nil && !revoked {
			t.Fatal("empty revocation body reported not-revoked (fail-open)")
		}
	})

	// A provably valid empty serials list is still not-revoked: a readable,
	// parseable status that reports no revocation.
	t.Run("valid empty list still not revoked", func(t *testing.T) {
		fake := &revocationListFake{response: `{"data":{"serials":[]}}`}
		issuer := NewServiceIssuer(fake, files)
		revoked, err := issuer.RevokedStatus(context.Background(), "01:02:03")
		if err != nil {
			t.Fatalf("valid empty list errored: %v", err)
		}
		if revoked {
			t.Fatal("valid empty list reported the serial as revoked")
		}
	})
}
