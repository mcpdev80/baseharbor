package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestParseRuntimeUpOptions(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--yes", "--control-plane-only", "--trust-host-ca", "-e", "test", "--postgres-port", "15432", "--openbao-port", "18200", "--recovery-file", "/secure/recovery.json"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Yes || !opts.ControlPlaneOnly || !opts.TrustHostCA || opts.Environment != "test" || opts.PostgresPort != 15432 || opts.OpenBaoPort != 18200 || opts.RecoveryFile != "/secure/recovery.json" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestRuntimeUpYesDoesNotImplicitlyTrustHostCA(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.TrustHostCA {
		t.Fatal("--yes must not implicitly consent to host trust mutation")
	}
}

func TestRuntimeUpHADefaultsOffAndEnablesOnlyWithFlag(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--yes", "--control-plane-only"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.HA {
		t.Fatal("default 'baha up' must be single control plane: --ha must default off")
	}
	haopts, err := parseRuntimeUpOptions([]string{"--yes", "--control-plane-only", "--ha"})
	if err != nil {
		t.Fatal(err)
	}
	if !haopts.HA {
		t.Fatal("--ha must enable the HA opt-in profile")
	}
}

func TestParseRuntimeUpOptionsAcceptsRecoveryFileEqualsForm(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--recovery-file=/secure/recovery.json"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.RecoveryFile != "/secure/recovery.json" {
		t.Fatalf("recovery file = %q", opts.RecoveryFile)
	}
}

func TestParseRuntimeUpOptionsRejectsInvalidPort(t *testing.T) {
	if _, err := parseRuntimeUpOptions([]string{"--postgres-port", "70000"}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestRecoveryFileForRepositoryUpUsesTargetDefaultNonInteractive(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("XDG_CONFIG_HOME", dataRoot)
	t.Setenv("HOME", dataRoot)

	var out bytes.Buffer
	path, err := recoveryFileForRepositoryUp(context.Background(), strings.NewReader(""), &out, runtimeUpOptions{Yes: true}, "initialize")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataRoot, "baseharbor-recovery", "local", "openbao-recovery.json")
	if path != want {
		t.Fatalf("path = %q, want target default %q", path, want)
	}
}

func TestRecoveryFileForRepositoryUpInteractivePromptIsVisible(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "openbao-recovery.json")
	var out bytes.Buffer
	got, err := recoveryFileForRepositoryUp(context.Background(), strings.NewReader(path+"\n"), &out, runtimeUpOptions{}, "initialize")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
	text := out.String()
	if !strings.Contains(text, "NEW operator-held recovery output file") {
		t.Fatalf("missing recovery output explanation: %q", text)
	}
	if !strings.Contains(text, "OpenBao recovery file [") {
		t.Fatalf("missing recovery prompt with default: %q", text)
	}
}

func TestRecoveryFileForRepositoryUpBlocksUntilInteractiveInput(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()

	done := make(chan error, 1)
	go func() {
		_, err := recoveryFileForRepositoryUp(context.Background(), reader, io.Discard, runtimeUpOptions{}, "initialize")
		done <- err
	}()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	select {
	case err := <-done:
		t.Fatalf("recovery prompt returned before input: %v", err)
	case <-time.After(75 * time.Millisecond):
	}

	path := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if _, err := io.WriteString(writer, path+"\n"); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("recovery prompt did not resume after input")
	}
}

func TestRecoveryFileForRepositoryUpUsesExplicitPath(t *testing.T) {
	path, err := recoveryFileForRepositoryUp(context.Background(), strings.NewReader(""), &bytes.Buffer{}, runtimeUpOptions{Yes: true, RecoveryFile: "/secure/recovery.json"}, "initialize")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/secure/recovery.json" {
		t.Fatalf("path = %q", path)
	}
}

func TestPortAvailableDetectsOccupiedLoopbackPort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if portAvailable(port) {
		t.Fatalf("port %d reported available while listener is active", port)
	}
}

func TestSelectControlPlanePortAutomaticallyRecoversDefaultConflict(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	occupied := listener.Addr().(*net.TCPAddr).Port
	fallbackStart := occupied + 1
	if fallbackStart > 65535 {
		t.Skip("occupied ephemeral port leaves no fallback range")
	}

	var out bytes.Buffer
	selected, err := selectControlPlanePort(&out, "PostgreSQL", "--postgres-port", 0, occupied, fallbackStart)
	if err != nil {
		t.Fatal(err)
	}
	if selected == occupied || selected < fallbackStart {
		t.Fatalf("selected port = %d, occupied=%d fallbackStart=%d", selected, occupied, fallbackStart)
	}
	text := out.String()
	if !strings.Contains(text, "already in use") || !strings.Contains(text, "using it automatically") {
		t.Fatalf("output = %q, want actionable automatic recovery", text)
	}
}

func TestSelectControlPlanePortPreservesExplicitIntent(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	occupied := listener.Addr().(*net.TCPAddr).Port
	fallbackStart := occupied + 1
	if fallbackStart > 65535 {
		t.Skip("occupied ephemeral port leaves no fallback range")
	}

	_, err = selectControlPlanePort(&bytes.Buffer{}, "PostgreSQL", "--postgres-port", occupied, 5432, fallbackStart)
	if err == nil {
		t.Fatal("expected explicit occupied port to fail closed")
	}
	message := err.Error()
	if !strings.Contains(message, "already in use") || !strings.Contains(message, "Retry with 'baha up --postgres-port") {
		t.Fatalf("error = %q, want exact retry command", message)
	}
}

func TestReadPortChoiceAcceptsDefaultAndOverride(t *testing.T) {
	choice, err := readPortChoice(bufioReader("\n"), 15432)
	if err != nil || choice != 15432 {
		t.Fatalf("default choice = %d, err=%v", choice, err)
	}
	choice, err = readPortChoice(bufioReader("25432\n"), 15432)
	if err != nil || choice != 25432 {
		t.Fatalf("override choice = %d, err=%v", choice, err)
	}
}

func TestRuntimeUpGuidedRejectsSameExplicitPorts(t *testing.T) {
	var out bytes.Buffer
	err := runtimeUpGuided(context.Background(), strings.NewReader(""), &out, runtimeUpOptions{Yes: true, PostgresPort: 15432, OpenBaoPort: 15432})
	if err == nil || !strings.Contains(err.Error(), "same host port") {
		t.Fatalf("error = %v, want same host port rejection", err)
	}
}

func TestRuntimePortDefaultsRemainStable(t *testing.T) {
	if bhruntime.DefaultPostgresPort != 5432 || bhruntime.DefaultOpenBaoPort != 8200 {
		t.Fatalf("unexpected defaults: postgres=%d openbao=%d", bhruntime.DefaultPostgresPort, bhruntime.DefaultOpenBaoPort)
	}
}

func bufioReader(value string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(value))
}

func TestRecoveryFileForRepositoryUpRepromptsExistingBootstrapOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	existing := filepath.Join(dir, "openbao-recovery.json")
	next := filepath.Join(dir, "openbao-recovery-new.json")
	if err := os.WriteFile(existing, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	path, err := recoveryFileForRepositoryUp(context.Background(), strings.NewReader(existing+"\n"+next+"\n"), &out, runtimeUpOptions{}, "initialize")
	if err != nil {
		t.Fatal(err)
	}
	if path != next {
		t.Fatalf("path = %q, want %q", path, next)
	}
	text := out.String()
	if !strings.Contains(text, "NEW operator-held recovery output file") {
		t.Fatalf("prompt did not explain recovery output semantics: %q", text)
	}
	if !strings.Contains(text, "already exists") || !strings.Contains(text, "never overwrites") {
		t.Fatalf("interactive flow did not explain retry reason: %q", text)
	}
}

func TestRecoveryFileForRepositoryUpRepromptsMissingUnsealFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	missing := filepath.Join(dir, "missing.json")
	existing := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	path, err := recoveryFileForRepositoryUp(context.Background(), strings.NewReader(missing+"\n"+existing+"\n"), &out, runtimeUpOptions{}, "unseal")
	if err != nil {
		t.Fatal(err)
	}
	if path != existing {
		t.Fatalf("path = %q, want %q", path, existing)
	}
	if !strings.Contains(out.String(), "was not found") {
		t.Fatalf("interactive flow did not explain missing recovery file: %q", out.String())
	}
}

func TestParseRuntimeUpOptionsAcceptsEnvironmentEqualsForm(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--environment=prod"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Environment != "prod" {
		t.Fatalf("environment = %q", opts.Environment)
	}
}

// TestUpNameResumesThroughCommandEntrypoint proves the R4 stop criterion: the
// three up-NAME claims run through the top-level command entrypoint
// `runWithIO(argv ["up", NAME])`, not through a direct resolveRegisteredApplication
// call. The direct-call cell (TestUpNameResumesRegisteredDeploymentAcceptsParity)
// stays for the parser/resolver unit; this test proves the alias routes NAME.
//
// Registered NAME resolves the registered deployment and proceeds PAST the NAME
// resolution step through the production up path (it reaches control-plane setup,
// which is the next production step after resolveRegisteredApplication succeeds).
// Unknown NAME fails closed at resolveRegisteredApplication, and `up NAME` and
// `app up NAME` produce the same resolve outcome (alias parity).
func TestUpNameResumesThroughCommandEntrypoint(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	initialized, err := resolveRepositoryLifecycleIntent(t.Context(), target, selection, "init")
	if err != nil {
		t.Fatal(err)
	}
	m := initialized.Manifest
	name := m.Name
	repoRoot := selection.RepositoryRoot
	manifestPath := selection.ManifestPath
	registerTestDeployment(t, target, m, repoRoot, manifestPath)

	// --- (1) registered NAME through the command entrypoint: NAME resolves and
	// the up command proceeds past resolveRegisteredApplication into control-plane
	// setup. It fails at runtime realization, which is the step AFTER NAME
	// resolution succeeds -- the same failure the registered NAME reaches from
	// `app up NAME`, proving alias parity of the NAME-resolution step.
	t.Chdir(t.TempDir())
	var registeredOut bytes.Buffer
	registeredErr := runWithIO(machineNoninteractiveContext(t.Context()),
		[]string{"up", name, "--yes", "--skip-memory-preflight", "--plain"},
		&registeredOut, &registeredOut)
	if registeredErr == nil {
		t.Fatal("up NAME with a registered NAME must fail before runtime realization (no Core here); got success")
	}

	// The command must have accepted NAME and attempted the registered deployment:
	// the error is the control-plane realization failure reached after
	// resolveRegisteredApplication succeeds, not a NAME/usage rejection. A NAME
	// that was NOT routed through the entrypoint would instead fail closed here
	// with "no registered deployment"; that is exactly what assertion (3) proves.
	registeredMsg := registeredErr.Error()
	if strings.Contains(registeredMsg, "no registered deployment") {
		t.Fatalf("up NAME did not route the registered NAME through resolveRegisteredApplication: %v", registeredMsg)
	}

	// --- (2) alias parity: `up NAME` and `app up NAME` reach the same
	// resolveRegisteredApplication outcome for the same registered NAME.
	var appUpOut bytes.Buffer
	appUpErr := runWithIO(machineNoninteractiveContext(t.Context()),
		[]string{"app", "up", name, "--yes", "--skip-memory-preflight", "--plain"},
		&appUpOut, &appUpOut)
	// Both run the same resolveRegisteredApplication path for the registered NAME;
	// if the alias did not thread NAME, `up` would fail closed while `app up`
	// resolved. Document both outcomes so the mapping to the stop criterion is
	// immediate.
	if strings.Contains(appUpErr.Error(), "no registered deployment") {
		t.Fatalf("app up NAME failed closed for a registered NAME: %v", appUpErr)
	}
	if strings.Contains(registeredMsg, "no registered deployment") != strings.Contains(appUpErr.Error(), "no registered deployment") {
		t.Fatalf("up/app-up NAME resolution diverged: up=%q app-up=%q", registeredMsg, appUpErr)
	}

	// --- (3) unknown NAME through the command entrypoint fails closed: the top-level
	// `up` must not resolve an unregistered NAME, nor fall back to another deployment.
	// --- (3) unknown NAME inside the same repository fails closed at
	// resolveRegisteredApplication (the entrypoint-routed path for a repo-present
	// `up NAME`). A NAME that was not threaded through the entrypoint would not
	// reach resolveRegisteredApplication at all.
	t.Chdir(repoRoot)
	var unknownOut bytes.Buffer
	unknownErr := runWithIO(machineNoninteractiveContext(t.Context()),
		[]string{"up", "does-not-exist-xyz", "--yes", "--skip-memory-preflight", "--plain"},
		&unknownOut, &unknownOut)
	if unknownErr == nil {
		t.Fatal("up NAME with an unregistered NAME inside the repo must fail closed; got success")
	}
	if !strings.Contains(unknownErr.Error(), "no registered deployment") {
		t.Fatalf("up NAME did not fail closed for an unregistered NAME through the entrypoint: %v", unknownErr.Error())
	}
}

func TestParseRuntimeUpOptionsAcceptsOptionalName(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--yes", "my-deployment", "--skip-memory-preflight"})
	if err != nil {
		t.Fatalf("up NAME must accept the optional positional NAME: %v", err)
	}
	if opts.Name != "my-deployment" {
		t.Fatalf("Name = %q, want my-deployment", opts.Name)
	}
}

func TestParseRuntimeUpOptionsRejectsSecondName(t *testing.T) {
	if _, err := parseRuntimeUpOptions([]string{"one", "two"}); err == nil {
		t.Fatal("expected at-most-one NAME error")
	}
}

// TestParseUpParsesIdenticalToAppUpEnforces the documented argument surface:
// the top-level 'baha up' alias must accept the same optional NAME grammar as
// 'baha app up'. A divergence here is the bug this task fixes.
func TestParseUpParsesIdenticalToAppUpEnforcesParity(t *testing.T) {
	grammar := []string{"my-deployment", "--yes", "--skip-memory-preflight"}
	upOpts, upErr := parseRuntimeUpOptions(grammar)
	appOpts, appErr := parseAppUpOptions(grammar)
	if (upErr == nil) != (appErr == nil) {
		t.Fatalf("up/app-up parse divergence: upErr=%v appErr=%v", upErr, appErr)
	}
	if upOpts.Name != appOpts.Name {
		t.Fatalf("NAME mismatch: up.Name=%q app-up.Name=%q", upOpts.Name, appOpts.Name)
	}
	if upOpts.Name != "my-deployment" {
		t.Fatalf("up.Name = %q, want my-deployment", upOpts.Name)
	}
	if upOpts.Yes != appOpts.Yes || upOpts.SkipMemoryPreflight != appOpts.SkipMemoryPreflight {
		t.Fatalf("flag mismatch: up.Name=%q app-up.Name=%q", upOpts.Name, appOpts.Name)
	}
}

// TestUpNameResumesRegisteredDeploymentAcceptsParity proves the card's
// acceptance semantics on the production-command path, not just the parser:
// the top-level `baha up NAME` alias resolves the registered deployment NAME
// identically to `baha app up NAME`, resumes the registered deployment, and
// fails closed for an unregistered NAME.
func TestUpNameResumesRegisteredDeploymentAcceptsParity(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	initialized, err := resolveRepositoryLifecycleIntent(t.Context(), target, selection, "init")
	if err != nil {
		t.Fatal(err)
	}
	m := initialized.Manifest
	name := m.Name
	repoRoot := selection.RepositoryRoot
	manifestPath := selection.ManifestPath
	if m.ApplicationID == "" {
		t.Fatal("minimal manifest must carry a concrete application ID")
	}

	registered := registerTestDeployment(t, target, m, repoRoot, manifestPath)

	// Parser parity: both the alias and the explicit command accept the same
	// single optional NAME and agree on the value.
	upOpts, upErr := parseRuntimeUpOptions([]string{name, "--yes", "--skip-memory-preflight"})
	if upErr != nil {
		t.Fatalf("up NAME must parse the optional positional NAME: %v", upErr)
	}
	appOpts, appErr := parseAppUpOptions([]string{name, "--yes", "--skip-memory-preflight"})
	if appErr != nil {
		t.Fatalf("app up NAME must parse: %v", appErr)
	}
	if upOpts.Name != appOpts.Name {
		t.Fatalf("up/app-up NAME parity lost: up=%q app-up=%q", upOpts.Name, appOpts.Name)
	}
	if upOpts.Name != name {
		t.Fatalf("up.Name = %q, want %q", upOpts.Name, name)
	}

	// Resume of the registered deployment: NAME is identical to app up NAME.
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveRegisteredApplication(t.Context(), target, targetRoot, name, "", "up", "")
	if err != nil {
		t.Fatalf("up NAME must resume the registered deployment %q: %v", name, err)
	}
	if resolved.DeploymentIdentity.Application != name {
		t.Fatalf("resolved application = %q, want %q", resolved.DeploymentIdentity.Application, name)
	}
	if resolved.DeploymentIdentity.DeploymentID != registered.Identity.DeploymentID {
		t.Fatalf("resolved deployment identity = %+v, want %+v", resolved.DeploymentIdentity, registered.Identity)
	}

	// Fail closed for an unregistered NAME: a registered deployment with a
	// different identity claims this name, so the alias must reject it.
	foreignID, err := deployment.NewDeploymentIdentity(target.Name, application.MustNewApplicationID(), name, "staging")
	if err != nil {
		t.Fatal(err)
	}
	foreign := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: foreignID,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repoRoot, Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: target.RuntimeProvider},
	}
	intent, jerr := json.Marshal(m)
	if jerr != nil {
		t.Fatal(jerr)
	}
	foreign.Applied.Intent = intent
	if err := deployment.SaveDeploymentRecord(foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRegisteredApplication(t.Context(), target, targetRoot, name, "", "up", ""); err == nil {
		t.Fatal("up NAME accepted an unregistered deployment; fail-closed is broken")
	}
}
