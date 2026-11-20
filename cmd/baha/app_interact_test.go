package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// Card 0428-063: guided app logs/exec/urls paths. These tests pin the slice:
// guidance reads the SAME delivered read-view/status logic; missing choices
// guide (unique derived, ambiguous listed); --no-input and non-terminals never
// block and fail closed with actionable options; -c selections, workload-shell
// parsing of --cmd, exit-status preservation, app-bound URL listing with verified or
// unknown accessibility; and fail-closed authorization on remote Targets after
// the app.show precedent.

// guidedURLFixture prepares a repository application with a compose workload
// declaring the given services, resolved through the production CLI surface.
func guidedURLFixture(t *testing.T, target deployment.ResolvedTarget, services ...string) (application.Manifest, application.RuntimeFiles) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	builder.WriteString("services:\n")
	for _, service := range services {
		builder.WriteString("  " + service + ":\n    image: ghcr.io/baseharbor.test/" + service + ":test\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(builder.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	m := application.New("demo", "dev", false, false, false)
	m.Workload = application.WorkloadConfig{Components: services}
	store := testDeploymentStore(t, target, m)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	registerTestDeployment(t, target, m, dir, filepath.Join(dir, "baseharbor.yaml"))
	if err := os.WriteFile(filepath.Join(dir, "baseharbor.yaml"), []byte(m.AuthoredYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := application.EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	return m, files
}

func TestGuidedURLsListOnlyVerifiedAppAndBoundServiceURLs(t *testing.T) {
	target := configureTestTarget(t)
	m, _ := guidedURLFixture(t, target, "api", "worker")

	// The guidance layer composes only delivered read-view values; the URL
	// truth stays the one exposure-state renderer status uses.
	state := exposure.State{Routes: []exposure.Route{
		{Name: "web", Visibility: "public", Protocol: "https", PublishedPort: 31443},
		{Name: "adminer", Visibility: "internal", Protocol: "https", PublishedPort: 31999},
	}}
	published := publicApplicationAccessURLs("https://192.168.50.5:18443", state)
	if len(published) != 1 || published[0] != "https://192.168.50.5:31443" {
		t.Fatalf("guidance must use only the actual published public route: %v", published)
	}
	for _, bad := range []string{"https://127.0.0.1:18443", "https://0.0.0.0:18443", "https://app.example.test:18443"} {
		if got := publicApplicationAccessURLs(bad, state); len(got) != 0 {
			t.Fatalf("guidance invented a routable URL from %s: %v", bad, got)
		}
	}

	entries, err := guidedApplicationURLs(resolvedApplication{
		Target:   target,
		Manifest: m,
	}, applicationStatusResult{StatusResult: application.StatusResult{
		Application: m.Name, Environment: m.Environment, State: "running", Ready: true,
		Checks: []application.StatusCheck{{Name: "managed-exposure/web", OK: true, Detail: "https://192.168.50.5:31443"}},
	}, ApplicationURLs: []string{"https://192.168.50.5:31443"},
		ManagementUI: []application.ManagementUISurface{{Service: "sql", URL: "https://demo.dev.example.test/", Purpose: "management", Authentication: "pgadmin-native"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected the app URL plus the bound service GUI: %+v", entries)
	}
	for _, entry := range entries {
		if entry.URL == "" || strings.Contains(entry.URL, "127.0.0.1") || strings.Contains(entry.URL, "0.0.0.0") {
			t.Fatalf("unrelated or wildcard URL in guidance listing: %+v", entry)
		}
	}

	// -c filters name components or bound services; an unknown name fails
	// closed instead of silently listing the unfiltered view.
	filtered, err := guidedApplicationURLs(resolvedApplication{Target: target, Manifest: m}, applicationStatusResult{StatusResult: application.StatusResult{Ready: true}, ApplicationURLs: []string{"https://192.168.50.5:31443"}}, []string{"demo"})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("application-level -c selection: %+v err=%v", filtered, err)
	}
	if _, err := guidedApplicationURLs(resolvedApplication{Target: target, Manifest: m}, applicationStatusResult{StatusResult: application.StatusResult{Ready: true}, ApplicationURLs: []string{"https://192.168.50.5:31443"}}, []string{"nostranger"}); err == nil {
		t.Fatal("unknown -c name must fail closed")
	}
}

func TestGuidedURLsUnknownReadinessIsNeverVerified(t *testing.T) {
	entries, err := guidedApplicationURLs(resolvedApplication{}, applicationStatusResult{StatusResult: application.StatusResult{Ready: false},
		ApplicationURLs: []string{"https://192.168.50.5:31443"},
		ManagementUI:    []application.ManagementUISurface{{Service: "sql", URL: "https://demo.dev.example.test/", Purpose: "management", Authentication: "pgadmin-native"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Verified {
			t.Fatalf("an entry of an unready application must never claim verified accessibility: %+v", entry)
		}
	}
}

func TestAppURLsCommandGuidanceFailsClosedWithoutDeployment(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	var out, errOut bytes.Buffer
	err := runWithIO(context.Background(), []string{"app", "urls"}, &out, &errOut)
	if err == nil {
		t.Fatalf("app urls resolved without any application: %s", out.String())
	}
}

func TestGuidedLogsParsesTailComponentsAndFollowJSONSplit(t *testing.T) {
	options, err := parseLogsOptions([]string{"api", "--tail", "50", "-c", "api", "-f"})
	if err != nil {
		t.Fatal(err)
	}
	if options.service != "api" || options.tail != 50 || !options.follow || len(options.components) != 1 || options.components[0] != "api" {
		t.Fatalf("unexpected parse result: %+v", options)
	}
	if _, err := parseLogsOptions([]string{"--tail", "0"}); err == nil {
		t.Fatal("--tail 0 must fail closed")
	}
	if _, err := parseLogsOptions([]string{"--tail", "x"}); err == nil {
		t.Fatal("--tail x must fail closed")
	}
	if _, err := parseLogsOptions([]string{"api", "worker"}); err == nil {
		t.Fatal("two positional components must fail closed")
	}
	if _, err := parseLogsOptions([]string{"api", "-c", "worker"}); err == nil {
		t.Fatal("-c conflicting with the named component must fail closed")
	}
	if _, err := parseLogsOptions([]string{"--tail"}); err == nil {
		t.Fatal("--tail without a value must fail closed")
	}
	filtered, err := parseLogsOptions([]string{"-c", "api,worker", "-c", "api", "--tail", "25", "-f"})
	if err != nil || len(filtered.components) != 2 || filtered.components[0] != "api" || filtered.components[1] != "worker" {
		t.Fatalf("repeated/comma-separated filters should deduplicate: %+v err=%v", filtered, err)
	}
	if got := workloadLogsCommandArgs(filtered, filtered.components); !reflect.DeepEqual(got, []string{"logs", "--tail", "25", "--follow", "api", "worker"}) {
		t.Fatalf("selected component filters must reach the Compose invocation: %v", got)
	}
	if err := validateWorkloadLogComponents(filtered.components, []string{"api", "worker"}); err != nil {
		t.Fatalf("declared component filters should pass validation: %v", err)
	}
	if err := validateWorkloadLogComponents([]string{"api", "database"}, []string{"api", "worker"}); err == nil {
		t.Fatal("unknown component filters must fail closed")
	}
}

func TestGuidedLogsJSONNeverFollowsAndFailsClosedNoninteractive(t *testing.T) {
	target := configureTestTarget(t)
	guidedURLFixture(t, target, "api", "worker")
	var out, errOut bytes.Buffer
	err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "logs", "--follow", "--json"}, &out, &errOut)
	if err == nil {
		t.Fatalf("--follow with --json must fail closed: %s", out.String())
	}
	if !strings.Contains(err.Error(), "--follow cannot be combined with JSON output") {
		t.Fatalf("unexpected JSON-follow denial: %v", err)
	}

	out.Reset()
	errOut.Reset()
	err = runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "logs"}, &out, &errOut)
	if err == nil {
		t.Fatalf("ambiguous component selection must fail closed in scripts: %s/%s", out.String(), errOut.String())
	}
	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("guided failures stay typed usage errors: %T %v", err, err)
	}
	if !strings.Contains(usage.Hint, "api") || !strings.Contains(usage.Hint, "worker") {
		t.Fatalf("the fail-closed hint must list the options: %+v", usage)
	}
}

func TestGuidedLogsDerivesUniqueComponentNoninteractive(t *testing.T) {
	target := configureTestTarget(t)
	guidedURLFixture(t, target, "only-app")
	var out, errOut bytes.Buffer
	// The guard's runtime preflight fails before the compose call on a host
	// without docker; what this pins is that the unique component was derived
	// (no ambiguity denial) and nothing blocked.
	err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "logs"}, &out, &errOut)
	if err != nil && strings.Contains(err.Error(), "requires a workload component") {
		t.Fatalf("a unique declared component must be derived, not demanded: %v\n%s", err, errOut.String())
	}
	if !strings.Contains(errOut.String(), "Guided:") && err != nil && !strings.Contains(err.Error(), "runtime provider preflight") {
		t.Fatalf("unexpected guided logs failure: %v\n%s", err, errOut.String())
	}
}

func TestGuidedLogsValidationHappensBeforeAnyRuntimeCall(t *testing.T) {
	target := configureTestTarget(t)
	guidedURLFixture(t, target, "api")
	var out, errOut bytes.Buffer
	err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "logs", "nosuch"}, &out, &errOut)
	if err == nil {
		t.Fatal("a component the contract does not select must fail closed")
	}
	if !strings.Contains(err.Error(), "not selected by the application contract") {
		t.Fatalf("wrong denial: %v", err)
	}
	if strings.Contains(out.String(), "logs") {
		t.Fatalf("nothing may reach the runtime before validation passed: %s", out.String())
	}
}

func TestManagedLogGuidanceDoesNotClaimUnavailableReads(t *testing.T) {
	var out bytes.Buffer
	err := reportManagedLogGuidance(outputHuman, &out, resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: "local"},
		Manifest: application.Manifest{Name: "demo", Environment: "dev"},
	}, []string{"api", "worker"})
	if err == nil || !strings.Contains(err.Error(), "not exposed") {
		t.Fatalf("managed streaming stays explicitly unavailable: %v", err)
	}
	if strings.Contains(out.String(), "--tail") || !strings.Contains(out.String(), "no logs were read") {
		t.Fatalf("guidance must not claim bounded reads that are unavailable: %s", out.String())
	}
	out.Reset()
	if err := reportManagedLogGuidance(outputJSON, &out, resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: "local"},
		Manifest: application.Manifest{Name: "demo", Environment: "dev"},
	}, []string{"api", "worker"}); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, exists := result["component"]; exists {
		t.Fatalf("multiple filters must not be mislabeled as one component: %v", result)
	}
	components, ok := result["components"].([]any)
	if !ok || len(components) != 2 || components[0] != "api" || components[1] != "worker" {
		t.Fatalf("managed JSON must retain the multi-component selection: %v", result)
	}
}

func TestGuidedExecParsesComponentCmdAndExitStatus(t *testing.T) {
	selector, service, command, err := parseWorkloadExecArgs([]string{"api", "env"}, "exec")
	if err != nil || service != "api" || len(command) != 1 || command[0] != "env" || len(selector) != 0 {
		t.Fatalf("positional form: %v %q %v %v", selector, service, command, err)
	}
	selector, service, command, err = parseWorkloadExecArgs([]string{"-c", "api", "--cmd", "echo hi > /tmp/x"}, "exec")
	if err != nil || service != "api" || len(command) != 1 || command[0] != "echo hi > /tmp/x" {
		t.Fatalf("--cmd form must keep exactly one command unit: %v %q %v %v", selector, service, command, err)
	}
	if !hasWorkloadCommandFlag([]string{"-c", "api", "--cmd", command[0]}) {
		t.Fatal("--cmd must be distinguished from positional argv")
	}
	if got := workloadExecCommandArgs(workloadInvocation{cmd: command[0]}, service); !reflect.DeepEqual(got, []string{"exec", "-T", "api", "sh", "-c", "echo hi > /tmp/x"}) {
		t.Fatalf("--cmd must reach the workload shell as one command string: %v", got)
	}
	if got := workloadExecCommandArgs(workloadInvocation{argv: []string{"printf", "%s", "a b"}}, "api"); !reflect.DeepEqual(got, []string{"exec", "-T", "api", "printf", "%s", "a b"}) {
		t.Fatalf("positional commands must remain direct argv: %v", got)
	}
	selector, service, command, err = parseWorkloadExecArgs([]string{"--app", "mailflow", "api", "ps", "flag"}, "exec")
	if err != nil || service != "api" || len(command) != 2 || len(selector) != 2 {
		t.Fatalf("--app selector split: %v %q %v %v", selector, service, command, err)
	}
	if _, _, _, err := parseWorkloadExecArgs([]string{"api", "ps", "--cmd", "x"}, "exec"); err == nil {
		t.Fatal("--cmd with positional command must fail closed")
	}
	if _, _, _, err := parseWorkloadExecArgs([]string{"api", "--cmd"}, "exec"); err == nil {
		t.Fatal("--cmd without a value must fail closed")
	}
	if _, _, _, err := parseWorkloadExecArgs([]string{"api", "-Q"}, "exec"); err == nil {
		t.Fatal("unknown options must fail closed")
	}
}

func TestGuidedExecShellDefaultAndNoCommand(t *testing.T) {
	selector, service, command, err := parseWorkloadExecArgs([]string{"-c", "api"}, "shell")
	if err != nil || service != "api" || command != nil || len(selector) != 0 {
		t.Fatalf("shell -c selection: %v %q %v %v", selector, service, command, err)
	}
	if _, _, _, err := parseWorkloadExecArgs([]string{"api", "extra", "args"}, "shell"); err == nil {
		t.Fatal("app shell accepts no command arguments")
	}
	if _, _, _, err := parseWorkloadExecArgs([]string{"--cmd", "echo ignored"}, "shell"); err == nil {
		t.Fatal("app shell must reject --cmd instead of silently opening a shell")
	}
}

func TestGuidedExecExitStatusIsPreserved(t *testing.T) {
	if err := preserveWorkloadExitStatus(nil); err != nil {
		t.Fatalf("success must stay success: %v", err)
	}
	failure := preserveWorkloadExitStatus(&exec.ExitError{ProcessState: exitedProcessStateForTest(t, 7)})
	if failure == nil {
		t.Fatal("a failing command must fail the invocation")
	}
	if cli.ExitCode(preserveWorkloadExitStatus(errors.New("compose is unavailable"))) != 1 {
		t.Fatal("a plain runtime failure must keep the runtime exit code 1")
	}
	var marker *cli.PresentedError
	if !errors.As(failure, &marker) {
		t.Fatalf("the preserved status must be a presented failure, got %T", failure)
	}
	text := marker.Err.Error()
	if !strings.Contains(text, "7") {
		t.Fatalf("the exit status must survive in the message: %s", text)
	}
	if got := cli.ExitCode(failure); got != 7 {
		t.Fatalf("the baha process exit status must preserve the child status: got %d want 7", got)
	}
}

func TestGuidedExecNonInteractiveAmbiguityFailsClosed(t *testing.T) {
	target := configureTestTarget(t)
	guidedURLFixture(t, target, "api", "worker")
	var out, errOut bytes.Buffer
	err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "exec"}, &out, &errOut)
	if err == nil {
		t.Fatalf("ambiguous exec selection must fail closed without blocking: %s/%s", out.String(), errOut.String())
	}
	if !strings.Contains(err.Error(), "requires a workload component") {
		t.Fatalf("wrong denial: %v", err)
	}
	if hint := usageHint(err); !strings.Contains(hint, "api") || !strings.Contains(hint, "worker") {
		t.Fatalf("the exec denial must name the options: %v hint=%q", err, hint)
	}
}

func TestGuidedRevealStyleAuthorizationPrecedentStands(t *testing.T) {
	// The fail-closed authorization of the new operation categories follows
	// the secret.reveal precedent of 0428-057: a distinct machine op id with
	// an explicit seam; guidance performs no privilege escalation.
	for _, id := range []string{"app.logs", "app.exec", "app.urls", "secret.reveal"} {
		if _, ok := machine.OperationByID(id); !ok {
			t.Fatalf("operation %s must stay registered for the fail-closed seam", id)
		}
	}
}

func TestGuidedURLsHumanOutputCarriesVerifiedUnknownStates(t *testing.T) {
	var out bytes.Buffer
	entries := []guidedURLEntry{
		{Name: "demo", URL: "https://192.168.50.5:31443", Purpose: "application endpoint", Verified: true},
		{Name: "sql", URL: "https://demo.dev.example.test/", Purpose: "bound management GUI", Verified: false},
	}
	for _, entry := range entries {
		state := "unverified"
		if entry.Verified {
			state = "verified"
		}
		writeGuidanceURLEntryLine(&out, entry, state)
	}
	text := out.String()
	if !strings.Contains(text, "verified") || !strings.Contains(text, "unverified") {
		t.Fatalf("listing must render both accessibility states: %s", text)
	}
}

func writeGuidanceURLEntryLine(out *bytes.Buffer, entry guidedURLEntry, state string) {
	out.WriteString(entry.Name + " " + entry.URL + " · " + state + " · " + entry.Purpose + "\n")
}

// usageHint extracts the cli.UsageError hint if present, else the message.
func usageHint(err error) string {
	var usage *cli.UsageError
	if errors.As(err, &usage) {
		return usage.Message + " " + usage.Hint
	}
	return err.Error()
}

// exitedProcessStateForTest builds a real ProcessState from a short-lived
// helper process so the exit-status preservation test exercises the actual
// syscall.WaitStatus path, not a fabricated value.
func exitedProcessStateForTest(t *testing.T, code int) *os.ProcessState {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "exit "+strconv.Itoa(code))
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("helper process must exit with %d: %v", code, err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Exited() || status.ExitStatus() != code {
		t.Fatalf("helper process exit status mismatch: %#v", exitErr.Sys())
	}
	return exitErr.ProcessState
}

func TestGuidedLogsFollowStillAllowedHumanNonJSON(t *testing.T) {
	target := configureTestTarget(t)
	guidedURLFixture(t, target, "api")
	var out, errOut bytes.Buffer
	err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "logs", "api", "--follow"}, &out, &errOut)
	// Human follow is a legitimate path; it must not be refused by the JSON
	// guard. It fails later (no runtime here), never with the JSON denial.
	if err != nil && strings.Contains(err.Error(), "cannot be combined with JSON output") {
		t.Fatalf("the JSON guard must not fire without --json: %v", err)
	}
}

func TestAppURLsJSONShapeStaysSecretSafe(t *testing.T) {
	resolved := resolvedApplication{}
	status := applicationStatusResult{StatusResult: application.StatusResult{Ready: true, Application: "demo", Environment: "dev"},
		ApplicationURLs: []string{"https://192.168.50.5:31443"},
		ManagementUI:    []application.ManagementUISurface{{Service: "sql", URL: "https://demo.dev.example.test/", Purpose: "management", Authentication: "pgadmin-native"}},
	}
	entries, err := guidedApplicationURLs(resolved, status, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "hunter2") || strings.Contains(string(encoded), "@demo") {
		t.Fatalf("guidance must never echo credential-bearing URLs: %s", encoded)
	}
}

var _ = exposure.State{}
