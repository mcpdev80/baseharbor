package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOwnedCoreLANAddressOnlyPermitsRoutedPrivateIPv4(t *testing.T) {
	for _, tc := range []struct {
		address string
		valid bool
	}{
		{"192.168.6.178", true},
		{"10.10.10.5", true},
		{"172.16.2.3", true},
		{"172.31.255.254", true},
		{"127.0.0.1", false},
		{"0.0.0.0", false},
		{"169.254.1.2", false},
		{"8.8.8.8", false},
		{"224.1.2.3", false},
		{"2001:db8::1", false},
	} {
		if got := usableOwnedCoreIP(net.ParseIP(tc.address)); got != tc.valid {
			t.Fatalf("Core auto-discovery classified %s as %t, want %t", tc.address, got, tc.valid)
		}
	}
}

func TestManagedCoreLANEndpointRecordIsCertificateBoundAndPrivate(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	host := "192.168.6.178"
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(71),
		NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP(host)},
		KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(t.TempDir(), "server.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveOwnedCoreLANEndpoint("core", host+":18443", host, certPath); err != nil {
		t.Fatalf("CA-issued Core serving SAN must be published: %v", err)
	}
	got, err := loadOwnedCoreLANEndpoint("core")
	if err != nil || got != "https://"+host+":18443" {
		t.Fatalf("wrong or missing private Core HTTPS endpoint: %q %v", got, err)
	}
	path, err := ownedCoreLANEndpointPath("core")
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("Core endpoint record not owner-private: %v %v", fi, err)
	}
	if err := saveOwnedCoreLANEndpoint("core", host+":18443", "192.168.6.179", certPath); err == nil {
		t.Fatal("Core advertised address without matching SAN")
	}
	if err := saveOwnedCoreLANEndpoint("core", "0.0.0.0:18443", host, certPath); err == nil {
		t.Fatal("wildcard Core listener was published")
	}
}
