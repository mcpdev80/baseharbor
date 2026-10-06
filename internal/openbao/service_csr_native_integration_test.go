package openbao

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// This opt-in test runs an isolated native OpenBao server with verified TLS,
// production Bootstrap and its real manager AppRole. It does not qualify the
// Connector transport or an application runtime journey.
func TestNativeOpenBaoManagedCoreAndNodeCSRRotation(t *testing.T) {
	if os.Getenv("BASEHARBOR_TEST_NATIVE_OPENBAO") != "1" {
		t.Skip("isolated native OpenBao qualification was not requested")
	}
	if _, err := exec.LookPath("bao"); err != nil {
		t.Fatal("requested native OpenBao binary is unavailable")
	}
	storageURL := os.Getenv("BASEHARBOR_TEST_OPENBAO_STORAGE_URL")
	storage, err := url.Parse(storageURL)
	if err != nil || storage.Scheme != "postgres" || storage.Host != "127.0.0.1:5432" || storage.Path != "/baseharbor_native_pki_fixture" || storage.RawQuery != "sslmode=disable" || storage.User == nil || storage.User.Username() != "baseharbor_fixture" || storage.Fragment != "" {
		t.Fatal("native qualification requires its isolated loopback PostgreSQL storage fixture")
	}
	addresses, err := net.LookupIP("openbao-member-1")
	if err != nil || len(addresses) == 0 {
		t.Fatal("isolated fixture hostname is unavailable")
	}
	for _, address := range addresses {
		if !address.IsLoopback() {
			t.Fatal("native fixture hostname must resolve only to loopback")
		}
	}
	root := t.TempDir()
	ca, certificate, key := nativeBaoListenerMaterial(t)
	for name, data := range map[string][]byte{"ca.pem": ca, "server.pem": certificate, "server.key": key} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configuration := fmt.Sprintf("api_addr = \"https://openbao-member-1:8200\"\nstorage \"postgresql\" {\n connection_url = %q\n ha_enabled = \"false\"\n}\nlistener \"tcp\" {\n address = \"127.0.0.1:8200\"\n tls_cert_file = %q\n tls_key_file = %q\n}\n", storageURL, filepath.Join(root, "server.pem"), filepath.Join(root, "server.key"))
	configPath := filepath.Join(root, "server.hcl")
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAO_ADDR", "https://openbao-member-1:8200")
	t.Setenv("BAO_CACERT", filepath.Join(root, "ca.pem"))
	t.Setenv("BAO_TOKEN", "")
	t.Setenv("BAO_SKIP_VERIFY", "false")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	serverCtx, stop := context.WithCancel(ctx)
	server := exec.CommandContext(serverCtx, "bao", "server", "-config="+configPath)
	var serverLog bytes.Buffer
	server.Stdout, server.Stderr = &serverLog, &serverLog
	if err := server.Start(); err != nil {
		t.Fatal("native OpenBao server could not start", err)
	}
	t.Cleanup(func() { stop(); _ = server.Wait() })
	executor := ExecutorFromCommand(nativeBaoCommandExecutor{})
	stateRoot := filepath.Join(root, "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{Compose: filepath.Join(stateRoot, "compose.yaml"), Env: filepath.Join(stateRoot, "runtime.env")}
	for deadline := time.Now().Add(15 * time.Second); ; {
		if _, err := Inspect(ctx, executor, files); err == nil {
			break
		}
		if time.Now().After(deadline) {
			stop()
			_ = server.Wait()
			t.Fatal("native OpenBao TLS listener did not become ready; process log bytes:", serverLog.Len())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := Bootstrap(ctx, executor, files, filepath.Join(root, "recovery.json")); err != nil {
		t.Fatal("production OpenBao bootstrap failed", err)
	}
	issuer := NewServiceIssuer(executor, files)
	before, err := issuer.TrustBundle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	coreRequest := nativeBaoCSR(t, "spiffe://baseharbor/platform/core/native-test", "core.test")
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "native-pki", NodeID: "node-a", Runtime: "docker"}
	nodeRequest := nativeBaoCSR(t, scope.Identity())
	authority, registry := nativeBaoEnrollmentAuthority(t, ctx, storageURL, issuer)
	oldCore, err := issuer.SignCoreCSR(ctx, coreRequest)
	if err != nil {
		t.Fatal("real managed Core CSR signing failed", err)
	}
	oldNode := nativeBaoEnroll(t, ctx, authority, scope, nodeRequest.CSRPEM, false)
	err = registry.AdmitCertificate(ctx, scope, oldNode.Serial, oldNode.ExpiresAt)
	if err != nil {
		t.Fatal("real managed node CSR signing failed", err)
	}
	verifyNativeBaoCertificate(t, oldCore, before.PEM, x509.ExtKeyUsageServerAuth)
	verifyNativeBaoCoreServerName(t, oldCore, "core.test")
	verifyNativeBaoCertificate(t, oldNode, before.PEM, x509.ExtKeyUsageClientAuth)
	if _, err := issuer.SignCoreCSR(ctx, nodeRequest); err == nil {
		t.Fatal("node identity crossed the Core issuer boundary")
	}
	if _, err := issuer.SignCSR(ctx, coreRequest); err == nil {
		t.Fatal("Core identity crossed the node enrollment issuer boundary")
	}
	if err := RotateServiceCA(ctx, executor, files); err != nil {
		t.Fatal("real managed CA rotation failed", err)
	}
	after, err := issuer.TrustBundle(ctx)
	if err != nil || bytes.Equal(before.PEM, after.PEM) {
		t.Fatal("managed CA rotation did not replace the active root", err)
	}
	newCore, err := issuer.SignCoreCSR(ctx, coreRequest)
	if err != nil {
		t.Fatal(err)
	}
	newNode := nativeBaoEnroll(t, ctx, authority, scope, nodeRequest.CSRPEM, true)
	err = registry.AdmitCertificate(ctx, scope, newNode.Serial, newNode.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	verifyNativeBaoCertificate(t, newCore, after.PEM, x509.ExtKeyUsageServerAuth)
	verifyNativeBaoCoreServerName(t, newCore, "core.test")
	verifyNativeBaoCertificate(t, newNode, after.PEM, x509.ExtKeyUsageClientAuth)
	if err := registry.AdmitCertificate(ctx, scope, oldNode.Serial, oldNode.ExpiresAt); err != nil {
		t.Fatal("persisted predecessor not admitted during bounded overlap", err)
	}
	other := scope
	other.NodeID = "foreign"
	if err := registry.AdmitCertificate(ctx, other, newNode.Serial, newNode.ExpiresAt); err == nil {
		t.Fatal("real issued certificate escaped persisted node scope")
	}
	for _, pair := range []struct {
		old, current serviceaccess.IssuedCertificate
	}{{oldCore, newCore}, {oldNode, newNode}} {
		oldBlock, _ := pem.Decode(pair.old.Certificate)
		newBlock, _ := pem.Decode(pair.current.Certificate)
		oldLeaf, _ := x509.ParseCertificate(oldBlock.Bytes)
		newLeaf, _ := x509.ParseCertificate(newBlock.Bytes)
		if !bytes.Equal(oldLeaf.RawSubjectPublicKeyInfo, newLeaf.RawSubjectPublicKeyInfo) || oldLeaf.SerialNumber.Cmp(newLeaf.SerialNumber) == 0 {
			t.Fatal("CSR renewal replaced the local key or reused the old serial")
		}
		overlap := append(append([]byte{}, before.PEM...), after.PEM...)
		verifyNativeBaoCertificate(t, pair.old, overlap, oldLeaf.ExtKeyUsage[0])
		verifyNativeBaoCertificate(t, pair.current, overlap, newLeaf.ExtKeyUsage[0])
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(after.PEM)
		if _, err := oldLeaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: oldLeaf.ExtKeyUsage}); err == nil {
			t.Fatal("old certificate survived retirement of its old CA")
		}
	}
	if err := registry.RevokeCertificate(ctx, scope, newNode.Serial); err != nil {
		t.Fatal("persisted certificate revocation failed", err)
	}
	for _, certificate := range []serviceaccess.IssuedCertificate{oldNode, newNode} {
		if err := registry.AdmitCertificate(ctx, scope, certificate.Serial, certificate.ExpiresAt); err == nil {
			t.Fatal("persisted revocation admitted old or current real certificate")
		}
	}
	if _, err := authority.CreateRenewal(ctx, scope, time.Minute, time.Hour); err == nil {
		t.Fatal("revoked real enrollment regained renewal authorization")
	}
	if err := issuer.Revoke(ctx, newNode.Serial); err != nil {
		t.Fatal("managed node revocation failed", err)
	}
	token, err := managerToken(ctx, executor, files)
	if err != nil {
		t.Fatal(err)
	}
	out, err := execWithToken(ctx, executor, files, token, "exec bao read -format=json baseharbor-pki/cert/crl")
	var reply struct {
		Data struct {
			Certificate string `json:"certificate"`
		} `json:"data"`
	}
	if err != nil || json.Unmarshal([]byte(out), &reply) != nil {
		t.Fatal("managed CRL could not be read", err)
	}
	block, _ := pem.Decode([]byte(reply.Data.Certificate))
	if block == nil {
		t.Fatal("managed CRL was not PEM")
	}
	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	leafBlock, _ := pem.Decode(newNode.Certificate)
	leaf, _ := x509.ParseCertificate(leafBlock.Bytes)
	found := false
	for _, entry := range crl.RevokedCertificateEntries {
		found = found || entry.SerialNumber.Cmp(leaf.SerialNumber) == 0
	}
	if !found {
		t.Fatal("revoked node serial is absent from the real managed CRL")
	}
}

type nativeBaoCommandExecutor struct{}

func (nativeBaoCommandExecutor) Exec(ctx context.Context, args ...string) (string, error) {
	return (nativeBaoCommandExecutor{}).ExecInput(ctx, nil, args...)
}

func (nativeBaoCommandExecutor) ExecInput(ctx context.Context, input []byte, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty native OpenBao command")
	}
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	// No user token helper, ambient manager token or TLS bypass is inherited.
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "BAO_ADDR=" + os.Getenv("BAO_ADDR"), "BAO_CACERT=" + os.Getenv("BAO_CACERT"), "BAO_SKIP_VERIFY=false", "BAO_TOKEN=isolated-fixture-not-a-credential"}
	command.Stdin = bytes.NewReader(input)
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return "", errors.New("native OpenBao command failed")
	}
	return output.String(), nil
}

func nativeBaoCSR(t *testing.T, identity string, dnsNames ...string) serviceaccess.CSRSigningRequest {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uri, err := url.Parse(identity)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: identity}, URIs: []*url.URL{uri}, DNSNames: dnsNames}, key)
	if err != nil {
		t.Fatal(err)
	}
	return serviceaccess.CSRSigningRequest{CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), Identity: identity, TTL: time.Hour, DNSNames: dnsNames}
}

func verifyNativeBaoCoreServerName(t *testing.T, issued serviceaccess.IssuedCertificate, name string) {
	t.Helper()
	block, _ := pem.Decode(issued.Certificate)
	if block == nil {
		t.Fatal("managed Core certificate is missing")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.VerifyHostname(name) != nil || leaf.VerifyHostname("foreign.test") == nil {
		t.Fatal("managed Core certificate does not bind its authorized TLS server name", err)
	}
}

func verifyNativeBaoCertificate(t *testing.T, issued serviceaccess.IssuedCertificate, trust []byte, usage x509.ExtKeyUsage) {
	t.Helper()
	block, _ := pem.Decode(issued.Certificate)
	if block == nil || len(issued.PrivateKey) != 0 {
		t.Fatal("issuer returned invalid or private material")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != usage {
		t.Fatal("issuer returned incorrect key usage", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trust) {
		t.Fatal("managed trust is invalid")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
		t.Fatal("real managed certificate does not validate", err)
	}
}

func nativeBaoListenerMaterial(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated native OpenBao listener"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	rootDER, err := x509.CreateCertificate(rand.Reader, template, template, public, key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "openbao-member-1"}, DNSNames: []string{"openbao-member-1"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: template.NotBefore, NotAfter: template.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, public, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}
