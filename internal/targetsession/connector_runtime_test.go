package targetsession

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// This black-box primitive qualification launches the actual separately built
// Connector and an actual engine. Its test CA/registry do not qualify production
// enrollment, OpenBao, persisted admission, Core ownership or a browser journey.
func TestConnectorRuntimeTransport(t *testing.T) {
	if os.Getenv("BASEHARBOR_CONNECTOR_RUNTIME_ACCEPTANCE") != "true" {
		t.Skip("explicit real Connector/runtime qualification")
	}
	engine := os.Getenv("BASEHARBOR_CONNECTOR_RUNTIME_KIND")
	image := os.Getenv("BASEHARBOR_CONNECTOR_RUNTIME_IMAGE")
	binary := os.Getenv("BASEHARBOR_TEST_CONNECTOR_BIN")
	if (engine != "docker" && engine != "podman") || !strings.Contains(image, "@sha256:") || !filepath.IsAbs(binary) {
		t.Fatal("real engine, immutable image and exact separately built Connector binary are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.CommandContext(ctx, engine, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("real runtime %s failed: %v\n%s", args[0], err, output)
		}
		return strings.TrimSpace(string(output))
	}
	if engine == "podman" && run("info", "--format", "{{.Host.Security.Rootless}}") != "true" {
		t.Fatal("Podman qualification must run rootless")
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "baha-connector-" + hex.EncodeToString(nonce[:])
	id := run("run", "--detach", "--name", name, image, "/bin/sh", "-c", "printf 'connector-runtime-ready\\n'; exec sleep 300")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if output, err := exec.CommandContext(cleanup, engine, "rm", "-f", id).CombinedOutput(); err != nil {
			t.Errorf("owned runtime fixture cleanup failed: %v\n%s", err, output)
		}
	})
	dir := t.TempDir()
	node := testNode()
	node.Runtime = engine
	r := &registry{scope: node.Scope()}
	issuer := serviceissuer.New(t)
	nodeURI, _ := url.Parse(node.Identity)
	coreURI, _ := url.Parse("spiffe://baseharbor/platform/core/transport-test")
	nodeCert, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: node.Identity, URIs: []*url.URL{nodeURI}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	coreCert, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: "core.test", DNSNames: []string{"core.test"}, URIs: []*url.URL{coreURI}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	trust, _ := issuer.TrustBundle(ctx)
	for file, data := range map[string][]byte{"node.crt": nodeCert.Certificate, "node.key": nodeCert.PrivateKey, "ca.pem": trust.PEM} {
		if err := os.WriteFile(filepath.Join(dir, file), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	pair, err := tls.X509KeyPair(coreCert.Certificate, coreCert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(trust.PEM)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pool := NewPool()
	finished := make(chan error, 1)
	go func() {
		finished <- pool.Serve(ctx, listener, &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, Certificates: []tls.Certificate{pair}}, r, coreURI.String())
	}()
	defer listener.Close()
	logFile, err := os.Create(filepath.Join(dir, "connector.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	command := exec.CommandContext(ctx, binary, "--runtime", engine, "--core", listener.Addr().String(), "--server-name", "core.test", "--core-identity", coreURI.String(), "--tenant-id", node.TenantID, "--target-id", node.TargetID, "--node-id", node.NodeID, "--state-root", filepath.Join(dir, "state"), "--quadlet-root", filepath.Join(dir, "quadlets"), "--cert", filepath.Join(dir, "node.crt"), "--key", filepath.Join(dir, "node.key"), "--ca", filepath.Join(dir, "ca.pem"))
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = command.Wait() }()
	until := time.Now().Add(20 * time.Second)
	for {
		if _, err := pool.LiveCapabilities(node.Scope()); err == nil {
			break
		}
		if time.Now().After(until) {
			data, _ := os.ReadFile(logFile.Name())
			t.Fatalf("actual outbound Connector was not admitted: %s", data)
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	dispatch := func(operation string, payload any) Response {
		t.Helper()
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		request := requestNow(hex.EncodeToString(nonce[:]) + "-" + strings.ReplaceAll(operation, ".", "-"))
		request.Operation = operation
		request.Payload = data
		request.DeadlineAt = time.Now().UTC().Add(30 * time.Second)
		response, err := pool.Dispatch(ctx, node.Scope(), request)
		if err != nil || !response.Success {
			t.Fatalf("real encrypted %s failed: %v, %v", operation, err, response.Error)
		}
		return response
	}
	response := dispatch("runtime.resource.list", struct{}{})
	var resources []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if json.Unmarshal(response.Result, &resources) != nil {
		t.Fatal("actual runtime inventory did not decode")
	}
	resourceID := ""
	for _, resource := range resources {
		if resource.Name == name {
			resourceID = resource.ID
		}
	}
	if resourceID == "" {
		t.Fatal("actual runtime fixture was absent from encrypted inventory")
	}
	for _, operation := range []string{"runtime.container.stop", "runtime.container.start", "runtime.container.restart"} {
		dispatch(operation, map[string]any{"resource_id": resourceID})
	}
	response = dispatch("runtime.exec", map[string]any{"resource_id": resourceID, "argv": []string{"printf", "encrypted-exec"}, "timeout_seconds": 10})
	var result struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	if json.Unmarshal(response.Result, &result) != nil || result.Stdout != "encrypted-exec" || result.ExitCode != 0 {
		t.Fatal("real typed exec result differed")
	}
	open := streamNow("terminal")
	open.ResourceID = resourceID
	open.DeadlineAt = time.Now().UTC().Add(20 * time.Second)
	terminal, err := pool.OpenStream(ctx, node.Scope(), open)
	if err != nil {
		t.Fatal("real remote terminal open", err)
	}
	defer terminal.Close()
	if err := terminal.Resize(40, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("printf 'encrypted-terminal\\n'; stty size; exit 7\n")); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(terminal)
	if err != nil || !strings.Contains(string(output), "encrypted-terminal") || !strings.Contains(string(output), "40 100") {
		t.Fatal("real terminal output/resize", string(output), err)
	}
	if code, err := terminal.Wait(ctx); err != nil || code != 7 {
		t.Fatal("real remote terminal exit", code, err)
	}
	logCtx, stopLogs := context.WithCancel(ctx)
	open = streamNow("logs")
	open.ResourceID = resourceID
	open.DeadlineAt = time.Now().UTC().Add(20 * time.Second)
	logs, err := pool.OpenStream(logCtx, node.Scope(), open)
	if err != nil {
		t.Fatal("real remote follow open", err)
	}
	defer logs.Close()
	marker := make([]byte, len("connector-runtime-ready\n"))
	if _, err := io.ReadFull(logs, marker); err != nil || string(marker) != "connector-runtime-ready\n" {
		t.Fatal("real follow output", string(marker), err)
	}
	stopLogs()
	wait, stopWait := context.WithTimeout(ctx, 2*time.Second)
	defer stopWait()
	if _, err := logs.Wait(wait); err == nil || wait.Err() != nil {
		t.Fatal("real quiet follow did not cancel promptly", err)
	}
	t.Log("Actual encrypted Connector inventory, lifecycle, exec, PTY input/resize/exit and follow cancellation passed. Test CA/registry; not production enrollment or release approval.")
}
