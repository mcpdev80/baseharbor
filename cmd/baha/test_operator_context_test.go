package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// runWithIOAsTestOperator keeps functional CLI tests on the same authenticated
// path as a real managed-environment invocation. Tests of unauthenticated
// denial call runWithIO directly so they continue to prove the fail-closed seam.
func runWithIOAsTestOperator(ctx context.Context, args []string, out, errOut io.Writer) error {
	return runWithIO(testOperatorContext(ctx), args, out, errOut)
}

func testOperatorContext(ctx context.Context) context.Context {
	if _, ok := operatorauth.PrincipalFromContext(ctx); ok {
		ctx = operatorauth.WithMembershipProviderIfAbsent(ctx, &readGrantingMembershipProvider{})
		return operatorauth.WithEnforcementIfAbsent(ctx)
	}
	principal := &identity.Principal{
		Issuer: "https://issuer.test", Subject: "cli-test-operator",
		IssuerVerified: true, Groups: []string{"editor"},
	}
	ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
	ctx = operatorauth.WithMembershipProviderIfAbsent(ctx, &readGrantingMembershipProvider{})
	return operatorauth.WithEnforcementIfAbsent(ctx)
}

// testIsolatedSelectionContext neutralises the process-wide selection state a
// test never configured, for cases that resolve a Target WITHOUT a fixture that
// configures one: the init working-directory paths and the Perses status
// probes. It must not be used by a fixture that pins BASEHARBOR_STATE_DIR to
// its own root (configureTestTarget keeps it for exactly that reason) — those
// fixtures own the selection through their config and their explicit
// BASEHARBOR_TARGET.
//
// The cmd/baha package runs every test inside ONE process, and two kinds of
// selection state leak across case boundaries:
//
//  1. The bootstrap helper activates a Target through os.Setenv of
//     BASEHARBOR_TARGET (cmd/baha/runtime_integration_test_support_test.go),
//     which the testing package never rolls back (unlike t.Setenv).
//     effectiveTarget gives that activation priority over the Target a test
//     would otherwise resolve (cmd/baha/target.go:
//     selectedTargetName(explicit, activated, cfg)).
//  2. The organization selection persists to disk (internal/orgconfig/store.go:
//     ActivePath/SaveActive) under the resolved config root. The validate cell
//     reuses persistent runner state under the host HOME and does not export
//     XDG_CONFIG_HOME, so on that runner the lookup lands in
//     /home/runner/.config, where a selection from an earlier invocation
//     survives. The integration cell, however, activates its bootstrap Target
//     through os.Setenv (1), and a test that rewrites the config root would
//     then be redirected away from the Target it configured. The override
//     therefore applies exactly where the lookup would consult the organization
//     selection instead of the config: when nothing is activated.
//
// Either leak makes a test that never selects a Target resolve a name it did
// not configure and fail in setup with `target "..." is not configured`
// instead of exercising the behaviour it asserts.
//
// This is the guarantee configureIsolatedTestTarget already documents for
// hermetic fixtures (cmd/baha/target_test_helpers_test.go, "Clear leaked
// globals FIRST"), extended to the disk-backed organization selection and
// applied to the cases that never reach that fixture. The private config root
// and the cleared variables are restored by the testing package at test
// cleanup, so the neutralisation can neither leak into another test nor mask a
// Target the test itself selects.
func testIsolatedSelectionContext(t *testing.T) {
	t.Helper()
	if os.Getenv("BASEHARBOR_TARGET") != "" || os.Getenv("BASEHARBOR_STATE_DIR") != "" {
		// The test's own selection or its pinned state root is authoritative:
		// clearing either would redirect it away from the fixture it configured
		// (configureTestTarget keeps BASEHARBOR_STATE_DIR for exactly that
		// reason). Nothing ambient can outrank an explicit activation, so
		// there is nothing to neutralise here.
		return
	}
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_ENV", "")
	// With no activation, an organization selection persisted by the host (a
	// persistent runner HOME) is what the resolver consults instead of the
	// test's config, so the lookup moves to a private, selection-free config
	// root (internal/orgconfig/store.go: configRoot). The condition is
	// evaluated for THIS test's environment, so a test that legitimately
	// persists a selection under a host root keeps observing it.
	if activeOrganizationSelectionVisibleToHost() {
		t.Setenv("XDG_CONFIG_HOME", hostConfigIsolationDir())
	}
}

// activeOrganizationSelectionVisibleToHost reports whether an active
// organization selection is readable from the host config root the orgconfig
// lookup would resolve WITHOUT an explicit root (internal/orgconfig/store.go:
// configRoot falls back to os.UserConfigDir). It is the one host-state case the
// environment variables cannot neutralise.
func activeOrganizationSelectionVisibleToHost() bool {
	path, err := orgconfig.ActivePath()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(path)
	return statErr == nil
}

// hostConfigIsolationDir returns a private, selection-free config root for the
// current test process. The directory itself is discarded with the test's
// temporary root; only the environment override needs the testing package's
// restore.
func hostConfigIsolationDir() string {
	root, err := os.MkdirTemp("", "baseharbor-0428-223-cfg-")
	if err != nil {
		panic("create host config isolation root: " + err.Error())
	}
	return root
}

// envWithoutLeakedTargetSelection drops the inherited selection environment
// from a self-executed test child so it resolves the selection its own fixture
// configures. A BASEHARBOR_TARGET or BASEHARBOR_STATE_DIR the parent process
// carries would otherwise outrank the child's own config
// (internal/deployment/target.go: ConfigPath) and send it to a Target the
// child never selected.
func envWithoutLeakedTargetSelection(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "BASEHARBOR_TARGET", "BASEHARBOR_STATE_DIR", "BASEHARBOR_ENV":
			continue
		}
		out = append(out, entry)
	}
	return out
}
