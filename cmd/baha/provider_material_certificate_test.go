package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestProviderMaterialCertificateReportsActualExpiryWithoutSecretBytes(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Date(2027, 3, 12, 10, 20, 30, 0, time.UTC)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: expires.Add(-time.Hour), NotAfter: expires, DNSNames: []string{"identity.localhost"}, KeyUsage: x509.KeyUsageDigitalSignature}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, block   string
		bytes         []byte
		isCertificate bool
	}{
		{"server.pem", "CERTIFICATE", certificate, true},
		{"private-key.pem", "PRIVATE KEY", private, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			encoded := pem.EncodeToMemory(&pem.Block{Type: tc.block, Bytes: tc.bytes})
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			material := application.ProviderMaterial{Kind: "configuration", Evidence: "protected file"}
			if err := inspectProviderCertificateMaterial(path, info, &material); err != nil {
				t.Fatal(err)
			}
			if tc.isCertificate && (material.Kind != "certificate" || material.ValidFrom != template.NotBefore.Format(time.RFC3339) || material.ExpiresAt != expires.Format(time.RFC3339) || !strings.Contains(material.Evidence, "future issuance/renewal/revocation not verified")) {
				t.Fatalf("incorrect validity/continuity projection: %#v", material)
			}
			if !tc.isCertificate && (material.Kind != "key" || material.ExpiresAt != "" || material.ValidFrom != "") {
				t.Fatal("private key acquired invented validity")
			}
			raw, err := json.Marshal(material)
			if err != nil || strings.Contains(string(raw), "BEGIN ") || strings.Contains(string(raw), string(encoded)) {
				t.Fatal("material contents entered result")
			}
		})
	}
}

func TestProviderCertificateBundleUsesWholeWindowAndNoKeyContents(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	makeCertificate := func(from, until time.Time, serial int64) []byte {
		t.Helper()
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: from, NotAfter: until, KeyUsage: x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	secret := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
	bundle := append(secret, makeCertificate(start, start.Add(48*time.Hour), 1)...)
	bundle = append(bundle, makeCertificate(start.Add(time.Hour), start.Add(24*time.Hour), 2)...)
	material := application.ProviderMaterial{}
	if err := describeProviderCertificateWindow(bundle, &material); err != nil {
		t.Fatal(err)
	}
	if material.ValidFrom != start.Add(time.Hour).Format(time.RFC3339) || material.ExpiresAt != start.Add(24*time.Hour).Format(time.RFC3339) || !strings.Contains(material.Evidence, "certificates=2") {
		t.Fatalf("bundle window: %#v", material)
	}
	raw, err := json.Marshal(material)
	if err != nil || strings.Contains(string(raw), "BEGIN ") || strings.Contains(string(raw), string(secret)) {
		t.Fatal("key contents entered result", err)
	}
	disjoint := append(makeCertificate(start, start.Add(time.Hour), 3), makeCertificate(start.Add(2*time.Hour), start.Add(3*time.Hour), 4)...)
	if err := describeProviderCertificateWindow(disjoint, &application.ProviderMaterial{}); err == nil {
		t.Fatal("disjoint certificate windows accepted")
	}
}

func TestProviderMaterialCertificateRejectsChangedOrUnprotectedOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "certificate.pem")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	material := application.ProviderMaterial{}
	if err := inspectProviderCertificateMaterial(path, info, &material); err == nil {
		t.Fatal("changed file owner accepted")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	info, err = os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := inspectProviderCertificateMaterial(path, info, &material); err == nil {
		t.Fatal("unprotected material accepted")
	}
}
