package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ma "github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/onboardingwizard"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
	wshttp "github.com/mcpdev80/baseharbor/internal/workstationenrollment/http"
)

// onboarding_test.go holds the actual wizard entry regressions for card
// t-20261018-r27-onboarding-wizard (0427-043): both connection modes,
// repeated operations with no re-prompt, invalid imports, conflicts and
// cancel/EOF, against the accepted production seams. No runtime is required
// (card "Required checks": no runtime). The package is cmd/baha, so the
// existing Go unit CI group discovers and runs every test below.

// useOnboardingSeams installs the test adapters (nil fields keep their
// production defaults, so a test never runs against a nil authority) and the
// stdin seam.
func useOnboardingSeams(t *testing.T, adapters onboardingAuthorityAdapters, input io.Reader) {
	t.Helper()
	production := onboardingAdapters
	if adapters.Discover == nil {
		adapters.Discover = production.Discover
	}
	if adapters.ConnectionBound == nil {
		adapters.ConnectionBound = production.ConnectionBound
	}
	if adapters.AppAttach == nil {
		adapters.AppAttach = production.AppAttach
	}
	if adapters.ImportProfile == nil {
		adapters.ImportProfile = production.ImportProfile
	}
	savedAdapters, savedInput := onboardingAdapters, onboardingInput
	onboardingAdapters, onboardingInput = adapters, input
	t.Cleanup(func() {
		onboardingAdapters, onboardingInput = savedAdapters, savedInput
	})
}

// nonTerminalReader makes the wizard's interactive path reachable from tests:
// readerIsTerminal treats non-*os.File readers as terminals.
func nonTerminalReader(answers string) io.Reader {
	return strings.NewReader(answers)
}

func TestOnboardingCommandContract(t *testing.T) {
	cmd := onboardCommand()
	if cmd.Name != "onboard" || cmd.Summary == "" || cmd.Run == nil {
		t.Fatal("onboarding factory is not a callable command")
	}
	useOnboardingSeams(t, onboardingAuthorityAdapters{}, nonTerminalReader(""))
	var out strings.Builder
	if err := cmd.Run(context.Background(), []string{"--bogus"}, &out, &out); err == nil {
		t.Fatal("unknown option accepted")
	}
	// Non-interactive without an explicit Target fails closed; it never
	// guesses a connection.
	nonInteractive := cli.WithOutputOptions(context.Background(), cli.OutputOptions{NonInteractive: true})
	if err := cmd.Run(nonInteractive, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("non-interactive attach without a Target was accepted")
	}
}

func TestOnboardingWizardBootstrapMinimalPath(t *testing.T) {
	var out strings.Builder
	dec, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:  nonTerminalReader("new\nno\ndefault\nyes\n"),
		Out: &out,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{}, nil
		},
	})
	if err != nil {
		t.Fatalf("bootstrap wizard failed: %v\n%s", err, out.String())
	}
	if dec.Mode != onboardingwizard.ModeBootstrap || dec.App != onboardingwizard.AppNew || dec.Persistence != onboardingwizard.PersistenceSessionOnly {
		t.Fatalf("unexpected decision: %+v", dec)
	}
}

func TestOnboardingWizardExistingAppIsDiscoveredNotAsked(t *testing.T) {
	var out strings.Builder
	dec, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:  nonTerminalReader("attach\nno\nyes\n"),
		Out: &out,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{ExistingAppFound: true, Targets: []string{"homelab"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("wizard failed: %v\n%s", err, out.String())
	}
	if dec.App != onboardingwizard.AppExisting {
		t.Fatalf("existing application was not attached: %+v", dec)
	}
}

func TestOnboardingWizardExplicitExistingVsNewIsARealDecision(t *testing.T) {
	var out strings.Builder
	dec, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		// With an installation present the entry default is "attach";
		// the explicit existing-versus-new question belongs to attach.
		In:  nonTerminalReader("attach\nexisting\nno\nyes\n"),
		Out: &out,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{HasInstallation: true, ExistingAppFound: true, Targets: []string{"homelab"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("wizard failed: %v\n%s", err, out.String())
	}
	if dec.App != onboardingwizard.AppExisting {
		t.Fatalf("explicit existing choice lost: %+v", dec)
	}
	if !strings.Contains(out.String(), "no duplicate services") {
		t.Fatalf("the no-duplicate promise must be visible:\n%s", out.String())
	}
}

func TestOnboardingWizardAttachSingleTargetNotAsked(t *testing.T) {
	var out strings.Builder
	dec, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		// attach answers: (entry, app, persistence, review)
		In:  nonTerminalReader("attach\nnew\nno\nyes\n"),
		Out: &out,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{Targets: []string{"homelab"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("attach wizard failed: %v\n%s", err, out.String())
	}
	if dec.Mode != onboardingwizard.ModeAttach || dec.Target != "homelab" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
	if strings.Contains(out.String(), "Select the installation Target") {
		t.Fatalf("wizard asked for the only discoverable Target:\n%s", out.String())
	}
}

func TestOnboardingWizardTrustedLocalRunsSilently(t *testing.T) {
	var out strings.Builder
	dec, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:     nonTerminalReader(""),
		Out:    &out,
		Target: "homelab",
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{HasInstallation: true, TrustedLocal: true, Targets: []string{"homelab"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("trusted repeat operation must continue silently, got %v\n%s", err, out.String())
	}
	if dec.Mode != onboardingwizard.ModeAttach || dec.Target != "homelab" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
	if out.Len() != 0 {
		t.Fatalf("repeat operation produced output: %q", out.String())
	}
}

func TestOnboardingWizardCancelNeverMutates(t *testing.T) {
	if _, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:  nonTerminalReader("cancel\n"),
		Out: io.Discard,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{}, nil
		},
	}); !errors.Is(err, onboardingwizard.ErrCancel) {
		t.Fatalf("cancel did not abort cleanly: %v", err)
	}
}

func TestOnboardingWizardEOFNeverMutates(t *testing.T) {
	if _, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:  nonTerminalReader("new\n"),
		Out: io.Discard,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{}, nil
		},
	}); err == nil {
		t.Fatal("a truncated session produced a committable decision")
	}
}

func TestOnboardingWizardRemoveIsLocalOnly(t *testing.T) {
	var out strings.Builder
	dec, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:  nonTerminalReader("remove\nyes\n"),
		Out: &out,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{HasInstallation: true, Targets: []string{"homelab"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("remove wizard failed: %v\n%s", err, out.String())
	}
	if dec.Mode != onboardingwizard.ModeRemove || dec.Target != "homelab" {
		t.Fatalf("expected local removal, got %+v", dec)
	}
	if !strings.Contains(out.String(), "revokes nothing remotely") {
		t.Fatalf("local cleanup must state the revocation boundary:\n%s", out.String())
	}
}

func TestOnboardingWizardReviewDenyChangesNothing(t *testing.T) {
	var out strings.Builder
	if _, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:  nonTerminalReader("attach\nnew\nno\nno\n"),
		Out: &out,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{Targets: []string{"homelab"}}, nil
		},
	}); !errors.Is(err, onboardingwizard.ErrCancel) {
		t.Fatalf("denying the review must abort: %v\n%s", err, out.String())
	}
}

func TestOnboardingWizardAttachNeedsAKnownTarget(t *testing.T) {
	var out strings.Builder
	if _, err := onboardingwizard.Run(context.Background(), onboardingwizard.Input{
		In:  nonTerminalReader("attach\nnew\nno\n"),
		Out: &out,
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{}, nil
		},
	}); !errors.Is(err, onboardingwizard.ErrIncompleteConfiguration) {
		t.Fatalf("attach without any discoverable Target must fail closed, got %v\n%s", err, out.String())
	}
}

// --- production-path entry tests -------------------------------------------

func TestOnboardingAttachRegistersVerifiedConnection(t *testing.T) {
	configureTestTarget(t)
	target := "docker-dev"
	state, err := verifiedFreshCore(t, target)
	if err != nil {
		t.Fatal(err)
	}
	var bound atomic.Int64
	useOnboardingSeams(t, onboardingAuthorityAdapters{
		ConnectionBound: func(_ context.Context, got string) (string, error) {
			bound.Add(1)
			if got != target {
				return "", fmt.Errorf("verification ran for the wrong Target %q", got)
			}
			return state.ID + "|ca-serial-1", nil
		},
	}, nonTerminalReader("never consumed\n"))
	var out strings.Builder
	if err := onboardCommand().Run(context.Background(), []string{"--target", target}, &out, &out); err != nil {
		t.Fatalf("explicit attach failed: %v\n%s", err, out.String())
	}
	if bound.Load() != 1 {
		t.Fatalf("the connection must be verified exactly once before persisting, got %d", bound.Load())
	}
	fact, found, err := loadOnboardingConnection(target)
	if err != nil || !found {
		t.Fatalf("join fact was not persisted: %v found=%v", err, found)
	}
	if fact.InstallationID != state.ID || fact.IssuerSerial != "ca-serial-1" {
		t.Fatalf("unexpected fact: %+v", fact)
	}
	if strings.Contains(fact.ManagementCAPEM, "PRIVATE KEY") {
		t.Fatal("the local join fact carries private material")
	}
}

func TestOnboardingRepeatOperationRunsSilently(t *testing.T) {
	configureTestTarget(t)
	target := "docker-dev"
	state, err := verifiedFreshCore(t, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveOnboardingConnection(onboardingConnectionFact{InstallationID: state.ID, Target: target, IssuerSerial: "ca-serial-1"}); err != nil {
		t.Fatal(err)
	}
	var asked atomic.Int64
	useOnboardingSeams(t, onboardingAuthorityAdapters{
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{HasInstallation: true, TrustedLocal: true, Targets: []string{target}, InstallationID: state.ID}, nil
		},
		ConnectionBound: func(context.Context, string) (string, error) { return state.ID + "|ca-serial-1", nil },
	}, countingReader{strings.NewReader("never consumed\n"), &asked})
	var out strings.Builder
	if err := onboardCommand().Run(context.Background(), []string{"--target", target}, &out, &out); err != nil {
		t.Fatalf("repeat onboarding failed: %v\n%s", err, out.String())
	}
	if asked.Load() != 0 {
		t.Fatalf("a repeat operation with valid reusable access asked %d questions", asked.Load())
	}
}

type countingReader struct {
	io.Reader
	reads *atomic.Int64
}

func (r countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.reads.Add(1)
	}
	return n, err
}

func TestOnboardingAttachUnverifiedConnectionPersistsNothing(t *testing.T) {
	configureTestTarget(t)
	target := "docker-dev"
	useOnboardingSeams(t, onboardingAuthorityAdapters{
		ConnectionBound: func(context.Context, string) (string, error) {
			return "", machine.NewError(machine.ErrorVerificationFailed, "connection did not verify", "Reconcile the Core.", false)
		},
	}, nonTerminalReader(""))
	var out strings.Builder
	if err := onboardCommand().Run(context.Background(), []string{"--target", target}, &out, &out); err == nil {
		t.Fatal("an unverified connection persisted a join fact")
	}
	if _, found, _ := loadOnboardingConnection(target); found {
		t.Fatal("failed verification wrote local state")
	}
}

func TestOnboardingRemoveNeverTouchesRemote(t *testing.T) {
	configureTestTarget(t)
	target := "docker-dev"
	if err := saveOnboardingConnection(onboardingConnectionFact{InstallationID: "11111111-1111-4111-8111-111111111111", Target: target, IssuerSerial: "s"}); err != nil {
		t.Fatal(err)
	}
	useOnboardingSeams(t, onboardingAuthorityAdapters{
		Discover: func(context.Context) (onboardingwizard.InstallState, error) {
			return onboardingwizard.InstallState{HasInstallation: true, Targets: []string{target}}, nil
		},
		ConnectionBound: func(context.Context, string) (string, error) {
			t.Fatal("local removal must never verify or reach a remote authority")
			return "", nil
		},
	}, nonTerminalReader("remove\nyes\n"))
	var out strings.Builder
	if err := onboardCommand().Run(context.Background(), nil, &out, &out); err != nil {
		t.Fatalf("guided removal failed: %v\n%s", err, out.String())
	}
	if _, found, _ := loadOnboardingConnection(target); found {
		t.Fatal("removal kept the local fact")
	}
}

func TestOnboardingSessionOnlyMaterialIsOwnerOnlyAndCleanable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	if err := ensureSessionOnlyWorkstationIdentity(); err != nil {
		t.Fatal(err)
	}
	path, err := workstationIdentityPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("session material is not owner-only: %v", info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE KEY") {
		t.Fatal("session material file embeds a private key")
	}
	if err := removeWorkstationIdentity(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("session material survived cleanup")
	}
}

func TestOnboardingImportValidatesBeforeMutation(t *testing.T) {
	configureTestTarget(t)
	if _, err := verifiedFreshCore(t, "docker-dev"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := importConnectionProfileTransactionally(context.Background(), path); err == nil {
		t.Fatal("a malformed profile imported without error")
	}
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if entries, dirErr := os.ReadDir(filepath.Join(root, "profiles")); dirErr == nil && len(entries) > 0 {
		t.Fatal("a rejected import left partial state behind")
	}
	// World-readable input is rejected before any read.
	mode0644 := filepath.Join(dir, "public.json")
	if err := os.WriteFile(mode0644, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := importConnectionProfileTransactionally(context.Background(), mode0644); err == nil {
		t.Fatal("a world-readable profile was processed")
	}
}

func TestOnboardingImportConflictFailsClosedWithoutReplacement(t *testing.T) {
	configureTestTarget(t)
	state, err := verifiedFreshCore(t, "docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Join(root, "profiles")
	profile := onboardingTestProfile(t, state.ID)
	container := &ma.ProfileContainer{Version: onboardingProfileContainerVersion, DestPath: "profiles", Records: []ma.RecordWire{profile.ToWire()}, Timestamp: time.Now().UTC()}
	store, err := ma.NewStore(storeDir, onboardingProfileContainerVersion, ma.ImportPolicyConflictReplace, onboardingExpectedBinding(state.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(container); err != nil {
		t.Fatalf("the first valid import failed: %v", err)
	}
	file := filepath.Join(t.TempDir(), "second.json")
	if _, err := ma.WriteExport(file, container); err != nil {
		t.Fatal(err)
	}
	// The production onboarding path uses the default conflict-REJECT store
	// (the seam above seeded with replace so the identity already exists).
	err = importConnectionProfileTransactionally(context.Background(), file)
	if err == nil {
		t.Fatal("a duplicate identity imported silently under the reject policy")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("conflict must be named explicitly: %v", err)
	}
}

func TestOnboardingImportStoreBindingRejectsForeignInstallation(t *testing.T) {
	configureTestTarget(t)
	if _, err := verifiedFreshCore(t, "docker-dev"); err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	store, err := newConnectionProfileImportStore(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	foreign := onboardingTestProfile(t, "99999999-9999-4999-8999-999999999999")
	container := &ma.ProfileContainer{Version: onboardingProfileContainerVersion, DestPath: "profiles", Records: []ma.RecordWire{foreign.ToWire()}, Timestamp: time.Now().UTC()}
	if _, err := store.Import(container); err == nil {
		t.Fatal("a foreign-installation record entered the destination store")
	}
}

func TestOnboardingDecisionValidation(t *testing.T) {
	cases := []struct {
		name string
		dec  onboardingwizard.Decision
		ok   bool
	}{
		{"attach-needs-target", onboardingwizard.Decision{Mode: onboardingwizard.ModeAttach}, false},
		{"attach-ok", onboardingwizard.Decision{Mode: onboardingwizard.ModeAttach, Target: "t"}, true},
		{"import-both", onboardingwizard.Decision{Mode: onboardingwizard.ModeImport, ImportBundle: "b", ImportFile: "f"}, false},
		{"import-none", onboardingwizard.Decision{Mode: onboardingwizard.ModeImport}, false},
		{"import-profile", onboardingwizard.Decision{Mode: onboardingwizard.ModeImport, ImportFile: "f"}, true},
		{"unknown-mode", onboardingwizard.Decision{Mode: "teleport"}, false},
		{"bootstrap-session", onboardingwizard.Decision{Mode: onboardingwizard.ModeBootstrap, App: onboardingwizard.AppNew, Persistence: onboardingwizard.PersistenceSessionOnly}, true},
		{"remove-needs-target", onboardingwizard.Decision{Mode: onboardingwizard.ModeRemove}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.dec.Validate(); (err == nil) != tc.ok {
				t.Fatalf("Validate() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

// onboardingEnrollmentBridge adapts the accepted one-use authority to the
// production HTTP seam. It exists only to drive the REAL authority through
// the REAL production handler in this test; the durable owner lives in
// cmd/baha/workstation_enrollment.go.
type onboardingEnrollmentBridge struct {
	*ws.Authority
	mu    sync.Mutex
	facts map[string]string // scope -> current serial
}

func (e *onboardingEnrollmentBridge) RecordRegistryFact(_ context.Context, scope ws.Scope, _ time.Time, _, _, serial string, _ time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.facts[scope.Identity()] = serial
	return nil
}

func (e *onboardingEnrollmentBridge) CurrentSerial(_ context.Context, scope ws.Scope) (string, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	serial, ok := e.facts[scope.Identity()]
	return serial, ok, nil
}

// TestOnboardingDeviceBundleExchangeConsumesOnce proves the import intent
// binds to the ACCEPTED one-use workstation-enrollment boundary: the
// authority's one-use store cannot be consumed twice, and no private key ever
// crosses the wire.
func TestOnboardingDeviceBundleExchangeConsumesOnce(t *testing.T) {
	installationID := "11111111-1111-4111-8111-111111111111"
	scope := ws.Scope{TenantID: installationID, Station: "dev-laptop-01"}
	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	authority, err := ws.New(store, issuer, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge := &onboardingEnrollmentBridge{Authority: authority, facts: map[string]string{}}
	// Re-anchor the authority so its lifecycle Owner seam observes the
	// registry facts this bridge records (durable-owner shape).
	authority, err = ws.New(store, issuer, bridge)
	if err != nil {
		t.Fatal(err)
	}
	bridge.Authority = authority
	handler, err := wshttp.NewProductionHandler(bridge, func(reqCtx context.Context, station string) (ws.Scope, error) {
		tenant, ok := tenancy.FromContext(reqCtx)
		if !ok || tenant.TenantID != installationID {
			return ws.Scope{}, ws.ErrDenied
		}
		return ws.Scope{TenantID: installationID, Station: station}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(onboardingTenancyMiddleware(installationID, handler))
	defer server.Close()

	csrPEM := onboardingTestCSR(t, scope.Identity())
	var bundleToken, bundleNonce string
	bundle := &onboardingDeviceBundle{
		contract:       onboardingBundleContract,
		baseURL:        server.URL,
		client:         server.Client(),
		scope:          scope,
		certificateTTL: time.Hour,
		capture:        func(token, nonce string) { bundleToken, bundleNonce = token, nonce },
	}
	result, err := bundle.redeem(context.Background(), csrPEM)
	if err != nil {
		t.Fatalf("first redemption failed: %v", err)
	}
	if result.LeafPEM == "" || result.ExpiresAt.Before(time.Now()) {
		t.Fatalf("invalid issued material: %+v", result)
	}
	if strings.Contains(fmt.Sprintf("%+v", result), "PRIVATE KEY") {
		t.Fatal("private key material crossed the wire")
	}
	// The one-use grant is bound to the (token,nonce) digest Consume just
	// spent: replaying those credentials cannot enroll a second identity,
	// even with a generous TTL, because the store no longer holds a grant
	// matching their digests.
	replay, err := ws.NewRequest(scope, bundleToken, bundleNonce, csrPEM, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Enroll(context.Background(), replay); err == nil {
		t.Fatal("spent one-use credentials enrolled a second identity")
	}
}

// TestOnboardingDeviceBundleImportCompletesThroughEnrollment proves the import
// intent now completes the device-bundle path through the ACCEPTED one-use
// workstation-enrollment boundary in the production commit seam: a real
// in-process enrollment authority redeems the bundle, the one-use grant cannot
// be replayed, and the join fact binds the installation without ever carrying
// a private key. It exercises the same decision the wizard returns and the same
// commitOnboardingDecision path a human review would reach.
func TestOnboardingDeviceBundleImportCompletesThroughEnrollment(t *testing.T) {
	installationID := "11111111-1111-4111-8111-111111111111"
	scope := ws.Scope{TenantID: installationID, Station: "dev-laptop-01"}
	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	authority, err := ws.New(store, issuer, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge := &onboardingEnrollmentBridge{Authority: authority, facts: map[string]string{}}
	authority, err = ws.New(store, issuer, bridge)
	if err != nil {
		t.Fatal(err)
	}
	bridge.Authority = authority
	handler, err := wshttp.NewProductionHandler(bridge, func(reqCtx context.Context, station string) (ws.Scope, error) {
		tenant, ok := tenancy.FromContext(reqCtx)
		if !ok || tenant.TenantID != installationID {
			return ws.Scope{}, ws.ErrDenied
		}
		return ws.Scope{TenantID: installationID, Station: station}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(onboardingTenancyMiddleware(installationID, handler))
	defer server.Close()

	csrPEM := onboardingTestCSR(t, scope.Identity())
	var bundleToken, bundleNonce string
	seed := &onboardingDeviceBundle{
		contract:       onboardingBundleContract,
		baseURL:        server.URL,
		client:         server.Client(),
		scope:          scope,
		certificateTTL: time.Hour,
		capture:        func(token, nonce string) { bundleToken, bundleNonce = token, nonce },
	}
	if _, err := seed.redeem(context.Background(), csrPEM); err != nil {
		t.Fatalf("pre-seeding the bundle grant failed: %v", err)
	}

	// The import decision carries the issued bundle handle: installation ID,
	// station and grant token.
	decision := onboardingwizard.Decision{
		Mode:         onboardingwizard.ModeImport,
		ImportBundle: installationID + "|" + scope.Station + "|" + bundleToken,
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("import decision invalid: %v", err)
	}

	// Point the production ImportBundle seam at the test's in-process
	// enrollment authority. The seam binds the same onboardingDeviceBundle that
	// commitOnboardingDecision builds (contract + scope + baseURL) to the
	// accepted one-use boundary and redeems it through the real default.
	saved := onboardingAdapters
	onboardingAdapters.ImportBundle = func(ctx context.Context, bundle *onboardingDeviceBundle) (string, error) {
		bundle.baseURL = server.URL
		bundle.client = server.Client()
		return redeemOnboardingBundle(ctx, bundle)
	}
	t.Cleanup(func() { onboardingAdapters = saved })

	var out strings.Builder
	if err := commitOnboardingDecision(context.Background(), decision, &out); err != nil {
		t.Fatalf("the bundle import path still fails closed: %v\n%s", err, out.String())
	}
	joined := out.String()
	if !strings.Contains(joined, installationID) {
		t.Fatalf("import did not complete the enrollment join: %q", joined)
	}
	if strings.Contains(joined, "PRIVATE KEY") {
		t.Fatal("the import exposed private key material")
	}

	// The one-use grant is spent: replaying the same (token,nonce) against the
	// production authority cannot enroll a second identity, exactly as the
	// accepted one-use store guarantees.
	replay, err := ws.NewRequest(scope, bundleToken, bundleNonce, csrPEM, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Enroll(context.Background(), replay); err == nil {
		t.Fatal("a spent one-use bundle enrolled a second identity")
	}
}

// onboardingTenancyMiddleware authenticates the test operator exactly like
// the production middleware binds tenancy for the workstation endpoints.
func onboardingTenancyMiddleware(tenantID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := tenancy.WithContext(r.Context(), &tenancy.Context{TenantID: tenantID, ExternalIdentityID: "identity-a", Roles: []string{"developer"}})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// verifiedFreshCore registers a genuine Core installation at the Target
// runtime root and returns its identity.
func verifiedFreshCore(t *testing.T, target string) (coreinstallation.State, error) {
	t.Helper()
	root, err := targetRuntimeStateRootByName(target)
	if err != nil {
		return coreinstallation.State{}, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return coreinstallation.State{}, err
	}
	state, err := coreinstallation.Run(context.Background(), root, coreinstallation.Spec{Target: target, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	})
	return state, err
}

// onboardingTestProfile builds one schema-valid, non-secret management-profile
// record bound to the given installation. The recipient follows the schema's
// platform-core SPIFFE pattern and the store binding prefix; the scope is a
// canonical lowercase slug; the trust is self-consistent: the fingerprint
// hashes to the CA itself.
func onboardingTestProfile(t *testing.T, installationID string) ma.ManagementAccessProfile {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	caPEM, caSHA := onboardingTestCA(t)
	return ma.ManagementAccessProfile{
		Kind:           ma.ManagementProfile,
		Version:        ma.CanonicalRecordVersion,
		InstallationID: installationID,
		Trust:          ma.Trust{CAPEM: caPEM, SHA256: caSHA},
		Recipient:      "spiffe://baseharbor/platform/core/" + installationID + "/management",
		Scope:          onboardingProfileScope,
		NotBefore:      now.Add(-time.Minute),
		NotAfter:       now.Add(time.Hour),
		Import:         ma.ImportCapability{},
		Storage:        ma.Storage{Class: ma.SessionOnly, Durable: false, Encrypted: false},
	}
}

func onboardingTestCA(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(7),
		Subject:               pkix.Name{CommonName: "baseharbor-onboarding-test-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), hex.EncodeToString(sum[:])
}

// onboardingExpectedBinding is the destination binding the onboarding store
// uses (shared with the production seam so both agree).
func onboardingExpectedBinding(installationID string) ma.ExpectedBinding {
	return ma.ExpectedBinding{
		InstallationID:  installationID,
		RecipientPrefix: "spiffe://baseharbor/platform/core/" + installationID,
		Scope:           onboardingProfileScope,
	}
}

func onboardingTestCSR(t *testing.T, identity string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uriIdentity, err := url.Parse(identity)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uriIdentity}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func TestOnboardBootstrapExecutesRealOwnerPathAfterCore(t *testing.T) {
	start, owner := onboardingStartCore, onboardingCreateOwner
	t.Cleanup(func() {
		onboardingStartCore, onboardingCreateOwner = start, owner
	})
	events := make([]string, 0, 2)
	onboardingStartCore = func(context.Context, io.Writer) error {
		events = append(events, "core")
		return nil
	}
	onboardingCreateOwner = func(_ context.Context, slug, name string, _ io.Writer) error {
		events = append(events, "operator:"+slug+":"+name)
		return nil
	}
	decision := onboardingwizard.Decision{
		Mode: onboardingwizard.ModeBootstrap, App: onboardingwizard.AppNew,
		Persistence: onboardingwizard.PersistenceSecure,
		TenantSlug: "acme", TenantName: "Acme GmbH",
	}
	var output bytes.Buffer
	if err := commitOnboardingDecision(context.Background(), decision, &output); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"core", "operator:acme:Acme GmbH"}) {
		t.Fatalf("first install did not invoke Core before verified owner: %v", events)
	}
}

func TestOnboardBootstrapStopsBeforeOwnerWhenCoreFails(t *testing.T) {
	start, owner := onboardingStartCore, onboardingCreateOwner
	t.Cleanup(func() {
		onboardingStartCore, onboardingCreateOwner = start, owner
	})
	onboardingStartCore = func(context.Context, io.Writer) error {
		return errors.New("Core unavailable")
	}
	onboardingCreateOwner = func(context.Context, string, string, io.Writer) error {
		t.Fatal("owner mutation after failed Core installation")
		return nil
	}
	decision := onboardingwizard.Decision{
		Mode: onboardingwizard.ModeBootstrap, App: onboardingwizard.AppNew,
		Persistence: onboardingwizard.PersistenceSecure,
	}
	var output bytes.Buffer
	if err := commitOnboardingDecision(context.Background(), decision, &output); err == nil {
		t.Fatal("failed Core installation returned success")
	}
}
