package main

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/mcpdev80/baseharbor/internal/deployment"
    "github.com/mcpdev80/baseharbor/internal/onboardingbundle"
    bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
    testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
)

// TestWorkstationOnboardingRemoteNativeInCI is deliberately a mandatory real
// native proof, not an HTTP mock. Its later cells must never be marked PASS by
// skipping a missing integration fixture. The CI proof reader also rejects a
// successful Go package exit when a cell was absent.
func TestWorkstationOnboardingRemoteNativeInCI(t *testing.T) {
    if os.Getenv("BASEHARBOR_ONBOARDING_NATIVE_ACCEPTANCE") != "1" {
        t.Skip("native workstation onboarding qualification is not requested")
    }
    kind := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
    if kind != "docker" && kind != "podman" {
        t.Fatalf("native onboarding requires Docker or Podman, got %q", kind)
    }
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
    defer cancel()
    ensureRuntimeIntegrationTrustPlane(t, ctx)
    name := runtimeIntegrationTrustPlaneTargetName
    if name == "" || !strings.HasPrefix(name, "w169-ci-bootstrap-") {
        t.Fatalf("isolated Core fixture unavailable: %q", name)
    }

    t.Run("CoreReady", func(t *testing.T) {
        provider, err := testruntime.Resolve(ctx)
        if err != nil {
            t.Fatal(err)
        }
        containers, err := provider.ListRuntimeContainers(ctx)
        if err != nil {
            t.Fatal(err)
        }
        for _, container := range containers {
            if container.Project == bhruntime.SharedProjectName(name) && container.Running {
                return
            }
        }
        t.Fatalf("no running owned Core services on native %s", kind)
    })

    var invitation string
    t.Run("Invitation", func(t *testing.T) {
        owned := filepath.Join(t.TempDir(), "private")
        if err := os.Mkdir(owned, 0o700); err != nil {
            t.Fatal(err)
        }
        invitation = filepath.Join(owned, "invite.json")
        var out bytes.Buffer
        err := runOnboardInvite(ctx, []string{
            "--target", name,
            "--station", "ci-workstation-native",
            "--output", invitation,
        }, &out)
        if err != nil {
            t.Fatalf("Core cannot issue an invitation without hidden manual endpoint setup: %v", err)
        }
        b, err := onboardingbundle.Read(invitation, time.Now())
        if err != nil {
            t.Fatalf("invalid native private invitation: %v", err)
        }
        if b.RuntimeProvider != kind || b.CoreURL == "" || b.CoreInstallationID == "" {
            t.Fatalf("invitation does not identify the native Core and runtime")
        }
    })

    // The following three cells are required but must remain RED until
    // genuinely implemented, not silently skipped or simulated with a local
    // fake HTTP server. This prevents promoting an untested new feature.
    t.Run("WorkstationImport", func(t *testing.T) {
        if invitation == "" {
            t.Fatal("native invitation is unavailable")
        }
        t.Fatal("missing E2E fixture: start an actual LAN-mTLS Core API and import on a separately isolated workstation")
    })
    t.Run("RemoteDeployment", func(t *testing.T) {
        t.Fatal("missing E2E fixture: verify plan, apply, status and source upload over the imported remote Core target")
    })
    t.Run("RemoteDestroy", func(t *testing.T) {
        t.Fatal("missing E2E fixture: verify remote teardown without destroying a foreign installation")
    })
}
