package runtimeexecutor

import (
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestRuntimeReadinessObserverIdentityAcceptedWithoutGrantingWorkloadIdentity(t *testing.T) {
	observer, err := url.Parse(openbao.RuntimeExecutorObserverSPIFFE)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{URIs: []*url.URL{observer}}
	if err := verifyRuntimeClientIdentity(tls.ConnectionState{PeerCertificates: []*x509.Certificate{certificate}}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := parseWorkloadURI(observer); ok {
		t.Fatal("observer gained an application workload identity")
	}
	other, _ := url.Parse("spiffe://foreign/observer")
	certificate.URIs = []*url.URL{other}
	if err := verifyRuntimeClientIdentity(tls.ConnectionState{PeerCertificates: []*x509.Certificate{certificate}}); err == nil {
		t.Fatal("unrecognized identity accepted")
	}
}
