package onboardingbundle

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

func testCoreServer(t *testing.T, tamper bool, mode ...string) (*httptest.Server, Bundle) {
	t.Helper()
	now := time.Now().UTC()
	longLived := len(mode) > 0 && mode[0] == "five-years"
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "onboard test CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	if longLived {
		caTemplate.NotAfter = now.Add(6 * 365 * 24 * time.Hour)
	}
	caDer, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDer)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDer})
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// httptest uses a loopback IP and TLS. Both the server certificate and
	// the workstation leaf must chain to the private test CA.
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDer, err := x509.CreateCertificate(rand.Reader, serverTemplate, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverCertKey, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	serverPair, err := tls.X509KeyPair(
		append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDer}), caPEM...),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverCertKey}))
	if err != nil {
		t.Fatal(err)
	}
	b := validBundle(t)
	b.CAPEM = string(caPEM)
	var lock sync.Mutex
	used := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(mode) > 0 && len(mode[0]) > len("redirect:") && mode[0][:len("redirect:")] == "redirect:" {
			w.Header().Set("Location", mode[0][len("redirect:"):])
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workstations/enroll" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var request struct {
			InstallationID string `json:"installation_id"`
			Station        string `json:"station"`
			Token          string `json:"token"`
			Nonce          string `json:"nonce"`
			CSRPEM         string `json:"csr_pem"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "bad", 400)
			return
		}
		lock.Lock()
		if used || request.Token != b.Token || request.Nonce != b.Nonce ||
			request.InstallationID != b.InstallationID || request.Station != b.Station {
			lock.Unlock()
			http.Error(w, "spent or foreign", 403)
			return
		}
		used = true
		lock.Unlock()
		p, _ := pem.Decode([]byte(request.CSRPEM))
		if p == nil {
			http.Error(w, "no CSR", 400)
			return
		}
		csr, err := x509.ParseCertificateRequest(p.Bytes)
		if err != nil || csr.CheckSignature() != nil {
			http.Error(w, "bad CSR", 400)
			return
		}
		uri, err := url.Parse("spiffe://baseharbor/workstations/" + b.InstallationID + "/" + b.Station)
		if err != nil {
			http.Error(w, "bad scope", 400)
			return
		}
		exp := now.Add(time.Hour).Truncate(time.Second)
		if longLived {
			exp = now.Add(5 * 365 * 24 * time.Hour).Truncate(time.Second)
		}
		serial := big.NewInt(33)
		if tamper {
			uri, _ = url.Parse("spiffe://baseharbor/workstations/" + b.InstallationID + "/foreign")
		}
		clientTemplate := &x509.Certificate{
			SerialNumber: serial, NotBefore: now.Add(-time.Minute), NotAfter: exp,
			URIs: []*url.URL{uri}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			KeyUsage: x509.KeyUsageDigitalSignature,
		}
		der, err := x509.CreateCertificate(rand.Reader, clientTemplate, ca, csr.PublicKey, caKey)
		if err != nil {
			http.Error(w, "could not sign", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(enrollReply{
			InstallationID: b.InstallationID, Station: b.Station, Serial: serial.Text(16),
			LeafPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
			TrustPEM: func() string {
				if len(mode) > 0 && mode[0] == "foreign-trust" {
					return "not the invitation CA"
				}
				return string(caPEM)
			}(), ExpiresAt: exp, At: now,
		})
	})
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{serverPair}}
	server.StartTLS()
	t.Cleanup(server.Close)
	b.CoreURL = server.URL
	return server, b
}

func TestRedeemOneUseCSRAndRejectReplay(t *testing.T) {
	_, b := testCoreServer(t, false)
	identity, err := Redeem(context.Background(), b, time.Now())
	if err != nil {
		t.Fatalf("one-use enrollment denied: %v", err)
	}
	if identity.InstallationID != b.InstallationID || identity.Station != b.Station || len(identity.PrivateKeyPEM) == 0 {
		t.Fatal("bound workstation identity is missing")
	}
	if _, err := Redeem(context.Background(), b, time.Now()); err == nil {
		t.Fatal("one-use invitation replay succeeded")
	}
}

func TestRedeemAcceptsFiveYearWorkstationCertificate(t *testing.T) {
	_, b := testCoreServer(t, false, "five-years")
	identity, err := Redeem(context.Background(), b, time.Now())
	if err != nil {
		t.Fatalf("five-year workstation enrollment denied: %v", err)
	}
	if identity.ExpiresAt.Before(time.Now().Add(5*365*24*time.Hour - time.Minute)) {
		t.Fatalf("five-year workstation certificate expires too early: %s", identity.ExpiresAt)
	}
}

func TestRedeemRejectsForeignSignedIdentity(t *testing.T) {
	_, b := testCoreServer(t, true)
	if _, err := Redeem(context.Background(), b, time.Now()); err == nil {
		t.Fatal("foreign station certificate was accepted")
	}
}

func TestRedeemRejectsIncorrectCA(t *testing.T) {
	_, b := testCoreServer(t, false)
	other := validBundle(t)
	b.CAPEM = other.CAPEM
	if _, err := Redeem(context.Background(), b, time.Now()); err == nil {
		t.Fatal("incorrect CA was accepted")
	}
}

func TestRedeemRejectsForeignReturnedTrust(t *testing.T) {
	_, bundle := testCoreServer(t, false, "foreign-trust")
	if _, err := Redeem(context.Background(), bundle, time.Now()); err == nil {
		t.Fatal("enrollment accepted Core trust different from pinned invitation")
	}
}

func TestRedeemNeverForwardsGrantOnRedirect(t *testing.T) {
	var forwarded bool
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()
	_, bundle := testCoreServer(t, false, "redirect:"+sink.URL)
	if _, err := Redeem(context.Background(), bundle, time.Now()); err == nil {
		t.Fatal("redirected enrollment was accepted")
	}
	if forwarded {
		t.Fatal("one-use enrollment grant was forwarded to redirect destination")
	}
}
