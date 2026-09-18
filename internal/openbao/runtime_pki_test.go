package openbao

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeMTLSIdentityValidReusesMatchingIdentity(t *testing.T) {
	caPEM, keyPEM, err := generateRuntimeCA()
	if err != nil {
		t.Fatal(err)
	}
	ca, caKey, err := parseRuntimeCA(caPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	identity := ApplicationIdentity{Name: "demo", Environment: "dev"}
	brokerCert, brokerKey, err := issueRuntimeCertificate(ca, caKey, identity, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := issueRuntimeCertificate(ca, caKey, identity, false)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	files := RuntimeMTLSFiles{
		CA:         filepath.Join(dir, "ca.pem"),
		BrokerCert: filepath.Join(dir, "broker-cert.pem"),
		BrokerKey:  filepath.Join(dir, "broker-key.pem"),
		ClientCert: filepath.Join(dir, "client-cert.pem"),
		ClientKey:  filepath.Join(dir, "client-key.pem"),
	}
	values := map[string][]byte{
		files.CA:         pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}),
		files.BrokerCert: brokerCert,
		files.BrokerKey:  brokerKey,
		files.ClientCert: clientCert,
		files.ClientKey:  clientKey,
	}
	for path, value := range values {
		if err := os.WriteFile(path, value, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	valid, err := runtimeMTLSIdentityValid(files, ca, identity)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("matching runtime identity should be reusable")
	}

	valid, err = runtimeMTLSIdentityValid(files, ca, ApplicationIdentity{Name: "other", Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("runtime identity for a different application must not be reused")
	}
}

func TestRuntimeMTLSIdentityValidRejectsDifferentCA(t *testing.T) {
	caPEM, keyPEM, err := generateRuntimeCA()
	if err != nil {
		t.Fatal(err)
	}
	ca, caKey, err := parseRuntimeCA(caPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	otherCAPEM, otherKeyPEM, err := generateRuntimeCA()
	if err != nil {
		t.Fatal(err)
	}
	otherCA, _, err := parseRuntimeCA(otherCAPEM, otherKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	identity := ApplicationIdentity{Name: "demo", Environment: "dev"}
	brokerCert, brokerKey, err := issueRuntimeCertificate(ca, caKey, identity, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := issueRuntimeCertificate(ca, caKey, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := RuntimeMTLSFiles{
		CA: filepath.Join(dir, "ca.pem"), BrokerCert: filepath.Join(dir, "broker-cert.pem"),
		BrokerKey: filepath.Join(dir, "broker-key.pem"), ClientCert: filepath.Join(dir, "client-cert.pem"),
		ClientKey: filepath.Join(dir, "client-key.pem"),
	}
	for path, value := range map[string][]byte{
		files.CA: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}),
		files.BrokerCert: brokerCert, files.BrokerKey: brokerKey,
		files.ClientCert: clientCert, files.ClientKey: clientKey,
	} {
		if err := os.WriteFile(path, value, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	valid, err := runtimeMTLSIdentityValid(files, otherCA, identity)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("identity signed by another runtime CA must not be reused")
	}

	_ = x509.ExtKeyUsageClientAuth
}
