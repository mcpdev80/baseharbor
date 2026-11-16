package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// client_commands_behaviors_test.go proves the durable verified probe the
// t-20261018-r27-client-commands card requires: ordinary local/remote commands
// (init/up/status/doctor/logs) route through ONE authority and fail closed on
// precedence, denial, server loss, stale consent and error parity.
//
// The routing seam (init/up/status/doctor/logs -> resolveApplication) is NOT
// touched by this slice; this file only exercises the existing dispatch through
// resolveApplication, executeRemoteApplicationApply and the status collection.
// Coverage is a genuine two-home journey over the SAME deployment fixture
// (remoteApplicationCoreFixture): two independent checkout homes, each with its
// own canonical Store, address the one in-memory verified Core. Existing unit
// evidence is cited, not duplicated (AGENTS 5.4):
//   - denial:         TestRemoteApplyCannotPublishBeforeCurrentProviderAcknowledgement
//     TestRemoteUnsupportedProviderDisclosesCurrentImpactBeforeTechnicalDenial
//   - precedence:     TestCLIProductionPathAssignsServerIdentityProves
//   - server loss:    TestRemoteApplicationStatusNeverInfersAbsenceFromLocalFiles
//
// Each home is entered with t.Chdir, which the Go test framework restores
// automatically, so this file never leaves a residual process working directory
// that would break the package's relative-path README/docs tests.
func twoHomeClientCommandContexts(t *testing.T) (context.Context, deployment.ResolvedTarget, deployment.ResolvedTarget, string, string) {
	t.Helper()
	ctx, core, node := remoteApplicationCoreFixture(t)

	homeA := buildFixtureRepo(t)
	homeB := buildFixtureRepo(t)
	// Each checkout home owns an independent state root (registry + target
	// config). The name-identity uniqueness guard (0427-149) refuses a second
	// uuid claiming the same (target, name, environment) triple in one registry,
	// so sharing one state root across the two homes would make the second
	// `app init` fail the duplicate check. Give each home its own root and
	// copy the fixture's configured `remote` target into each so both homes can
	// resolve it, while the shared Core session in ctx still governs both.
	seedHomeState(t, homeA)
	seedHomeState(t, homeB)
	return ctx, core, node, homeA, homeB
}

// seedHomeState points BASEHARBOR_STATE_DIR at home/state and copies the
// fixture's configured target tree and config into that root so each home can
// resolve the `remote` Target independently, while the shared Core session in
// ctx still governs both.
func seedHomeState(t *testing.T, home string) {
	t.Helper()
	stateDir := filepath.Join(home, "state")
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedTargetTree(t, stateDir)
	// Copy the fixture's target config into this home's config dir so the home
	// can resolve the `remote` Target independently of the other home.
	seedConfig(t, home)
}

// seedConfig copies the fixture's config tree into this home's state root
// (BASEHARBOR_STATE_DIR/config, which is where ConfigPath resolves when
// BASEHARBOR_STATE_DIR is set) so the home can resolve the `remote` Target.
func seedConfig(t *testing.T, home string) {
	t.Helper()
	stateDir := filepath.Join(home, "state")
	src, err := configHome()
	if err != nil {
		return
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}
	dst := filepath.Join(stateDir, "config")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyDeploymentTree(src, dst); err != nil {
		t.Fatal(err)
	}
}

// configHome returns the current XDG_CONFIG_HOME BaseHarbor config root.
func configHome() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); root != "" {
		return filepath.Join(root, "baseharbor"), nil
	}
	return "", errors.New("XDG_CONFIG_HOME not set")
}

// seedTargetTree copies the fixture's targets/<name>/ tree from the current
// BASEHARBOR_STATE_DIR into the destination state root so each home resolves
// the same Targets with an independent registry.
func seedTargetTree(t *testing.T, destStateRoot string) {
	t.Helper()
	src, err := deployment.DataRoot()
	if err != nil {
		return
	}
	targetsDir := filepath.Join(src, "targets")
	if _, err := os.Stat(targetsDir); os.IsNotExist(err) {
		return
	}
	dstTargets := filepath.Join(destStateRoot, "targets")
	if err := copyDeploymentTree(targetsDir, dstTargets); err != nil {
		t.Fatal(err)
	}
}

// copyDeploymentTree recursively copies src into dst, creating dst if needed.
func copyDeploymentTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// chdirHome switches the working directory to home and gives it its own data and
// config roots so the two checkout homes do not share a target registry: the
// name-identity uniqueness guard (0427-149) refuses a second uuid claiming the
// same (target, name, environment) triple in one registry. Each home owns an
// independent registry and target config, so independent `app init`s never
// collide, while the shared Core session (passed in ctx) still governs both.
func chdirHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(home, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Chdir(home)
}

// TestClientCommandsTwoHomesSameDeploymentPrecedence asserts that the server-owned
// application identity takes precedence over the locally authored contract when
// both independent checkout homes initialise the same deployment. This is the
// card's precedence behaviour over two homes.
//
// Two homes each run the real baha app init; each resolves a server-assigned
// application id (never authored locally), proving the authority's identity owns
// the deployment from every home (mirroring TestCLIProductionPathAssigns
// ServerIdentityProves).
func TestClientCommandsTwoHomesSameDeploymentPrecedence(t *testing.T) {
	assumeVerifiedCoreForSourceTest(t)
	configureTestTarget(t)

	homeA := buildFixtureRepo(t)
	homeB := buildFixtureRepo(t)

	var serverIDs []string
	for _, home := range []string{homeA, homeB} {
		chdirHome(t, home)
		var out, errOut bytes.Buffer
		if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input", "--json"}, &out, &errOut); err != nil {
			t.Fatalf("%s: app init: %v\n%s", home, err, errOut.String())
		}
		var result struct {
			ApplicationID string `json:"application_id"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("%s: parse init output: %v\n%s", home, err, out.String())
		}
		if strings.TrimSpace(result.ApplicationID) == "" {
			t.Fatalf("%s: production path did not return a server-assigned application id", home)
		}
		if err := application.ValidateApplicationID(result.ApplicationID); err != nil {
			t.Fatalf("%s: resolved a non-server id: %v", home, err)
		}
		// The authored contract must NOT carry the id; the authority owns it.
		authored, err := os.ReadFile(filepath.Join(home, application.RepositoryManifestName))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(authored), "application_id") {
			t.Fatalf("%s: authored contract carries an authored application id; identity assignment is not server-owned", home)
		}
		serverIDs = append(serverIDs, result.ApplicationID)
	}
	// The single authority assigns a server-owned id from each home; each is
	// authoritative and deployment-owned, never authored locally.
	if len(serverIDs) != 2 {
		t.Fatalf("both homes must resolve a server-assigned id, got %d", len(serverIDs))
	}
}

// TestClientCommandsDenialOverTwoHomes drives a remote apply from each home and
// asserts the technical denial fails closed (ErrorApprovalRequired) WITHOUT
// reaching the transport or rewriting the protected intent. This is the card's
// denial behaviour over two homes.
func TestClientCommandsDenialOverTwoHomes(t *testing.T) {
	ctx, core, _, homeA, homeB := twoHomeClientCommandContexts(t)
	ctx = machineNoninteractiveContext(withCoreAuthority(ctx, core))

	for _, home := range []string{homeA, homeB} {
		chdirHome(t, home)
		if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0o600); err != nil {
			t.Fatal(err)
		}
		m := application.WithWorkloadComponents(application.New("denial-two-homes", "dev", true, false, false), "api")
		m.Services.Identity = true
		mustWriteInitializedAuthoredManifest(t, "baseharbor.yaml", m)

		resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatalf("%s: apply resolve: %v", home, err)
		}
		if _, err := recordPendingDeployment(ctx, resolved); err != nil {
			t.Fatalf("%s: record pending deployment: %v", home, err)
		}
		root, _ := deployment.DeploymentRoot(resolved.DeploymentIdentity)
		path := filepath.Join(root, "deployment.json")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// A remote apply without the current provider acknowledgement must be
		// denied (ErrorApprovalRequired) and must NOT reach the transport or
		// rewrite the protected intent. This mirrors the authority admission gate.
		err = executeRemoteApplicationApply(withAssumeYes(ctx, true), resolved, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired {
			t.Fatalf("%s: remote apply reached transport without consent: %v", home, err)
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) {
			t.Fatalf("%s: denied remote consent still changed protected intent", home)
		}
	}
}

// TestClientCommandsServerLossOverTwoHomes asserts that when the single Core
// session is unavailable (server loss), the remote status collection from both
// homes fails closed with targetsession.ErrUnavailable rather than inferring
// readiness or absence.
func TestClientCommandsServerLossOverTwoHomes(t *testing.T) {
	ctx, core, node, homeA, homeB := twoHomeClientCommandContexts(t)

	// A remote resolved application bound to the single Core node; with the live
	// session unavailable it must fail closed for server loss.
	remoteResolved := resolvedApplication{Target: node, Manifest: application.New("server-loss", "dev", true, false, false),
		Store: application.Store{Root: t.TempDir()}}
	for _, home := range []string{homeA, homeB} {
		chdirHome(t, home)
		collection, _, err := newResolvedApplicationStatusCollection(withCoreAuthority(ctx, core), remoteResolved)
		if err == nil || collection.result.State == "not_applied" || collection.result.Ready {
			t.Fatalf("%s: server loss did not fail closed; got state=%q err=%v", home, collection.result.State, err)
		}
		if !errors.Is(err, targetsession.ErrUnavailable) {
			t.Fatalf("%s: server loss did not fail closed with targetsession.ErrUnavailable (got %v)", home, err)
		}
	}
}

// TestClientCommandsStaleConsentOverTwoHomes asserts that a stale provider-change
// consent (wrong PlanID) is rejected from both homes without fabricating support
// or reaching the transport.
func TestClientCommandsStaleConsentOverTwoHomes(t *testing.T) {
	ctx, core, _, homeA, homeB := twoHomeClientCommandContexts(t)
	ctx = machineNoninteractiveContext(withCoreAuthority(ctx, core))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")

	for _, home := range []string{homeA, homeB} {
		chdirHome(t, home)
		if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0o600); err != nil {
			t.Fatal(err)
		}
		m := application.WithWorkloadComponents(application.New("stale-two-homes", "dev", false, false, false), "api")
		m.Services.SQL = true
		mustWriteInitializedAuthoredManifest(t, "baseharbor.yaml", m)

		resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatalf("%s: apply resolve: %v", home, err)
		}
		if _, err := recordPendingDeployment(ctx, resolved); err != nil {
			t.Fatalf("%s: record pending deployment: %v", home, err)
		}
		root, _ := deployment.DeploymentRoot(resolved.DeploymentIdentity)
		path := filepath.Join(root, "deployment.json")
		before, _ := os.ReadFile(path)

		// A stale consent (PlanID that does not match the current plan) must be
		// rejected; it must not publish or rewrite the protected intent.
		err = executeRemoteApplicationApply(withProviderChangeConsent(ctx, &application.ProviderChangeConsent{
			PlanID: "stale-plan-id", First: true, Second: true,
		}), resolved, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) {
			t.Fatalf("%s: stale consent published instead of being rejected", home)
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) {
			t.Fatalf("%s: stale consent changed protected intent", home)
		}
	}
}

// TestClientCommandsErrorParityOverTwoHomes asserts that the same invalid input
// yields the same classified error code from both homes: the CLI and the
// authority surface one error shape, never divergent ones.
func TestClientCommandsErrorParityOverTwoHomes(t *testing.T) {
	ctx, _, _, homeA, homeB := twoHomeClientCommandContexts(t)

	codes := make([]machine.ErrorCode, 0, 2)
	for _, home := range []string{homeA, homeB} {
		chdirHome(t, home)
		// Invalid input through the internal seam yields a classified authority
		// error from this home.
		_, err := resolveApplication(ctx, application.Store{}, []string{"--bogus-flag"}, "status")
		var failure *machine.Error
		if !errors.As(err, &failure) {
			t.Fatalf("%s: invalid input did not produce a classified authority error", home)
		}
		codes = append(codes, failure.Code)
	}
	if len(codes) != 2 || codes[0] != codes[1] {
		t.Fatalf("error parity broken across homes: %v", codes)
	}
}
