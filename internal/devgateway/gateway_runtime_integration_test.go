package devgateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestGatewayRuntimeContinuityAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_GATEWAY_ACCEPTANCE") != "1" {
		t.Skip("development gateway acceptance requires BASEHARBOR_GATEWAY_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	const (
		target          = "gateway-acceptance"
		upstreamProject = "bh-gateway-acceptance-upstream"
		networkName     = "bh-gateway-acceptance-net"
		host            = "gateway-acceptance.baseharbor.localhost"
	)
	upstreamDir := t.TempDir()
	composePath := filepath.Join(upstreamDir, "compose.yaml")
	envPath := filepath.Join(upstreamDir, "runtime.env")
	compose := fmt.Sprintf(`services:
  upstream-a:
    image: docker.io/library/nginx:1.29.4-alpine
    restart: unless-stopped
    networks:
      gateway:
        aliases: ["gateway-upstream-a"]
  upstream-b:
    image: docker.io/library/nginx:1.29.4-alpine
    restart: unless-stopped
    networks:
      gateway:
        aliases: ["gateway-upstream-b"]

networks:
  gateway:
    name: %s
`, networkName)
	if err := os.WriteFile(composePath, []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigProject(ctx, upstreamProject, composePath, envPath); err != nil {
		t.Fatalf("validate gateway acceptance upstreams: %v", err)
	}
	if err := runtime.UpProject(ctx, upstreamProject, composePath, envPath); err != nil {
		t.Fatalf("start gateway acceptance upstreams: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = DestroyTarget(cleanupCtx, runtime, target)
		_ = runtime.DestroyProject(cleanupCtx, upstreamProject, composePath, envPath)
	}()

	issuer := serviceissuer.New(t)
	route := Route{
		Key:      "acceptance/upstream",
		Host:     host,
		Upstream: "http://gateway-upstream-a:80",
		Network:  networkName,
	}
	if err := ReplaceOwnerRoutes(ctx, runtime, issuer, target, "acceptance", []Route{route}); err != nil {
		t.Fatalf("start canonical gateway route: %v", err)
	}
	waitForGatewayHost(t, ctx, target, host, "initial canonical route")

	// Reconcile only the Caddy route configuration while preserving the same
	// canonical host, host port and attached network. The gateway runs Caddy
	// with --watch, so this exercises the non-disruptive config-reload path.
	route.Upstream = "http://gateway-upstream-b:80"
	if err := ReplaceOwnerRoutes(ctx, runtime, issuer, target, "acceptance", []Route{route}); err != nil {
		t.Fatalf("reconcile canonical gateway route: %v", err)
	}

	// Remove the old backend after the route switch. If the gateway retained
	// stale topology/config, the stable canonical endpoint would now fail.
	if err := runtime.StopProjectFilesSelected(ctx, upstreamProject, upstreamDir, map[string]string{}, []string{"upstream-a"}, composePath); err != nil {
		t.Fatalf("stop retired gateway upstream: %v", err)
	}
	waitForGatewayHost(t, ctx, target, host, "backend replacement")
}

func waitForGatewayHost(t *testing.T, ctx context.Context, target, host, phase string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		last = VerifyHosts(probeCtx, target, []string{host})
		cancel()
		if last == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s did not become reachable through stable gateway host: %v", phase, last)
}
