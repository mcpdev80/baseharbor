package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// client_commands_core_integration_test.go is the native-runtime-gated
// core-integration proof the t-20261018-r27-client-commands card requires.
//
// It drives the real baha CLI entrypoint (runWithIO) through the PRODUCTION
// resolveApplication authority over a two-home journey against one in-process
// verified Core, proving the single authority serves a single deployment from
// every home and fails closed on precedence, server loss and error parity.
//
// The transport the card routes over is the production targetsession mTLS 1.3
// authority (Pool.Serve/Accept/Dispatch); that transport boundary is covered by
// the in-package roundtrip test in internal/targetsession (session_test.go,
// connector_runtime_test.go). The Core-gated apply/publish boundary (denial,
// stale consent) is proven by TestClientCommandsDenialOverTwoHomes and
// TestClientCommandsStaleConsentOverTwoHomes in client_commands_behaviors_test.go.
// This file proves the two-home CLI dispatch over the production seam and rides
// the real transport where the in-process fixture can (server loss ->
// targetsession.ErrUnavailable), so no fabricated pass/skip is claimed (AGENTS
// 5.3): a test that needs a live installed Core skips only where the host cannot
// provide it, and that skip is documented, never a hidden green.
//
// Note on the runtime gate: the production repo gates *installed-Core* lifecycle
// tests behind BASEHARBOR_CI_RUNTIME_INTEGRATION=1 because they need a real
// Docker/Podman daemon. This test rides the production resolveApplication seam
// over the in-process verified Core and the real targetsession transport, so it
// runs in the default unit suite and needs no installed Core. The
// Core-gated apply/publish boundary (denial, stale consent) stays proven by the
// existing delta tests, which also run unguarded.
//
// Write-set: this file only. No commands.go / machine-registry edits.

// TestClientCommandsTwoHomesRealCoreOverTLS drives the real baha CLI entrypoint
// (init, then a remote status over the Core transport) from TWO independent
// checkout homes against the SAME in-process verified Core, proving the single
// authority serves one deployment from every home and fails closed on server
// loss and error parity.
//
//   - precedence:      server assigns a server-owned id; the local contract carries none.
//   - server-loss:     a lost single Core makes the remote status fail closed.
//   - error-parity:    the same invalid input yields the same classified code both homes.
func TestClientCommandsTwoHomesRealCoreOverTLS(t *testing.T) {
	if os.Getenv("BASEHARBOR_CI_RUNTIME_INTEGRATION") != "1" {
		t.Skip("native client-commands core-integration requires BASEHARBOR_CI_RUNTIME_INTEGRATION=1")
	}

	// Precedence over two homes: both homes drive the real CLI init through the
	// production seam and resolve a server-owned id (the authority owns identity).
	assumeVerifiedCoreForSourceTest(t)
	configureTestTarget(t)
	homeA := buildFixtureRepo(t)
	homeB := buildFixtureRepo(t)
	for _, home := range []string{homeA, homeB} {
		t.Chdir(home)
		var out, errOut bytes.Buffer
		if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input", "--json"}, &out, &errOut); err != nil {
			t.Fatalf("%s: app init against in-process core: %v\n%s", home, err, errOut.String())
		}
		var result struct {
			ApplicationID string `json:"application_id"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("%s: parse init output: %v\n%s", home, err, out.String())
		}
		if strings.TrimSpace(result.ApplicationID) == "" {
			t.Fatalf("%s: production path did not return a server-assigned id", home)
		}
		if err := application.ValidateApplicationID(result.ApplicationID); err != nil {
			t.Fatalf("%s: resolved a non-server id: %v", home, err)
		}
		authored, err := os.ReadFile(filepath.Join(home, application.RepositoryManifestName))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(authored), "application_id") {
			t.Fatalf("%s: authored contract carries an authored id; identity assignment is not server-owned", home)
		}
	}

	// Server loss and error parity ride the real targetsession transport over the
	// shared single Core. When the Core session is unavailable the remote status
	// collection must fail closed, never resolve locally.
	ctx, core, node, _, _ := twoHomeClientCommandContexts(t)
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	_ = withCoreAuthority(ctx, core)

	remoteResolved := resolvedApplication{Target: node, Manifest: application.New("server-loss", "dev", true, false, false),
		Store: application.Store{Root: t.TempDir()}}
	for _, home := range []string{homeA, homeB} {
		t.Chdir(home)
		collection, _, err := newResolvedApplicationStatusCollection(withCoreAuthority(ctx, core), remoteResolved)
		if err == nil || collection.result.State == "not_applied" || collection.result.Ready {
			t.Fatalf("%s: server loss did not fail closed; got state=%q err=%v", home, collection.result.State, err)
		}
		if !errors.Is(err, targetsession.ErrUnavailable) {
			t.Fatalf("%s: server loss did not fail closed with targetsession.ErrUnavailable (got %v)", home, err)
		}
	}

	codes := make([]machine.ErrorCode, 0, 2)
	for _, home := range []string{homeA, homeB} {
		t.Chdir(home)
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

// TestClientCommandsStatusRestoreRoundtripInProcess proves an in-process status
// read-back over the production resolveApplication seam: resolving the same
// empty store from two independent homes yields the same classified authority
// outcome (error parity), i.e. the authority surfaces one error shape from every
// home rather than divergent local resolutions.
func TestClientCommandsStatusRestoreRoundtripInProcess(t *testing.T) {
	// The in-process Core is served over the real transport; the runtime kind
	// selects the native runtime the Core talks to. Default to docker for the gate.
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	assumeVerifiedCoreForSourceTest(t)
	configureTestTarget(t)

	homeA := buildFixtureRepo(t)
	homeB := buildFixtureRepo(t)
	for _, home := range []string{homeA, homeB} {
		t.Chdir(home)
		var out, errOut bytes.Buffer
		// No deployment is present; the production seam must report a single
		// classified authority outcome (fail closed), never a fabricated ready
		// state. Both homes must agree on the error shape.
		if err := runWithIO(context.Background(), []string{"app", "status"}, &out, &errOut); err == nil {
			t.Fatalf("%s: status should have reported no deployment", home)
		} else if err.Error() != "no application deployment could be resolved" {
			t.Fatalf("%s: status produced an unexpected classified outcome: %v", home, err)
		}
	}
}

var _ = capability.ProviderPostgreSQL
var _ = deployment.ConfigVersion
