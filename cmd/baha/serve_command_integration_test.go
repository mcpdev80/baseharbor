package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// The environment variables that drive the two-process serveCommand integration
// selectors. A child selects its role through serveMhIntegrationModeEnv (holder
// or challenger); the shared durable state root is passed through
// serveMhIntegrationRootEnv so both processes resolve the SAME root.
const (
	serveMhIntegrationModeEnv  = "BAHA_TEST_SERVE_MH_INTEG_MODE"
	serveMhIntegrationRootEnv  = "BAHA_TEST_SERVE_MH_INTEG_ROOT"
	serveMhIntegrationReadyEnv = "BAHA_TEST_SERVE_MH_INTEG_READY"
)

// TestServeCommandIntegrationOwnershipOnValidRoot is the act-1405c round-4 item 2
// regression: a genuine two-process serveCommand integration cell that proves the
// CODEX requirement directly.
//
// Process A runs the real serveCommand on a durable state root that already
// carries a genuine Core installation. It verifies ownership on that valid root
// (acquiring the active-writer lifecycle lock on the attach path), then binds
// the control-plane at listen address A and serves. Process B runs the same
// serveCommand against the SAME valid root at a DIFFERENT listen address B and
// must be refused specifically for ownership while A is still bound+serving. A
// demonstrably remains reachable/serving at address A.
//
// Distinct addresses guarantee the refusal is caused by A's ownership of the
// root and NOT by a port collision: B is refused because A owns the root, not
// because B could not bind.
func TestServeCommandIntegrationOwnershipOnValidRoot(t *testing.T) {
	xdir := filepath.Join(t.TempDir(), "xdg")
	if err := os.MkdirAll(filepath.Join(xdir, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(xdir, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(xdir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(xdir, "data"))

	// Configure a local installation target and seed a genuine Core installation
	// at its runtime root. serveCommand's attach path (valid root) will verify
	// ownership there; both processes share this XDG pair.
	target := configureTestTarget(t)
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatalf("resolve runtime state root: %v", err)
	}

	// Two distinct free listen addresses so a port collision cannot create a
	// false refusal.
	addrA := integFreeAddr(t)
	addrB := integFreeAddr(t)

	// Shared TLS files (self-signed for localhost); serveCommand needs them to
	// bind the control-plane.
	certFile, keyFile := integGenCert(t)
	ready := filepath.Join(t.TempDir(), "ready")

	// Process A: holder. Runs the real serveCommand on the valid root, verifies
	// ownership, binds+serving at address A, and signals readiness by writing
	// "BOUND A <addr>" to the ready marker.
	holder := exec.Command(os.Args[0], "-test.run=^TestServeCommandIntegrationHolder$")
	holder.Env = append(os.Environ(),
		serveMhIntegrationModeEnv+"=holder",
		serveMhIntegrationRootEnv+"="+root,
		serveMhIntegrationReadyEnv+"="+ready,
		"XDG_CONFIG_HOME="+filepath.Join(xdir, "config"),
		"XDG_DATA_HOME="+filepath.Join(xdir, "data"),
		"BASEHARBOR_API_LISTEN_ADDR="+addrA,
		"BASEHARBOR_API_TLS_CERT_FILE="+certFile,
		"BASEHARBOR_API_TLS_KEY_FILE="+keyFile,
		"BASEHARBOR_RUNTIME_APP_NAME=",
		"BASEHARBOR_RUNTIME_SECRETS_ENABLED=false",
	)
	holder.Stderr = os.Stderr
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Process.Kill() }()

	_, boundAddr := integWaitBoundReady(t, ready, holder)

	// A must be demonstrably bound and serving at its address.
	integAssertServing(t, boundAddr)

	// Process B: challenger. Runs the same serveCommand against the SAME valid
	// root at the DIFFERENT address B. While A holds ownership, B must be refused
	// for ownership.
	challenger := exec.Command(os.Args[0], "-test.run=^TestServeCommandIntegrationChallenger$")
	challenger.Env = append(os.Environ(),
		serveMhIntegrationModeEnv+"=challenger",
		serveMhIntegrationRootEnv+"="+root,
		"XDG_CONFIG_HOME="+filepath.Join(xdir, "config"),
		"XDG_DATA_HOME="+filepath.Join(xdir, "data"),
		"BASEHARBOR_API_LISTEN_ADDR="+addrB,
		"BASEHARBOR_API_TLS_CERT_FILE="+certFile,
		"BASEHARBOR_API_TLS_KEY_FILE="+keyFile,
		"BASEHARBOR_RUNTIME_APP_NAME=",
		"BASEHARBOR_RUNTIME_SECRETS_ENABLED=false",
	)
	out, cerr := challenger.CombinedOutput()
	if cerr == nil {
		t.Fatalf("challenger serveCommand was not refused while A owned the root; output:\n%s", out)
	}
	if !strings.Contains(string(out), "already owned") {
		t.Fatalf("challenger refused for the wrong reason; want an ownership refusal, got:\n%s", out)
	}

	// A must still be reachable/serving at address A after B was refused: the
	// holder's ownership is not disturbed by the challenger's failed attempt.
	integAssertServing(t, boundAddr)
}

// integWaitBoundReady blocks until the holder writes a "BOUND A <addr>" ready
// marker, proving it passed the ownership gate, bound, and is serving. A holder
// that exits before the marker never held ownership (test setup failure).
func integWaitBoundReady(t *testing.T, ready string, holder *exec.Cmd) (string, string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(ready); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= 3 && fields[0] == "BOUND" && fields[1] == "A" {
				return string(data), fields[2]
			}
		}
		if holder.ProcessState != nil {
			t.Fatalf("holder serveCommand exited (%v) before binding", holder.ProcessState.ExitCode())
		}
		time.Sleep(30 * time.Millisecond)
	}
	if holder.ProcessState != nil {
		t.Fatalf("holder serveCommand exited early (exit=%v)", holder.ProcessState.ExitCode())
	}
	t.Fatalf("holder did not report BOUND within timeout")
	return "", ""
}

// integAssertServing polls the /healthz endpoint at addr and requires HTTP 200,
// proving the holder is demonstrably bound+serving.
func integAssertServing(t *testing.T, addr string) {
	t.Helper()
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		Timeout:   2 * time.Second,
	}
	baseURL := "https://" + addr
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("holder not demonstrably bound+serving at %s", addr)
}

// integGenCert writes a self-signed TLS cert/key pair for localhost and returns
// the two file paths.
func integGenCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

// integFreeAddr reserves a free localhost address and returns it.
func integFreeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

// TestServeCommandIntegrationHolder is the holder child body selector: with no
// selector it is a no-op. When selected it runs the real serveCommand on the
// valid root, verifies ownership on the attach path (lifecycle lock), binds
// serving, and writes "BOUND A <addr>" to the ready marker before blocking on
// the parent's kill.
func TestServeCommandIntegrationHolder(t *testing.T) {
	if os.Getenv(serveMhIntegrationModeEnv) != "holder" {
		return
	}
	root := os.Getenv(serveMhIntegrationRootEnv)
	ready := os.Getenv(serveMhIntegrationReadyEnv)
	if root == "" || ready == "" {
		fmt.Fprintln(os.Stderr, "holder: missing integration env")
		os.Exit(2)
	}
	store := application.Store{Root: t.TempDir()}
	cmd := serveCommand(store)
	addr := os.Getenv("BASEHARBOR_API_LISTEN_ADDR")
	fmt.Fprintf(os.Stderr, "holder: root=%s addr=%s cert=%s\n", root, addr, os.Getenv("BASEHARBOR_API_TLS_CERT_FILE"))
	go func() {
		err := cmd.Run(context.Background(), nil, os.Stdout, os.Stderr)
		fmt.Fprintf(os.Stderr, "holder: serveCommand returned err=%v\n", err)
	}()
	// The serve listener binds at the address from BASEHARBOR_API_LISTEN_ADDR;
	// we read the concrete bound addr from that env (serveCommand binds exactly
	// this address) after confirming the listener is up via /healthz.
	integAssertServing(t, addr)
	_ = os.WriteFile(ready, []byte("BOUND A "+addr+"\n"), 0600)
	fmt.Fprintf(os.Stderr, "holder bound+serving at %s\n", addr)
	select {}
}

// TestServeCommandIntegrationChallenger is the challenger child body selector:
// with no selector it is a no-op. When selected it runs the same serveCommand
// on the same valid root and, while the holder owns the root, must be refused
// for ownership.
func TestServeCommandIntegrationChallenger(t *testing.T) {
	if os.Getenv(serveMhIntegrationModeEnv) != "challenger" {
		return
	}
	fmt.Fprintln(os.Stderr, "challenger: running real serveCommand on the shared valid root")
	// The real serveCommand verifies ownership on the valid root; while the
	// holder holds the lifecycle lock it must fail closed with an ownership
	// refusal ("already owned"). If it returns without refusing, the challenger
	// exits non-zero so the parent detects the missing refusal.
	err := serveCommand(application.Store{Root: t.TempDir()}).Run(context.Background(), nil, os.Stdout, os.Stderr)
	fmt.Fprintf(os.Stderr, "challenger: serveCommand returned err=%v\n", err)
	if err != nil {
		// A refusal is the desired outcome: signal it with exit code 9 so the
		// parent's CombinedOutput sees a non-zero status AND can confirm the
		// refusal message below.
		fmt.Fprintln(os.Stderr, "challenger: refused for ownership as required")
		os.Exit(9)
	}
	fmt.Fprintln(os.Stderr, "challenger: serveCommand returned WITHOUT an ownership refusal")
	os.Exit(1)
}
