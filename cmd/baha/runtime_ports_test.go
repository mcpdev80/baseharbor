package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRecoveryFileForRepositoryUpRequiresExplicitPathNonInteractive(t *testing.T) {
	var out bytes.Buffer
	_, err := recoveryFileForRepositoryUp(context.Background(), strings.NewReader(""), &out, runtimeUpOptions{Yes: true}, "initialize")
	if err == nil || !strings.Contains(err.Error(), "recovery file") {
		t.Fatalf("error = %v, want actionable recovery-file failure", err)
	}
}

func TestRecoveryFileForRepositoryUpInteractivePromptIsVisible(t *testing.T) {
	dir := t.TempDir()
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
	if !strings.Contains(text, "Where should BaseHarbor create the new recovery file?") {
		t.Fatalf("missing explicit recovery question: %q", text)
	}
	if !strings.Contains(text, "OpenBao-recovery-key:") {
		t.Fatalf("missing shell-style recovery prompt: %q", text)
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
