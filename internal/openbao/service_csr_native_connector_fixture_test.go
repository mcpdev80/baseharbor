package openbao

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type nativeConnectorFixture struct {
	engine, image, binary, dir, name string
	sequence                         uint64
}

func newNativeConnectorFixture(t *testing.T, ctx context.Context) *nativeConnectorFixture {
	t.Helper()
	engine, image, binary := os.Getenv("BASEHARBOR_CONNECTOR_RUNTIME_KIND"), os.Getenv("BASEHARBOR_CONNECTOR_RUNTIME_IMAGE"), os.Getenv("BASEHARBOR_TEST_CONNECTOR_BIN")
	if (engine != "docker" && engine != "podman") || !strings.Contains(image, "@sha256:") || !filepath.IsAbs(binary) {
		t.Fatal("actual runtime, immutable image and absolute separately built Connector binary required")
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, engine, args...).CombinedOutput()
		if err != nil {
			t.Fatal("actual owned runtime fixture command failed", err)
		}
		return strings.TrimSpace(string(out))
	}
	if engine == "podman" && run("info", "--format", "{{.Host.Security.Rootless}}") != "true" {
		t.Fatal("native Podman qualification must run rootless")
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "baha-managed-enrollment-" + hex.EncodeToString(nonce[:])
	id := run("run", "--detach", "--name", name, "--label", "baseharbor.enrollment-qualification=true", "--user", "1000:1000", "--read-only", image, "sleep", "300")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := exec.CommandContext(cleanup, engine, "rm", "-f", id).Run(); err != nil {
			t.Error("owned native fixture cleanup failed", err)
		}
		output, err := exec.CommandContext(cleanup, engine, "ps", "-aq", "--filter", "id="+id).Output()
		if err != nil || strings.TrimSpace(string(output)) != "" {
			t.Error("owned native fixture remained after cleanup", err)
		}
	})
	return &nativeConnectorFixture{engine: engine, image: image, binary: binary, dir: t.TempDir(), name: name}
}

func (f *nativeConnectorFixture) write(t *testing.T, name string, data []byte) {
	t.Helper()
	// Publish complete protected material; a reconnect never reads a partial CA.
	tmp := filepath.Join(f.dir, name+".tmp")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(f.dir, name)); err != nil {
		t.Fatal(err)
	}
}

func (f *nativeConnectorFixture) read(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(f.dir, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatal("Connector identity material is not a protected regular file", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (f *nativeConnectorFixture) leaf(t *testing.T) *x509.Certificate {
	t.Helper()
	return nativeConnectorLeaf(t, f.read(t, "node.crt"))
}

func (f *nativeConnectorFixture) start(t *testing.T, ctx context.Context, scope targetenrollment.Scope, core, bootstrap, identity string, renew bool) func() {
	t.Helper()
	quadletRoot := filepath.Join(f.dir, "quadlets")
	if f.engine == "podman" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if !filepath.IsAbs(runtimeDir) {
			t.Fatal("rootless Podman requires its real user runtime directory")
		}
		quadletRoot = filepath.Join(runtimeDir, "containers", "systemd")
	}
	if err := os.MkdirAll(quadletRoot, 0700); err != nil {
		t.Fatal(err)
	}
	args := []string{"--runtime", f.engine, "--core", core, "--server-name", "core.test", "--core-identity", identity, "--tenant-id", scope.TenantID, "--target-id", scope.TargetID, "--node-id", scope.NodeID, "--sessions", "1", "--state-root", filepath.Join(f.dir, "state"), "--quadlet-root", quadletRoot, "--cert", filepath.Join(f.dir, "node.crt"), "--key", filepath.Join(f.dir, "node.key"), "--ca", filepath.Join(f.dir, "ca.pem"), "--bootstrap-url", bootstrap, "--bootstrap-ca", filepath.Join(f.dir, "ca.pem"), "--bootstrap-authorization-file", filepath.Join(f.dir, "authorization.json")}
	if renew {
		args = append(args, "--renew-certificate")
	}
	processCtx, cancel := context.WithCancel(ctx)
	log, err := os.OpenFile(filepath.Join(f.dir, "connector.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	command := exec.CommandContext(processCtx, f.binary, args...)
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		cancel()
		_ = log.Close()
		t.Fatal(err)
	}
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		_ = command.Wait()
		_ = log.Close()
	}
}

func (f *nativeConnectorFixture) dispatch(ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope, operation string, payload any) (targetsession.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return targetsession.Response{}, err
	}
	f.sequence++
	id := fmt.Sprintf("%s-%d", f.name, f.sequence)
	now := time.Now().UTC()
	request := targetsession.Request{ContractVersion: "baseharbor.target-access/v1", ProtocolVersion: "1", RequestID: id, CorrelationID: id, TargetID: scope.TargetID, Operation: operation, IssuedAt: now, DeadlineAt: now.Add(10 * time.Second), Payload: data}
	return pool.Dispatch(ctx, scope, request)
}

func (f *nativeConnectorFixture) inventory(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope) string {
	t.Helper()
	until := time.Now().Add(20 * time.Second)
	for time.Now().Before(until) && ctx.Err() == nil {
		// Only fresh read observations are polled after a reconnect, each with a new
		// request ID. A mutation is never replayed on another connection.
		response, err := f.dispatch(ctx, pool, scope, "runtime.resource.list", struct{}{})
		if err == nil && response.Success {
			var resources []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(response.Result, &resources) != nil {
				t.Fatal("actual Connector inventory was invalid")
			}
			for _, resource := range resources {
				if resource.Name == f.name {
					return resource.ID
				}
			}
			t.Fatal("actual owned fixture absent from Connector inventory")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Process logs remain private and may contain operational identifiers.
	info, _ := os.Stat(filepath.Join(f.dir, "connector.log"))
	var size int64
	if info != nil {
		size = info.Size()
	}
	t.Fatal("actual managed Connector failed admission/read; process log bytes", size)
	return ""
}

func (f *nativeConnectorFixture) exec(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope, id string) {
	t.Helper()
	response, err := f.dispatch(ctx, pool, scope, "runtime.exec", map[string]any{"resource_id": id, "argv": []string{"printf", "managed-encrypted-exec"}, "timeout_seconds": 10})
	var result struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	if err != nil || !response.Success || json.Unmarshal(response.Result, &result) != nil || result.Stdout != "managed-encrypted-exec" || result.ExitCode != 0 {
		t.Fatal("actual managed Connector bounded exec failed", err)
	}
}
