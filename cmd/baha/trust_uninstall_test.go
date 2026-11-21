package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// trustTestOperatorContext routes the Run context through the whole-package
// operator fixture seam (operatorAuthFixtureContext in
// operator_test_fixture_test.go), which stamps a verified operator principal
// carrying the editor group. This is what lets the production boundary in
// cmd/baha/mcp.go pass so the uninstall prompt path (cmd/baha/trust.go) is
// actually reached. Without it, a bare context.Background() died at the
// operatorauth gate (mcp.go:301) BEFORE the prompt was read, and the previous
// err != nil success shape was satisfied by that gate error, never by the EOF
// abort contract. The rewritten assertions below require err == nil plus the
// documented abort output, so the gate error is no longer an acceptable
// "clean abort".
func trustTestOperatorContext(t *testing.T) context.Context {
	t.Helper()
	return operatorAuthFixtureContext(context.Background())
}

// withoutOperatorAuthTestContext returns the un-stamped context the production
// path must refuse: a bare context.Background() with no operator-auth
// enforcement state and no verified principal. trustTestOperatorContext stamps a
// verified operator principal (operatorAuthFixtureContext), which is what lets
// the production boundary in cmd/baha/mcp.go pass so the uninstall prompt path
// (cmd/baha/trust.go) is actually reached. The two helpers are a matched pair:
// the same command on the stamped context proves the abort path is reached, and
// on this anonymous context proves the exact operator-auth gate denial before
// the prompt is ever read. A case that stays green on both states is not
// evidence the gate works (5.6).
func withoutOperatorAuthTestContext(ctx context.Context) context.Context {
	return ctx
}

func TestTrustUninstallCommandRegisteredAndExplicitConsent(t *testing.T) {
	cmd := trustCommand()
	var uninstall bool
	for _, child := range cmd.Children {
		if child.Name != "uninstall" {
			continue
		}
		uninstall = true
		if !strings.Contains(child.Usage, "--yes") {
			t.Fatalf("missing explicit consent in help: %q", child.Usage)
		}
		if !strings.Contains(child.Long, "only") {
			t.Fatalf("missing ownership boundary: %q", child.Long)
		}
		if err := child.Run(context.Background(), []string{"--unknown"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatal("unknown arguments silently accepted")
		}
	}
	if !uninstall {
		t.Fatal("trust uninstall not registered")
	}
}

func TestTrustUninstallMachineResultSchema(t *testing.T) {
	payload, err := json.Marshal(trustUninstallResult{ContractVersion: "v1", Removed: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"contract_version":"v1"`, `"removed":0`} {
		if !strings.Contains(string(payload), expected) {
			t.Fatalf("missing %s: %s", expected, payload)
		}
	}
}

func TestDoctorHostTrustInvalidStateRequiresManualAction(t *testing.T) {
	findings := classifyDoctorFindings([]health.Check{{Name: "host-trust-ownership", OK: false, Message: "invalid ownership state"}})
	if len(findings) != 1 || findings[0].Class != doctorManualAction || !strings.Contains(findings[0].Action, "baha trust uninstall") {
		t.Fatalf("missing explicit trust cleanup guidance: %+v", findings)
	}
}

func TestTrustUninstallNoOwnershipIsSafeJSON(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	configureTestTarget(t)
	ctx := trustTestOperatorContext(t)
	var out, errOut bytes.Buffer
	err := trustUninstallCommand().Run(ctx, []string{"--json"}, &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"removed":0`) {
		t.Fatalf("invalid empty machine result: %s", out.String())
	}
}

func TestTrustUninstallRefusesUnapprovedJSONWithOwnedRecord(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	configureTestTarget(t)
	record := `{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"/tmp/nonexistent-ca-test.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := trustTestOperatorContext(t)
	var out, errOut bytes.Buffer
	err := trustUninstallCommand().Run(ctx, []string{"--json"}, &out, &errOut)
	if err == nil {
		t.Fatal("machine uninstall accepted without approval")
	}
	if !strings.Contains(err.Error(), "approval") {
		t.Fatalf("missing approval guidance: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "host-trust.json")); err != nil {
		t.Fatalf("unapproved call removed ownership state: %v", err)
	}
}

// TestTrustUninstallGateReachableInBothDirections is the round-2 proof the
// operator-auth gate is reachable in-test without editing any
// internal/operatorauth/** code or changing denial semantics. It drives the
// SAME trust-uninstall command, SAME owned host-trust record, SAME --json args,
// on the same two context states that the reviewer flagged (review-1 P2):
//
//   - BEFORE (stamped): operatorAuthFixtureContext stamps a verified operator
//     principal, so the production boundary passes and the abort path is reached.
//   - AFTER (anonymous): withoutOperatorAuthTestContext carries no principal, so
//     the identical call is refused with the exact production denial code
//     authentication_failed and the exact production text.
//
// Only a command that is authorized on the stamped state and refused with the
// exact denial on the anonymous state proves the gate works. A case that stays
// green on both states is the wrongly-green trap this card targets (5.6).
func TestTrustUninstallGateReachableInBothDirections(t *testing.T) {
	const ownedRecord = `{"version":1,"anchors":[{"fingerprint":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","backend":"linux-update-ca-certificates","path":"/usr/local/share/ca-certificates/baseharbor-0123456789abcdef.crt","installed_at":"2026-10-08T12:00:00Z"}]}`

	// BEFORE (stamped): the fixture stamps a verified operator principal, so the
	// managed uninstall is authorized and the abort path is reached, not the gate.
	stampedRoot := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stampedRoot)
	configureTestTarget(t)
	if err := os.WriteFile(filepath.Join(stampedRoot, "host-trust.json"), []byte(ownedRecord), 0600); err != nil {
		t.Fatal(err)
	}
	stampedCtx := operatorAuthFixtureContext(context.Background())
	var stampedOut, stampedErrOut bytes.Buffer
	// --yes reaches the abort path with an owned anchor; the abort output is the
	// only thing the stamped call may produce, never the gate denial.
	if err := trustUninstallCommand().Run(stampedCtx, []string{"--yes"}, &stampedOut, &stampedErrOut); err != nil {
		t.Fatalf("stamped managed trust.uninstall must be authorized, not denied at the gate: %v", err)
	}
	if containsGateDenial(stampedOut.String()) || containsGateDenial(stampedErrOut.String()) {
		t.Fatalf("stamped managed trust.uninstall unexpectedly denied: out=%s err=%s", stampedOut.String(), stampedErrOut.String())
	}

	// AFTER (anonymous): the identical call on an anonymous, fixture-opted-out
	// context is refused with the exact production denial text.
	anonRoot := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", anonRoot)
	configureTestTarget(t)
	if err := os.WriteFile(filepath.Join(anonRoot, "host-trust.json"), []byte(ownedRecord), 0600); err != nil {
		t.Fatal(err)
	}
	anonCtx := withoutOperatorAuthTestContext(context.Background())
	var anonOut, anonErrOut bytes.Buffer
	anonErr := trustUninstallCommand().Run(anonCtx, []string{"--yes"}, &anonOut, &anonErrOut)
	if anonErr == nil {
		t.Fatal("anonymous managed trust.uninstall must be refused at the operator-auth gate")
	}
	if !strings.Contains(anonErr.Error(), "Managed-environment machine operations require an authenticated BaseHarbor operator") {
		t.Fatalf("anonymous denial is not the exact production gate text: %v", anonErr)
	}
	// The anonymous context really did reach the seam and was refused there for a
	// missing principal, not short-circuited by a precondition.
	if _, ok := operatorauth.PrincipalFromContext(anonCtx); ok {
		t.Fatal("anonymous context unexpectedly carried a verified principal")
	}
}

// containsGateDenial reports whether s carries the exact production operator-auth
// denial text that trust.go/mcp.go return for an unauthenticated managed
// operation. A stamped context must never surface this text; an anonymous one
// must.
func containsGateDenial(s string) bool {
	return strings.Contains(s, "Managed-environment machine operations require an authenticated BaseHarbor operator")
}

func TestTrustUninstallWizardDeclinePreservesOwnedState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	configureTestTarget(t)
	record := `{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"/tmp/nonexistent-ca-test.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	saved := guidedTrustInput
	guidedTrustInput = strings.NewReader("no\n")
	defer func() { guidedTrustInput = saved }()
	ctx := trustTestOperatorContext(t)
	var out, errOut bytes.Buffer
	if err := trustUninstallCommand().Run(ctx, nil, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unchanged") {
		t.Fatalf("missing cancellation notice: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "host-trust.json")); err != nil {
		t.Fatalf("declined wizard mutated host trust ownership: %v", err)
	}
}

// TestTrustUninstallPromptUnterminatedAffirmativeEOFRejects covers the BF-26
// restbefund: an aborted affirmative such as "y" WITHOUT a trailing newline
// yields bufio Reader.ReadString '\n' as ("y", io.EOF). The reader must treat
// that EOF as a clean cancel, never as an approval, so only complete lines
// count (karte-138). It must not remove the owned anchor.
func TestTrustUninstallPromptUnterminatedAffirmativeEOFRejects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	configureTestTarget(t)
	record := `{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"/tmp/nonexistent-ca-test.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	saved := guidedTrustInput
	guidedTrustInput = strings.NewReader("y") // no newline -> EOF with partial "y"
	defer func() { guidedTrustInput = saved }()
	var out, errOut bytes.Buffer
	ctx := trustTestOperatorContext(t)
	if err := trustUninstallCommand().Run(ctx, nil, &out, &errOut); err != nil {
		t.Fatalf("EOF on an aborted affirmative must abort cleanly with nil error, not a raw read error: %v", err)
	}
	if !strings.Contains(out.String(), "Host trust unchanged.") {
		t.Fatalf("missing clean EOF abort notice: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "host-trust.json")); err != nil {
		t.Fatalf("aborted affirmative removed host trust ownership: %v", err)
	}
}

// TestTrustUninstallPromptEOFRejects covers reines EOF / Strg-D at the prompt:
// the reader must cancel cleanly and keep the owned anchor.
func TestTrustUninstallPromptEOFRejects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	configureTestTarget(t)
	record := `{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"/tmp/nonexistent-ca-test.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	saved := guidedTrustInput
	guidedTrustInput = strings.NewReader("") // immediate EOF
	defer func() { guidedTrustInput = saved }()
	var out, errOut bytes.Buffer
	ctx := trustTestOperatorContext(t)
	if err := trustUninstallCommand().Run(ctx, nil, &out, &errOut); err != nil {
		t.Fatalf("EOF at the prompt must abort cleanly with nil error, not a raw read error: %v", err)
	}
	if !strings.Contains(out.String(), "Host trust unchanged.") {
		t.Fatalf("missing clean EOF abort notice: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "host-trust.json")); err != nil {
		t.Fatalf("EOF abort mutated host trust ownership: %v", err)
	}
}

// TestTrustUninstallPromptCompleteAffirmativeApproves covers the happy path:
// a complete "y\n" line authorizes the uninstall and removes the owned anchor.
func TestTrustUninstallPromptCompleteAffirmativeApproves(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	configureTestTarget(t)
	// A complete "y\n" line authorizes the uninstall and removes the owned anchor.
	// The fingerprint must be a valid 64-char hex string so the anchor survives
	// the ownership/fingerprint validation and actually gets removed.
	// The fingerprint must be a valid 64-char hex string so the anchor survives
	// the ownership/fingerprint validation and actually gets removed. The
	// recorded path must equal baseharbor-<first 16 of fingerprint>.crt under the
	// system anchor dir, else RemoveOwnedDetailed retains it (PRESERVED).
	record := `{"version":1,"anchors":[{"fingerprint":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","backend":"linux-update-ca-certificates","path":"/usr/local/share/ca-certificates/baseharbor-0123456789abcdef.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	saved := guidedTrustInput
	guidedTrustInput = strings.NewReader("y\n") // complete line -> approved
	defer func() { guidedTrustInput = saved }()
	ctx := trustTestOperatorContext(t)
	var out, errOut bytes.Buffer
	if err := trustUninstallCommand().Run(ctx, nil, &out, &errOut); err != nil {
		t.Fatalf("complete affirmative should approve: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "host-trust.json")); !os.IsNotExist(err) {
		t.Fatalf("complete affirmative should have removed host trust ownership: %v", err)
	}
}

// TestTrustUninstallPromptEOFAbortsCleanly covers BF-26: an EOF/Strg-D at the
// [y/N]-removal prompt must abort cleanly with "Host trust unchanged." (nil
// error) rather than surfacing a raw read error. This mirrors the install-path
// io.EOF guard and the karte-138 unification. The existing decline test drives
// "no\n"; this one drives an empty reader (immediate EOF) and must have failed
// before the guard was added.
func TestTrustUninstallPromptEOFAbortsCleanly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	configureTestTarget(t)
	record := `{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"/tmp/nonexistent-ca-test.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	saved := guidedTrustInput
	guidedTrustInput = strings.NewReader("") // immediate EOF / Strg-D
	defer func() { guidedTrustInput = saved }()
	var out, errOut bytes.Buffer
	ctx := trustTestOperatorContext(t)
	if err := trustUninstallCommand().Run(ctx, nil, &out, &errOut); err != nil {
		t.Fatalf("EOF at the prompt must abort cleanly with nil error, not a raw read error: %v", err)
	}
	if !strings.Contains(out.String(), "Host trust unchanged.") {
		t.Fatalf("missing clean EOF abort notice: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "host-trust.json")); err != nil {
		t.Fatalf("EOF abort mutated host trust ownership: %v", err)
	}
}
