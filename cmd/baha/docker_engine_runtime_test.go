package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	dockerprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/docker"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDualDockerEngineRootlessCoreAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_DUAL_DOCKER_ACCEPTANCE") != "1" {
		t.Skip("isolated concurrent rootless and system Docker acceptance is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	// Reproduce the user's active rootless context without DOCKER_* overrides.
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	rootless, err := bhruntime.ResolveDockerEngine(ctx, "docker")
	if err != nil || !rootless.Verified || rootless.Mode != "rootless" {
		t.Fatalf("rootless context unavailable: %+v %v", rootless, err)
	}
	system, err := bhruntime.ResolveDockerEngine(bhruntime.WithDockerEngineSelection(ctx, bhruntime.DockerEngineSelection{Endpoint: "unix:///var/run/docker.sock", Mode: "rootful"}), "docker")
	if err != nil || !system.Verified || system.DaemonID == rootless.DaemonID {
		t.Fatalf("independent system engine required: %+v %v", system, err)
	}
	systemRun := func(args ...string) string {
		t.Helper()
		data, err := exec.CommandContext(ctx, "docker", append([]string{"--host", system.Endpoint}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("isolated System Docker fixture operation %s failed", args[0])
		}
		return strings.TrimSpace(string(data))
	}
	// A same-Target volume is already enough to prohibit duplicate bootstrap.
	target := configureTestTarget(t)
	conflict := "baseharbor-863-conflict-" + strings.ToLower(time.Now().UTC().Format("150405"))
	systemRun("volume", "create", "--label", "com.docker.compose.project="+targetRuntimeProjectName(target), conflict)
	defer func() {
		exec.CommandContext(context.Background(), "docker", "--host", system.Endpoint, "volume", "rm", conflict).Run()
	}()
	provider := &dockerprovider.Provider{Compose: bhruntime.NewDockerCLIBackend("docker", rootless)}
	if err := validateTargetDockerBinding(ctx, target, provider, true); err == nil {
		t.Fatal("fresh rootless Target admitted existing System Docker owned resources")
	}
	systemRun("volume", "inspect", conflict)
	systemRun("volume", "rm", conflict)
	unavailable := bhruntime.WithDockerEngineSelection(ctx, bhruntime.DockerEngineSelection{Endpoint: "unix:///run/user/1000/baseharbor-863-missing.sock"})
	if engine, err := bhruntime.ResolveDockerEngine(unavailable, "docker"); err == nil || engine.Verified {
		t.Fatal("unreachable selected engine fell back")
	}
	// Unrelated system resources and persistent data must survive every native operation.
	foreign := "baseharbor-863-foreign-" + strings.ToLower(time.Now().UTC().Format("150405"))
	volume := foreign + "-data"
	systemRun("volume", "create", volume)
	defer func() {
		exec.CommandContext(context.Background(), "docker", "--host", system.Endpoint, "container", "rm", "-f", foreign).Run()
		exec.CommandContext(context.Background(), "docker", "--host", system.Endpoint, "volume", "rm", volume).Run()
	}()
	systemRun("run", "-d", "--name", foreign, "--volume", volume+":/data", "alpine:3.22", "sh", "-c", "printf system-owned-data > /data/marker; exec sleep 3600")
	systemRun("exec", foreign, "sh", "-c", "test -s /data/marker")
	// Compare immutable identities rather than human uptime text.
	identitySnapshot := func() string {
		return systemRun("container", "ls", "-a", "--format", "{{.ID}}|{{.Names}}") + "\n" + systemRun("volume", "ls", "--format", "{{.Name}}") + "\n" + systemRun("network", "ls", "--format", "{{.ID}}|{{.Name}}")
	}
	before := identitySnapshot()
	runCoreOnlyBootstrapRuntime(t, coreinstallation.Development)
	if after := identitySnapshot(); after != before {
		t.Fatal("Core lifecycle changed System Docker resource identities")
	}
	if systemRun("exec", foreign, "cat", "/data/marker") != "system-owned-data" {
		t.Fatal("System Docker persistent data changed")
	}
	owned, err := provider.ListOwnedProjectResources(ctx, targetRuntimeProjectName(target))
	if err != nil || len(owned) != 0 {
		t.Fatalf("rootless owned cleanup incomplete: %v %v", owned, err)
	}
	t.Logf("actual concurrent Docker engines verified: rootless endpoint=%s daemon=%s; system endpoint=%s daemon=%s; Core ready/idempotent, CLI/MCP parity, application lifecycle, no fallback/duplicate migration, owned cleanup, system data preserved", rootless.Endpoint, rootless.DaemonID, system.Endpoint, system.DaemonID)
}

func verifyDualDockerCLIAndMCP(t *testing.T, ctx context.Context, target deployment.ResolvedTarget) {
	t.Helper()
	rt, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	expected := bhruntime.DockerEngineForProvider(rt)
	if expected == nil || expected.Mode != "rootless" {
		t.Fatal("Core was not bound to rootless engine")
	}
	// CLI and a long-running MCP client must keep the protected original binding.
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	t.Setenv("DOCKER_CONTEXT", "default")
	assertEngine := func(data []byte) {
		t.Helper()
		var result struct {
			DockerEngine *bhruntime.DockerEngineObservation `json:"docker_engine"`
		}
		if json.Unmarshal(data, &result) != nil || result.DockerEngine == nil || !result.DockerEngine.Verified || result.DockerEngine.Endpoint != expected.Endpoint || result.DockerEngine.DaemonID != expected.DaemonID || result.DockerEngine.Mode != "rootless" {
			t.Fatal("public read result did not expose the actual bound rootless engine")
		}
	}
	for _, args := range [][]string{{"--target", target.Name, "target", "show", "--json"}, {"--target", target.Name, "status", "--json"}, {"--target", target.Name, "doctor", "--json"}} {
		var out, stderr bytes.Buffer
		if err := runWithIO(ctx, args, &out, &stderr); err != nil {
			t.Fatalf("native read %v failed: %v", args, err)
		}
		assertEngine(out.Bytes())
	}
	server := newMCPServer(application.DefaultStore())
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "dual-engine-rootless-qualification", Version: "v1"}, nil)
	session, err := client.Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, tool := range []string{"baseharbor.control-plane.status", "baseharbor.control-plane.doctor", "baseharbor.control-plane.up"} {
		result := callMCPAcceptanceTool(t, ctx, session, tool, map[string]any{"target": target.Name})
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		assertEngine(data)
	}
	runManagedProviderOnlyReadinessRegression(t, ctx)
}
