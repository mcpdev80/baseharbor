package openbao

import (
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
	workloadCert, workloadKey, err := issueRuntimeWorkloadCertificate(ca, caKey, identity)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	files := RuntimeMTLSFiles{}
	files.CA = filepath.Join(dir, "ca.pem")
	files.BrokerCert = filepath.Join(dir, "broker-cert.pem")
	files.BrokerKey = filepath.Join(dir, "broker-key.pem")
	files.ClientCert = filepath.Join(dir, "client-cert.pem")
	files.ClientKey = filepath.Join(dir, "client-key.pem")
	files.WorkloadCert = filepath.Join(dir, "workload-cert.pem")
	files.WorkloadKey = filepath.Join(dir, "workload-key.pem")
	values := map[string][]byte{}
	values[files.CA] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	values[files.BrokerCert] = brokerCert
	values[files.BrokerKey] = brokerKey
	values[files.ClientCert] = clientCert
	values[files.ClientKey] = clientKey
	values[files.WorkloadCert] = workloadCert
	values[files.WorkloadKey] = workloadKey
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
	workloadCert, workloadKey, err := issueRuntimeWorkloadCertificate(ca, caKey, identity)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := RuntimeMTLSFiles{}
	files.CA = filepath.Join(dir, "ca.pem")
	files.BrokerCert = filepath.Join(dir, "broker-cert.pem")
	files.BrokerKey = filepath.Join(dir, "broker-key.pem")
	files.ClientCert = filepath.Join(dir, "client-cert.pem")
	files.ClientKey = filepath.Join(dir, "client-key.pem")
	files.WorkloadCert = filepath.Join(dir, "workload-cert.pem")
	files.WorkloadKey = filepath.Join(dir, "workload-key.pem")
	values := map[string][]byte{}
	values[files.CA] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	values[files.BrokerCert] = brokerCert
	values[files.BrokerKey] = brokerKey
	values[files.ClientCert] = clientCert
	values[files.ClientKey] = clientKey
	values[files.WorkloadCert] = workloadCert
	values[files.WorkloadKey] = workloadKey
	for path, value := range values {
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
}
