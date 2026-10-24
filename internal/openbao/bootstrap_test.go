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
	memberStates    map[string]*haMemberState
}

type haMemberState struct {
	initialized          bool
	sealed               bool
	tries                int
	inspectCount         int
	preUnsealAttempts    int
	writes               int
	postUnsealVerification int
	verifiedAfterUnseal  bool
	retryAttempts        int
}

func newHaMemberState(initialized, sealed bool) *haMemberState {
	return &haMemberState{
		initialized: initialized,
		sealed:      sealed,
	}
}

func (f *flakyMemberExecutor) ExecProject(ctx context.Context, _, _, _, service string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "bao status -format=json") {
		return "", nil
	}

	// Resolve service name to a member state.
	var ms *haMemberState
	var ok bool

	// For the admin service (openbao-admin), return the state of any unsealed member.
	if service == serviceName {
		for _, candidate := range f.memberStates {
			if !candidate.sealed {
				ms = candidate
				break
			}
		}
		if ms == nil {
			// All members still sealed: return the first member's sealed state.
			for _, st := range f.states {
				ms = newHaMemberState(st.Initialized, st.Sealed)
				break
			}
		}
	} else {
		ms, ok = f.memberStates[service]
		if !ok {
			st, exists := f.states[service]
			if !exists {
				// Fallback: use first available state.
				for _, st := range f.states {
					ms = newHaMemberState(st.Initialized, st.Sealed)
					f.memberStates[service] = ms
					break
				}
				if ms == nil {
					return "", errors.New("member unavailable")
				}
			} else {
				ms = newHaMemberState(st.Initialized, st.Sealed)
				f.memberStates[service] = ms
			}
		}
	}

	ms.tries++

	// If this is a post-unseal verification call (ms.writes > 0), return current sealed state.
	if ms.writes > 0 {
		ms.postUnsealVerification++
		if !ms.sealed {
			ms.verifiedAfterUnseal = true
		}
		return statusJSON(ms.initialized, ms.sealed), nil
	}

	// Pre-unseal probing: check transient window.
	if fails, ok := f.transientWindow[service]; ok && fails > 0 {
		f.transientWindow[service]--
		ms.preUnsealAttempts++
		return "", errors.New("connection refused")
	}

	// Successful probe: first call is initial Inspect, subsequent are retry-success.
	if ms.inspectCount == 0 {
		ms.inspectCount++
	}
	return statusJSON(ms.initialized, ms.sealed), nil
}

func (f *flakyMemberExecutor) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, svc string, args ...string) (string, error) {
	unsealing := false
	var payload struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(input, &payload); err == nil {
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

	// Parse target member from BAO_ADDR in shell script arguments.
	var targetMember string
	for _, arg := range args {
		if idx := strings.Index(arg, "BAO_ADDR=https://"); idx >= 0 {
			rest := arg[idx+len("BAO_ADDR="):]
			addr := ""
			if eqIdx := strings.Index(rest, "="); eqIdx >= 0 {
				addr = rest[:eqIdx]
			} else {
				// BAO_ADDR=https://... at end of arg.
				addr = rest
			}
			addr = strings.TrimPrefix(addr, "https://")
			// Extract just the host:port before the next space.
			if spIdx := strings.Index(addr, " "); spIdx >= 0 {
				addr = addr[:spIdx]
			}
			addr = strings.TrimSuffix(addr, ":8200")
			targetMember = addr
		}
	}
	if targetMember == "" {
		targetMember = svc
	}
	ms, ok := f.memberStates[targetMember]
	if !ok {
		st, exists := f.states[targetMember]
		if exists {
			ms = newHaMemberState(st.Initialized, st.Sealed)
		} else {
			// Fallback: pick first state.
			for m, s := range f.states {
				targetMember = m
				ms = newHaMemberState(s.Initialized, s.Sealed)
				break
			}
		}
		f.memberStates[targetMember] = ms
	}
	ms.writes++
	ms.sealed = false
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
			"openbao-member-1": 3,
		},
		memberStates: map[string]*haMemberState{},
	}
	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	recovery := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if err := os.WriteFile(recovery, []byte("{\"version\":1,\"key_shares\":1,\"key_threshold\":1,\"unseal_keys\":[\"unseal-secret\"]}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Unseal(ctx, executor, files, recovery); err != nil {
		t.Fatalf("unseal failed on transient member probe: %v", err)
	}

	ms1 := executor.memberStates["openbao-member-1"]
	if ms1 == nil {
		t.Fatal("no member-1 state recorded")
	}
	if ms1.inspectCount == 0 {
		t.Fatal("member-1: expected at least one successful initial Inspect")
	}
	if ms1.preUnsealAttempts < 3 {
		t.Fatalf("member-1 pre-unseal attempts = %d, want at least 3 after transient failures", ms1.preUnsealAttempts)
	}
	if ms1.writes == 0 {
		t.Fatal("member-1: unseal write was never executed")
	}
	if ms1.postUnsealVerification == 0 {
		t.Fatal("member-1: post-unseal verification was never executed")
	}
	if !ms1.verifiedAfterUnseal {
		t.Fatal("member-1: sealed=false not verified after unseal")
	}
}

// neverAnswersExecutor simulates a member that passes initial Inspect but
// permanently fails during probeMemberState retry: first ExecProject returns
// the member's state, all subsequent calls return connection-refused.
type neverAnswersExecutor struct {
	states       map[string]State
	memberStates map[string]*haMemberState
	inspectDone  bool
}

func (f *neverAnswersExecutor) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "bao status -format=json") {
		return "", nil
	}
	ms, ok := f.memberStates[service]
	if !ok {
		st, exists := f.states[service]
		if !exists {
			return "", errors.New("member unavailable")
		}
		ms = &haMemberState{
			initialized: st.Initialized,
			sealed:      st.Sealed,
		}
		f.memberStates[service] = ms
	}
	ms.tries++

	// First ExecProject call (initial Inspect): return success.
	// Subsequent calls (probeMemberState retry): return connection refused.
	if !f.inspectDone {
		f.inspectDone = true
		return statusJSON(ms.initialized, ms.sealed), nil
	}
	// All subsequent calls are probeMemberState with retry=true: fail with transient error.
	ms.preUnsealAttempts++
	return "", errors.New("connection refused")
}

func (f *neverAnswersExecutor) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, _ ...string) (string, error) {
	return "", nil
}

// TestUnsealFailsFastWhenMemberNeverAnswers proves fail-fast is preserved: a
// member that never answers its status probe exhausts its retry budget and the
// unseal call returns an error instead of hanging or silently succeeding.
// Uses phase-specific counters: exactly 5 pre-unseal attempts, zero writes.
func TestUnsealFailsFastWhenMemberNeverAnswers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executor := &neverAnswersExecutor{
		states: map[string]State{
			"openbao-member-1": {Initialized: true, Sealed: true},
		},
		memberStates: map[string]*haMemberState{
			"openbao-member-1": {
				initialized: true,
				sealed:      true,
			},
		},
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

	ms1 := executor.memberStates["openbao-member-1"]
	if ms1 == nil {
		t.Fatal("no member-1 state recorded")
	}
	// Exactly 5 pre-unseal attempts: attempt loop 1..5, cap at attempt >= 5.
	if ms1.preUnsealAttempts != 5 {
		t.Fatalf("member-1 pre-unseal attempts = %d, want exactly 5", ms1.preUnsealAttempts)
	}
	// Zero unseal writes: precondition (probe failure) prevents unseal.
	if ms1.writes != 0 {
		t.Fatalf("member-1 unseal writes = %d, want 0 (precondition failed)", ms1.writes)
	}
}

// TestUnsealFailsImmediatelyOnPermanentNonTransientError proves that
// permanent (non-transient) errors are not retried: a member that returns
// a TLS/CA error exits with exactly one probe execution, matching the
// fail-fast contract for terminal runtime failures.
func TestUnsealFailsImmediatelyOnPermanentNonTransientError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// permErrorExecutor simulates a permanent TLS error during pre-unseal
	// probing: initial Inspect succeeds, then the retry path returns
	// x509-permanent error. This exercises the retry-enabled path where
	// isTransientError must classify the error as permanent and fail fast.
	executor := &permErrorExecutor{
		memberStates: map[string]*haMemberState{
			"openbao-member-1": {
				initialized: true,
				sealed:      true,
			},
		},
		permanentDuringRetry: true,
	}
	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	recovery := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if err := os.WriteFile(recovery, []byte("{\"version\":1,\"key_shares\":1,\"key_threshold\":1,\"unseal_keys\":[\"unseal-secret\"]}"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Unseal(ctx, executor, files, recovery)
	if err == nil {
		t.Fatal("expected unseal to fail on permanent error")
	}
	ms1 := executor.memberStates["openbao-member-1"]
	if ms1 == nil {
		t.Fatal("no member-1 state recorded")
	}
	// Exactly one attempt in the pre-unseal phase: permanent error is not retried.
	// Note: Inspect also makes one call (tries=2 total), but ms1.retryAttempts=1.
	if ms1.retryAttempts != 1 {
		t.Fatalf("member-1 retry attempts = %d, want exactly 1 (no retry for permanent error)", ms1.retryAttempts)
	}
	// Error identity preserved: x509 error is returned.
	if err == nil || !strings.Contains(err.Error(), "x509") {
		t.Fatalf("error = %v, want x509-permanent error", err)
	}
}

type permErrorExecutor struct {
	memberStates         map[string]*haMemberState
	permanentDuringRetry bool
	inspectSuccessOnce   bool
}

func (p *permErrorExecutor) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	ms, ok := p.memberStates[service]
	if !ok {
		return "", errors.New("member unavailable")
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "bao status -format=json") {
		return "", nil
	}
	// First call (initial Inspect via inspectMemberState): succeeds.
	// Subsequent calls (probeMemberState retry path): permanent error, 1 retry attempt.
	if !p.inspectSuccessOnce {
		p.inspectSuccessOnce = true
		return statusJSON(ms.initialized, ms.sealed), nil
	}
	ms.retryAttempts++
	return "", errors.New("x509: certificate signed by unknown authority")
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

// cancellationExecutor: always fails with transient error to trigger retry
// backoff, then context cancellation during backoff causes probeMemberState
// to return context.Canceled.
type cancellationExecutor struct {
	memberStates map[string]*haMemberState
}

func (f *cancellationExecutor) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "bao status -format=json") {
		return "", nil
	}
	_, ok := f.memberStates[service]
	if !ok {
		f.memberStates[service] = &haMemberState{
			initialized: true,
			sealed:      true,
		}
	}
	// Always fail with transient error to trigger retry backoff.
	return "", errors.New("connection refused")
}

func (f *cancellationExecutor) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, _ ...string) (string, error) {
	return "", nil
}

// TestProbeMemberStateCancellationDuringRetryBackoff proves that cancellation
// during the retry backoff interval in probeMemberState propagates ctx.Err():
// a canceled context causes probeMemberState to return immediately without
// exhausting the retry budget.
func TestProbeMemberStateCancellationDuringRetryBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// cancellationExecutor: fails with transient error to trigger retry backoff,
	// then context cancellation during backoff causes probeMemberState to return
	// context.Canceled.
	executor := &cancellationExecutor{
		memberStates: map[string]*haMemberState{
			"openbao-member-1": {
				initialized: true,
				sealed:      true,
			},
		},
	}

	// Cancel the context after 10ms, before the 500ms retry backoff.
	time.AfterFunc(10*time.Millisecond, cancel)

	// probeMemberState with retry=true and canceled context during backoff.
	// This directly tests probeMemberState, bypassing Unseal/Inspect.
	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	_, err := probeMemberState(ctx, executor, files, "openbao-member-1", true)
	if err == nil {
		t.Fatal("expected error from canceled context during retry backoff")
	}
	if err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// invalidJSONExecutor: returns a valid HTTP-like response that is not
// valid JSON, causing json.Unmarshal to fail immediately without retry.
type invalidJSONExecutor struct {
	memberStates map[string]*haMemberState
	callCount    int
}

func (f *invalidJSONExecutor) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "bao status -format=json") {
		return "", nil
	}
	f.callCount++
	// Return non-JSON that is not a transient error.
	return "{not valid json", nil
}

func (f *invalidJSONExecutor) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, _ ...string) (string, error) {
	return "", nil
}

// TestProbeMemberStateInvalidJSONNoRetry proves that a successful but
// invalid JSON status response (not a transient error) is not retried:
// probeMemberState returns immediately with the invalid response error.
func TestProbeMemberStateInvalidJSONNoRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	executor := &invalidJSONExecutor{
		memberStates: map[string]*haMemberState{
			"openbao-member-1": {
				initialized: true,
				sealed:      true,
			},
		},
	}

	files := bhruntime.Files{HA: true, Compose: "compose.yaml", Env: "runtime.env"}
	_, err := probeMemberState(ctx, executor, files, "openbao-member-1", true)
	if err == nil {
		t.Fatal("expected error from invalid JSON response")
	}
	if err.Error() != "invalid status response" {
		t.Fatalf("error = %v, want 'invalid status response'", err)
	}
}
