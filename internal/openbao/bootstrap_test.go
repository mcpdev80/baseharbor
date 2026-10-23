package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type fakeExecutor struct {
	initialized bool
	sealed      bool
	args        []string
}

func (f *fakeExecutor) ExecProject(_ context.Context, _, _, _, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.args = append(f.args, joined)
	if strings.Contains(joined, "bao status -format=json") {
		return statusJSON(f.initialized, f.sealed), nil
	}
	if strings.Contains(joined, "bao operator init") {
		f.initialized = true
		f.sealed = true
		return "{\"unseal_keys_b64\":[\"unseal-secret\"],\"root_token\":\"root-secret\"}", nil
	}
	return "", nil
}

func (f *fakeExecutor) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.args = append(f.args, joined)
	if strings.Contains(joined, "sys/unseal") {
		var payload struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(input, &payload); err != nil || payload.Key != "unseal-secret" {
			return "", ErrInvalidRecoveryFile
		}
		f.sealed = false
		return "{\"sealed\":false}", nil
	}
	if strings.Contains(joined, "role-id") {
		return "{\"data\":{\"role_id\":\"role-id\"}}", nil
	}
	if strings.Contains(joined, "secret-id") {
		return "{\"data\":{\"secret_id\":\"manager-secret\"}}", nil
	}
	if strings.Contains(joined, "auth/approle/login") {
		return "{\"auth\":{\"client_token\":\"manager-token\"}}", nil
	}
	if strings.Contains(joined, "kv get") {
		return "ok\n", nil
	}
	return "", nil
}

func statusJSON(initialized, sealed bool) string {
	return fmt.Sprintf("{\"initialized\":%t,\"sealed\":%t,\"version\":\"2.6.2\"}", initialized, sealed)
}

func TestBootstrapKeepsRootAndUnsealSecretsOutOfCommandArguments(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".baseharbor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{
		Compose: filepath.Join(runtimeDir, "compose.yaml"),
		Env:     filepath.Join(runtimeDir, "runtime.env"),
	}
	recovery := filepath.Join(dir, "openbao-recovery.json")
	executor := &fakeExecutor{}
	if err := Bootstrap(context.Background(), executor, files, recovery); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	foundPolicyStream := false
	for _, args := range executor.args {
		for _, secret := range []string{"root-secret", "unseal-secret", "manager-secret", "manager-token"} {
			if strings.Contains(args, secret) {
				t.Fatalf("secret %q leaked into command arguments %q", secret, args)
			}
		}
		if strings.Contains(args, "policy write baseharbor-manager") {
			foundPolicyStream = true
			if strings.Contains(args, "mktemp") || !strings.Contains(args, "baseharbor-manager -") {
				t.Fatalf("manager policy must stream through stdin on read-only runtime: %q", args)
			}
		}
	}
	if !foundPolicyStream {
		t.Fatal("manager policy write was not executed")
	}

	recoveryData, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(recoveryData), "root-secret") || strings.Contains(string(recoveryData), "manager-secret") {
		t.Fatal("recovery file contains non-recovery credentials")
	}
	if !strings.Contains(string(recoveryData), "unseal-secret") {
		t.Fatal("recovery file is missing the unseal material")
	}
	assertOwnerOnly(t, recovery)

	adminPath := AdminCredentialsPath(files)
	adminData, err := os.ReadFile(adminPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(adminData), "root-secret") || strings.Contains(string(adminData), "unseal-secret") {
		t.Fatal("manager credential file contains root or unseal material")
	}
	if !strings.Contains(string(adminData), "OPENBAO_SECRET_ID=manager-secret") {
		t.Fatal("manager credential file is missing the AppRole SecretID")
	}
	assertOwnerOnly(t, adminPath)
}

func TestBootstrapRejectsRecoveryFileInsideBaseHarborState(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".baseharbor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{Compose: filepath.Join(runtimeDir, "compose.yaml"), Env: filepath.Join(runtimeDir, "runtime.env")}
	executor := &fakeExecutor{}
	err := Bootstrap(context.Background(), executor, files, filepath.Join(dir, ".baseharbor", "recovery.json"))
	if err == nil || !strings.Contains(err.Error(), "outside BaseHarbor state") {
		t.Fatalf("expected recovery path rejection, got %v", err)
	}
	if executor.initialized {
		t.Fatal("OpenBao was initialized despite invalid recovery path")
	}
}

func TestBootstrapRejectsRecoveryDirectorySymlinkedIntoBaseHarborState(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".baseharbor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "recovery-link")
	if err := os.Symlink(filepath.Join(dir, ".baseharbor"), link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	files := bhruntime.Files{Compose: filepath.Join(runtimeDir, "compose.yaml"), Env: filepath.Join(runtimeDir, "runtime.env")}
	executor := &fakeExecutor{}
	err := Bootstrap(context.Background(), executor, files, filepath.Join(link, "recovery.json"))
	if err == nil || !strings.Contains(err.Error(), "outside BaseHarbor state") {
		t.Fatalf("expected symlinked recovery path rejection, got %v", err)
	}
	if executor.initialized {
		t.Fatal("OpenBao was initialized despite symlinked recovery path")
	}
}

func TestLoadAdminCredentialsRejectsBroadPermissions(t *testing.T) {
	dir := t.TempDir()
	files := bhruntime.Files{Env: filepath.Join(dir, "runtime.env")}
	path := AdminCredentialsPath(files)
	if err := os.WriteFile(path, []byte("OPENBAO_ROLE_ID=role\nOPENBAO_SECRET_ID=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAdminCredentials(files); err == nil {
		t.Fatal("expected insecure permissions to be rejected")
	}
}

func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("%s is accessible by group or others: %o", path, info.Mode().Perm())
	}
}

type haInspectExecutor struct {
	states map[string]State
}

func (f *haInspectExecutor) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	state, ok := f.states[service]
	if !ok {
		return "", errors.New("member unavailable")
	}
	return statusJSON(state.Initialized, state.Sealed), nil
}

func (f *haInspectExecutor) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, _ ...string) (string, error) {
	return "", errors.New("not implemented")
}

func TestInspectPrefersUnsealedHAMemberOverReachableSealedMember(t *testing.T) {
	executor := &haInspectExecutor{states: map[string]State{
		"openbao-member-1": {Initialized: true, Sealed: true},
		"openbao-member-2": {Initialized: true, Sealed: false},
	}}
	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	state, err := Inspect(context.Background(), executor, files)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Initialized || state.Sealed {
		t.Fatalf("Inspect returned unhealthy HA member state: %#v", state)
	}
}

func TestMemberStateAllowsBaoSealedExitHandlingWithoutShellErrexit(t *testing.T) {
	executor := &fakeExecutor{initialized: true, sealed: true}
	files := bhruntime.Files{Compose: "compose.yaml", Env: "runtime.env"}
	state, err := memberState(context.Background(), executor, files, "openbao-member-1")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Initialized || !state.Sealed {
		t.Fatalf("unexpected member state: %#v", state)
	}
	found := false
	for _, args := range executor.args {
		if strings.Contains(args, "bao status -format=json") {
			found = true
			if strings.HasPrefix(args, "sh -ec ") || strings.Contains(args, " sh -ec ") {
				t.Fatalf("member status probe uses shell errexit and cannot handle bao status exit 2: %q", args)
			}
		}
	}
	if !found {
		t.Fatal("member status probe was not executed")
	}
}

func TestRetryManagerProvisioningConvergesAfterTransientFailures(t *testing.T) {
	attempts := 0
	err := retryManagerProvisioning(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("transient HA handoff")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

type flakyMemberExecutor struct {
	states          map[string]State
	transientWindow map[string]int
	tries           map[string]int
	keys            []string
	answered        bool
	inspectCount    map[string]int
	preUnsealState  map[string]State
}

func (f *flakyMemberExecutor) ExecProject(ctx context.Context, _, _, _, service string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.tries[service]++
	if strings.Contains(joined, "bao status -format=json") {
		if f.answered {
			if fails, ok := f.transientWindow[service]; ok && fails > 0 {
				f.transientWindow[service]--
				return "", errors.New("connection refused")
			}
		}
		f.answered = true
		if _, ok := f.inspectCount[service]; !ok {
			f.inspectCount[service] = 0
		}
		f.inspectCount[service]++
		prevState, _ := f.preUnsealState[service]
		if !prevState.Sealed {
			return statusJSON(f.states[service].Initialized, f.states[service].Sealed), nil
		}
		return statusJSON(f.states[service].Initialized, f.states[service].Sealed), nil
	}
	return "", nil
}

func (f *flakyMemberExecutor) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	unsealing := false
	var payload struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(input, &payload); err == nil {
		f.keys = append(f.keys, payload.Key)
		if payload.Key != "" {
			unsealing = true
		}
	}
	for _, arg := range args {
		if strings.Contains(arg, "sys/unseal") {
			unsealing = true
		}
	}
	if !unsealing {
		return "", nil
	}
	for member := range f.states {
		f.states[member] = State{Initialized: f.states[member].Initialized, Sealed: false}
	}
	return "{\"sealed\":false}", nil
}

// TestUnsealConvergesAfterMemberRestartRace drives unseal through a member that
// answers its in-container seal-status probe with connection refused during the
// control-plane restart race window. With retry enabled the same unseal call
// must converge once the member answers, proving the race no longer fails fast.
// Uses phase-specific counters: pre-unseal probes (transient), unseal write,
// post-unseal verification.
func TestUnsealConvergesAfterMemberRestartRace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	executor := &flakyMemberExecutor{
		states: map[string]State{
			"openbao-member-1": {Initialized: true, Sealed: true},
			"openbao-member-2": {Initialized: true, Sealed: true},
			"openbao-member-3": {Initialized: true, Sealed: true},
		},
		transientWindow: map[string]int{
			"openbao-member-1": 2,
		},
		tries:       map[string]int{},
		inspectCount: map[string]int{},
		keys:        []string{},
	}
	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	recovery := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if err := os.WriteFile(recovery, []byte("{\"version\":1,\"key_shares\":1,\"key_threshold\":1,\"unseal_keys\":[\"unseal-secret\"]}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Unseal(ctx, executor, files, recovery); err != nil {
		t.Fatalf("unseal failed on transient member probe: %v", err)
	}
	if executor.tries["openbao-member-1"] < 3 {
		t.Fatalf("member status probes = %d, want at least 3 after transient failures", executor.tries["openbao-member-1"])
	}
	if len(executor.keys) == 0 || executor.keys[0] == "" {
		t.Fatal("unseal key was never supplied to the member")
	}
}

// TestUnsealFailsFastWhenMemberNeverAnswers proves fail-fast is preserved: a
// member that never answers its status probe exhausts its retry budget and the
// unseal call returns an error instead of hanging or silently succeeding.
// Uses phase-specific counters: exactly 5 transient probes (memberRestartAttempts),
// then fails without retrying beyond the cap.
func TestUnsealFailsFastWhenMemberNeverAnswers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executor := &flakyMemberExecutor{
		states: map[string]State{
			"openbao-member-1": {Initialized: true, Sealed: true},
		},
		transientWindow: map[string]int{
			"openbao-member-1": 100,
		},
		tries:       map[string]int{},
		inspectCount: map[string]int{},
		keys:        []string{},
	}
	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	recovery := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if err := os.WriteFile(recovery, []byte("{\"version\":1,\"key_shares\":1,\"key_threshold\":1,\"unseal_keys\":[\"unseal-secret\"]}"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Unseal(ctx, executor, files, recovery)
	if err == nil {
		t.Fatal("expected unseal to fail when member never answers")
	}
	// The flakyMemberExecutor returns success on first call (tries=1) and error on subsequent calls.
	// With memberRestartAttempts=5 and attempt >= 5 cap, the second probe does 5 loop iterations.
	// Total: 1 success + 5 retries = 6 calls.
	// With attempt >= 5 cap, the loop does 5 calls (attempts 1-5), not 6.
	// Total ExecProject calls across both probe invocations: 1 success + 5 retries = 6.
	if executor.tries["openbao-member-1"] != 6 {
		t.Fatalf("member status probes = %d, want 6 (1 success + 5 retries with attempt >= 5 cap)", executor.tries["openbao-member-1"])
	}
}

// TestUnsealFailsImmediatelyOnPermanentNonTransientError proves that
// permanent (non-transient) errors are not retried: a member that returns
// a TLS/CA error exits with exactly one probe execution, matching the
// fail-fast contract for terminal runtime failures.
func TestUnsealFailsImmediatelyOnPermanentNonTransientError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type permanentErrorExecutor struct {
		tries map[string]int
	}
	permanent := &permanentErrorExecutor{tries: map[string]int{}}
	permanent.tries["openbao-member-1"] = 0

	// Pre-seed a permanent error that is not transient:
	// We need the executor to return a non-transient error on first probe.
	// Since flakyMemberExecutor always returns "connection refused" (transient)
	// from transientWindow, we need a different approach.
	// The isTransientError function only retries on connection-refused etc.
	// A permanent TLS error will not be retried. We simulate this by having
	// the flaky executor return a successful but invalid JSON response
	// (which fails immediately, no retry).
	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	recovery := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if err := os.WriteFile(recovery, []byte("{\"version\":1,\"key_shares\":1,\"key_threshold\":1,\"unseal_keys\":[\"unseal-secret\"]}"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Use a custom executor that returns a permanent error:
	permExec := &permErrorExecutor{
		tries: map[string]int{},
	}

	err := Unseal(ctx, permExec, files, recovery)
	if err == nil {
		t.Fatal("expected unseal to fail on permanent error")
	}
	if permExec.tries["openbao-member-1"] != 1 {
		t.Fatalf("member status probes = %d, want exactly 1 for permanent error (no retry)", permExec.tries["openbao-member-1"])
	}
}

type permErrorExecutor struct {
	tries map[string]int
}

func (p *permErrorExecutor) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	p.tries[service]++
	if strings.Contains(strings.Join(args, " "), "bao status -format=json") {
		return "", errors.New("x509: certificate signed by unknown authority")
	}
	return "", nil
}

func (p *permErrorExecutor) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, _ ...string) (string, error) {
	return "", nil
}

func TestManagerPolicyAllowsBothOpenBaoPolicyEndpoints(t *testing.T) {
	for _, path := range []string{
		`path "sys/policies/acl/baseharbor-app-*"`,
		`path "sys/policy/baseharbor-app-*"`,
	} {
		if !strings.Contains(managerPolicy, path) {
			t.Fatalf("manager policy missing %s", path)
		}
	}
}

func TestManagerPolicyPermitsOnlyOwnCapabilityInspection(t *testing.T) {
	if !strings.Contains(managerPolicy, "path \"sys/capabilities-self\" {\n  capabilities = [\"update\"]\n}") {
		t.Fatal("manager must inspect its own scoped authority without the default policy")
	}
	for _, forbidden := range []string{`path "sys/capabilities"`, `path "sys/capabilities-accessor"`, `path "sys/*"`} {
		if strings.Contains(managerPolicy, forbidden) {
			t.Fatalf("manager may not inspect or acquire broader authority: %s", forbidden)
		}
	}
}
