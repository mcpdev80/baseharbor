package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
)

var (
	runtimeIntegrationTrustPlaneOnce sync.Once
	runtimeIntegrationTrustPlaneErr  error
)

func ensureRuntimeIntegrationTrustPlane(t *testing.T, ctx context.Context) {
	t.Helper()
	runtimeIntegrationTrustPlaneOnce.Do(func() {
		configHome, err := os.MkdirTemp("", "baseharbor-ci-config-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		dataHome, err := os.MkdirTemp("", "baseharbor-ci-data-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("XDG_CONFIG_HOME", configHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("XDG_DATA_HOME", dataHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Isolation is a CODE-GARANTIE, not an environmental precondition: the
		// bootstrap target below is a *fresh* configured target with its own
		// `<name>-default` native network and its own management Core, so this
		// single shared `up --control-plane-only` never gates on the ambient
		// `local` Core that a polluted shared daemon may carry. The ambient
		// `local` Core (if any) is never touched, reset or deleted. The four
		// lifecycle tests below each add their own distinct BASEHARBOR_TARGET
		// after this helper runs (one isolated Core per test), so a mutation
		// that drops a single test's target fails only that one test at the
		// gate — a per-test discriminating proof, not a systemic failure.
		//
		// The bootstrap target name is generated fresh per process so a
		// contaminated or pre-existing Core on a fixed name can never be the
		// isolation boundary: each run owns its own `<name>-default` network and
		// management Core. A per-process suffix keeps the cell reproducible only
		// by re-running, never by reusing another lane's fixed-name Core.
		bootstrapTarget := "w169-ci-bootstrap-" + bootstrapTargetSuffix()
		if err := os.Setenv("BASEHARBOR_TARGET", bootstrapTarget); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Always restore the selection, including when OpenBao was already
		// initialized or when bootstrap fails. Otherwise a later lifecycle
		// test silently inherits this bootstrap target.
		defer func() {
			if err := os.Unsetenv("BASEHARBOR_TARGET"); err != nil && runtimeIntegrationTrustPlaneErr == nil {
				runtimeIntegrationTrustPlaneErr = err
			}
			if runtimeIntegrationTrustPlaneErr == nil {
				runtimeIntegrationTrustPlaneErr = mutatePersistedTarget("", true)
			}
		}()
		// The shared rootless daemon is the integration endpoint. An active
		// CLI context (the prepare step's setuptool registers "rootless")
		// takes precedence over DOCKER_HOST inside the engine resolver, so
		// make the endpoint authoritative and neutralize the context.
		_ = os.Unsetenv("DOCKER_CONTEXT")

		var out bytes.Buffer
		if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
			if err := runWithIO(ctx, []string{"target", "create", bootstrapTarget, "--runtime-provider", "podman", "--access", "local", "--access-provider", "local", "--reference", "local", "--default"}, &out, &out); err != nil {
				runtimeIntegrationTrustPlaneErr = err
				return
			}
			out.Reset()
		} else {
			// Docker (the default test runtime) needs its own bootstrap target
			// configured before the single shared `up` below; this target has
			// its own `<name>-default` native network and management Core, so
			// the gate never sees the ambient `local` Core.
			if err := runWithIO(ctx, []string{"target", "create", bootstrapTarget, "--runtime-provider", "docker", "--access", "local", "--access-provider", "local", "--reference", "local", "--default"}, &out, &out); err != nil {
				runtimeIntegrationTrustPlaneErr = err
				return
			}
			out.Reset()
		}
		if err := runWithIO(ctx, []string{"up", "--control-plane-only", "--yes"}, &out, &out); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}

		// The shared `up --control-plane-only` just brought up the bootstrap
		// management Core, which wrote a genuine, READY installation.json at its
		// own runtime root. Record that installation as the loan's boot Core
		// HERE (before the OpenBao read-only path may return early) so the owner
		// loan can lend from it for every isolated test Target in this process —
		// the bootstrap target runs under a per-process random name that
		// effective-target selection cannot resolve as the boot Core.
		if bootState, bootErr := bootstrapCoreInstallation(ctx, bootstrapTarget); bootErr == nil {
			registerBootInstallation(bootRuntimeRootFor(bootstrapTarget), bootState)
		}

		compose, files, err := openBaoRuntime(ctx)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		state, err := platformopenbao.Inspect(ctx, compose, files)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if state.Initialized {
			if state.Sealed {
				runtimeIntegrationTrustPlaneErr = os.ErrPermission
				return
			}
			runtimeIntegrationTrustPlaneErr = platformopenbao.CheckManager(ctx, compose, files)
			return
		}

		dir, err := os.MkdirTemp("", "baseharbor-ci-openbao-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		recoveryPath := filepath.Join(dir, "recovery.json")
		out.Reset()
		runtimeIntegrationTrustPlaneErr = runWithIO(ctx, []string{"openbao", "bootstrap", "--recovery-file", recoveryPath}, &out, &out)
	})
	if runtimeIntegrationTrustPlaneErr != nil {
		t.Fatalf("prepare BaseHarbor integration trust plane: %v", runtimeIntegrationTrustPlaneErr)
	}
	if _, err := existingTargetRuntimeFiles(ctx); err != nil {
		t.Fatalf("BaseHarbor integration trust plane state missing: %v", err)
	}
	// A missing per-test isolation call must never silently fall back to
	// the bootstrap Core or a configured default target.
	sentinel := "w169-isolation-required-" + bootstrapTargetSuffix()
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load targets for isolation guard: %v", err)
	}
	if _, exists := cfg.Targets[sentinel]; exists {
		t.Fatalf("isolation guard target %q unexpectedly exists", sentinel)
	}
	t.Setenv("BASEHARBOR_TARGET", sentinel)
}

// isolateRuntimeForTest configures a fresh target for the requested runtime.
// Target and provider are verified through the real deployment resolver, not
// inferred from environment variables or constructed network names.
func isolateRuntimeForTest(t *testing.T, name string) {
	t.Helper()
	provider := integrationRuntimeProvider(t)
	isoTarget := "w169-" + name + "-" + bootstrapTargetSuffix() + "-iso"
	if err := os.Setenv("BASEHARBOR_TARGET", isoTarget); err != nil {
		t.Fatalf("set isolated target %q: %v", isoTarget, err)
	}
	var buf bytes.Buffer
	args := []string{"target", "create", isoTarget, "--runtime-provider", provider, "--access", "local", "--access-provider", "local", "--reference", "local"}
	if err := runWithIO(t.Context(), args, &buf, &buf); err != nil {
		t.Fatalf("configure isolated %s target %q: %v\\n%s", provider, isoTarget, err, buf.String())
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load actual target configuration: %v", err)
	}
	selected, err := selectedTargetName("", os.Getenv("BASEHARBOR_TARGET"), cfg)
	if err != nil {
		t.Fatalf("resolve active target: %v", err)
	}
	resolved, err := cfg.ResolveTarget(selected, "")
	if err != nil {
		t.Fatalf("resolve configured runtime target: %v", err)
	}
	if resolved.Name != isoTarget || resolved.RuntimeProvider != provider || resolved.AccessProvider != "local" {
		t.Fatalf("runtime isolation mismatch: want target=%q provider=%q access=local, got %+v", isoTarget, provider, resolved)
	}
	// TEST-SCOPE owner loan marker (card t-20261106-r27-remote-up-podman-owner-missing,
	// lane w286): this isolated test Target has no installation of its own by
	// design — the boot Target's genuine Core installation is the single
	// durable owner. The marker file documents the isolated test Target; the
	// production owner loan resolves this namespace against the boot Core by
	// its genuine, READY installation, not by the marker alone.
	writeBahaTestIsolationMarker(t, isoTarget)
	t.Logf("isolated runtime resolved from deployment config: target=%q runtime=%q endpoint=%q context=%q", resolved.Name, resolved.RuntimeProvider, resolved.DockerEndpoint, resolved.DockerContext)
}

func integrationRuntimeProvider(t *testing.T) string {
	t.Helper()
	provider := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if provider == "" {
		return "docker"
	}
	if provider != "docker" && provider != "podman" {
		t.Fatalf("unsupported integration runtime provider %q", provider)
	}
	return provider
}

// bootRuntimeRootFor resolves the runtime root of the named bootstrap target
// (targets/<name>/runtime) via the deployment state root.
func bootRuntimeRootFor(targetName string) string {
	root, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		return ""
	}
	return filepath.Join(root, "runtime")
}

// bootstrapCoreInstallation loads the genuine, READY Core installation the
// bootstrap selector just brought up at the bootstrap target's runtime root.
// It is a test-scoped probe of the durable installation.json the production
// `up` path wrote; it never creates, copies or rewrites that state, so the
// one-installation-owner rule (ADR 0021) stays intact. A load failure is
// ignored: without a recorded boot Core the owner loan keeps its fail-closed
// ErrDeploymentOwnerMissing, exactly as before this follow-on.
func bootstrapCoreInstallation(ctx context.Context, targetName string) (coreinstallation.State, error) {
	root := bootRuntimeRootFor(targetName)
	if root == "" {
		return coreinstallation.State{}, errors.New("cannot resolve bootstrap target runtime root")
	}
	state, err := coreinstallation.Load(root)
	return state, err
}

// bootstrapTargetSuffix returns a short random hex suffix so the shared trust
// plane helper configures a bootstrap target that cannot collide with a
// contaminated or pre-existing Core on a fixed name; each process gets its own
// `w169-ci-bootstrap-<suffix>` isolated Core.
func bootstrapTargetSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000"
	}
	return hex.EncodeToString(b[:])
}

func TestIntegrationRuntimeProviderSelection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime string
		want    string
	}{
		{name: "default-docker", runtime: "", want: "docker"},
		{name: "explicit-docker", runtime: "docker", want: "docker"},
		{name: "explicit-podman", runtime: "podman", want: "podman"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_TEST_RUNTIME", tc.runtime)
			if got := integrationRuntimeProvider(t); got != tc.want {
				t.Fatalf("runtime provider selection: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMissingRuntimeIsolationCannotFallBackToBootstrap(t *testing.T) {
	const sentinel = "w169-isolation-required-regression"
	t.Setenv("BASEHARBOR_TARGET", sentinel)
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "w169-ci-bootstrap",
		Targets: map[string]deployment.TargetDefinition{
			"w169-ci-bootstrap": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}},
		},
	}
	selected, err := selectedTargetName("", os.Getenv("BASEHARBOR_TARGET"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if selected != sentinel {
		t.Fatalf("missing isolation selected %q instead of guard %q", selected, sentinel)
	}
	if _, err := cfg.ResolveTarget(selected, ""); err == nil {
		t.Fatal("unconfigured isolation guard resolved to the bootstrap target")
	}
}
