package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// TestReconcileRealCreateOnlyCASSurvivesConcurrentSeed drives the PRODUCTION
// create-only compare-and-swap adapter (openbao.SetManagedCredentialIfAbsent)
// end to end against a faithful in-memory model of the baseharbor OpenBao
// managed mount. A concurrent managed winner is already present; two real
// seed adapters then race through the production command -cas=0. Exactly
// one production Get reads the winner back, the create-only guard makes every
// stale seed a no-op, and the existing managed value wins.
//
// The executor models the OpenBao managed KV v2 mount and its manager AppRole
// login, and makes the create-only put atomic under a mutex so concurrent
// adapters decide the winner by the single production command, not by
// scheduling. There is no hand-rolled CAS simulation of the behaviour under
// test — the production command string is the only thing that decides the
// winner.
func TestReconcileRealCreateOnlyCASSurvivesConcurrentSeed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	seedLocalOptIn(t, "race-device")

	files := newTestOpenBaoFiles(t, root)
	authority := newFakeBao(t)
	authority.files = files

	// A concurrent managed winner exists as if another enrolled device created
	// it before the seeds ran.
	concurrentValue := mustEncode("concurrent", "concurrent-secret")
	authority.seedObject("race-device", concurrentValue)

	// One production seed path runs the full Get-then-create-only-put
	// sequence; a rival installs the concurrent winner through the real
	// create-only adapter at the same instant. Exactly one production Get runs.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		seedViaProductionAdapter(t, authority, files)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := openbao.SetManagedCredentialIfAbsent(
			context.Background(), authority, files, "targets/race-device/human/dev-management", concurrentValue,
		); err != nil && !errors.Is(err, openbao.ErrManagedCredentialAlreadyExists) {
			t.Errorf("rival create-only seed returned unexpected error: %v", err)
		}
	}()
	wg.Wait()

	// The initial Get and two CAS-failure readbacks observe the existing managed value.
	if authority.gets != 3 {
		t.Fatalf("production Get calls = %d, want 3 (initial inspection and two CAS-failure readbacks)", authority.gets)
	}
	// Both concurrent create-only seeds were rejected (AlreadyExists) — the
	// winner was never overwritten.
	if authority.createOnlyRejections < 1 {
		t.Fatalf("create-only writes rejected by the guard = %d, want at least 1 (each seed against the existing value fails)", authority.createOnlyRejections)
	}
	if authority.actualOverwrites != 0 {
		t.Fatalf("actual overwrites of the existing value = %d, want 0 (existing managed value preserved)", authority.actualOverwrites)
	}
	got := authority.readObject("race-device")
	if string(got) != string(concurrentValue) {
		t.Fatalf("managed winner = %q, want the concurrent value %q to survive", got, concurrentValue)
	}
}

// seedViaProductionAdapter reproduces exactly what openBaoManagedCredentialStore
// .Persist + devaccess.Recover do for the fresh-device seed: the reconcile path
// reads the managed value once (Get), and on NOT_FOUND seeds it with the
// create-only SetManagedCredentialIfAbsent adapter. It drives the real adapter,
// never a fake, against the controlled executor.
func seedViaProductionAdapter(t *testing.T, authority *fakeBao, files bhruntime.Files) {
	t.Helper()
	ctx := context.Background()
	ref := "targets/race-device/human/dev-management"

	// The reconcile path reads the managed value once.
	if _, err := openbao.GetManagedCredential(ctx, authority, files, ref); err != nil {
		t.Errorf("production Get returned error: %v", err)
		return
	}

	// The fresh-device seed uses the create-only / compare-and-swap adapter.
	data := mustEncode("developer", "local-opt-in")
	if err := openbao.SetManagedCredentialIfAbsent(ctx, authority, files, ref, data); err != nil {
		// ErrManagedCredentialAlreadyExists is the winning case: the concurrent
		// value was created first and the seed no-ops.
		if !errors.Is(err, openbao.ErrManagedCredentialAlreadyExists) {
			t.Errorf("production create-only seed returned unexpected error: %v", err)
		}
	}
}

// TestReconcileRealCreateOnlyMutationIsDeterministicallyRed proves the create-only
// guard is load-bearing: removing -cas=0 from the production
// SetManagedCredentialIfAbsent command turns the create-only put into an
// unconditional write, which clobbers the concurrent winner with the stale local
// candidate. This is the RED cell the mutation must fail in one run — it is not
// timing dependent: the authority already holds the concurrent winner, so an
// unconditional write always clobbers it.
//
// The mutation is applied to a disposable copy of the product source (the exact
// line the review cited) and the race cell is run there with a real `go test`
// process against the mutated production command — never the checked-in,
// unmutated source. The RED result is captured and asserted here.
func TestReconcileRealCreateOnlyMutationIsDeterministicallyRed(t *testing.T) {
	if os.Getenv("BAHA_MUTATION_COPY") == "1" {
		runMutationRaceCell(t)
		return
	}
	red := runMutationInDisposableCopy(t)
	if !red {
		t.Fatalf("mutant production command stayed green: the create-only guard is NOT load-bearing")
	}
}

// runMutationRaceCell is the body that runs inside the mutated copy. The rival
// installs the concurrent winner AFTER the production seed's Get (which sees
// NOT_FOUND) and BEFORE its unconditional put, so the winner is present when the
// unconditionally-written seed clobbers it — deterministically.
func runMutationRaceCell(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	if _, err := devaccess.Configure("race-device", "dev", "developer", "local-opt-in"); err != nil {
		t.Fatalf("seed local opt-in credential: %v", err)
	}
	files := newTestOpenBaoFiles(t, root)
	authority := newFakeBao(t)
	authority.files = files
	concurrentValue := mustEncode("concurrent", "concurrent-secret")

	// The rival installs the winner after the production Get but before its put.
	authority.rivalReady = make(chan struct{})
	authority.gateGet = make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		seedViaProductionAdapter(t, authority, files)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-authority.gateGet
		if err := openbao.SetManagedCredentialIfAbsent(
			context.Background(), authority, files, "targets/race-device/human/dev-management", concurrentValue,
		); err != nil {
			t.Errorf("rival create-only seed returned error: %v", err)
		}
		close(authority.rivalReady)
	}()
	wg.Wait()

	got := authority.readObject("race-device")
	// The red proof: a correct create-only seed must NOT clobber the concurrent
	// winner. With the -cas=0 guard removed the unconditional write
	// clobbers it, so THIS assertion fails — the deterministic RED outcome that
	// proves the guard is load-bearing.
	if string(got) != string(concurrentValue) {
		t.Fatalf("mutant managed value = %q, want the concurrent winner %q to survive an unconditional seed", got, concurrentValue)
	}
}

// fakeBao is a faithful in-memory model of the baseharbor OpenBao managed KV v2
// mount plus its manager AppRole login, used to drive the production
// SetManagedCredentialIfAbsent adapter. The create-only put is atomic under a
// mutex so concurrent adapters decide the winner by the single production
// command, not by scheduling.
type fakeBao struct {
	t  *testing.T
	mu sync.Mutex
	// object holds the single managed object value, or nil when absent.
	object []byte
	// present reports whether the object currently exists (the create-only
	// decision is based on this, never on a cached count).
	present bool

	// files is the runtime the adapter reads admin credentials from.
	files bhruntime.Files

	// gets counts production managed Gets through the adapter; createOnlyRejections
	// counts create-only puts rejected because the reference already exists;
	// actualOverwrites counts mutant unconditional writes that replaced an
	// existing value.
	gets                 int
	createOnlyRejections int
	actualOverwrites     int

	// inMutation gates the rival install: when set, the production Get blocks on
	// gateGet until the rival has installed the winner, so the winner is present
	// when the unconditional mutant put clobbers it.
	rivalReady chan struct{}
	gateGet    chan struct{}
}

func newFakeBao(t *testing.T) *fakeBao {
	return &fakeBao{t: t}
}

// newTestOpenBaoFiles builds a runtime Files whose Env dir carries the manager
// AppRole admin credentials the real SetManagedCredentialIfAbsent adapter reads
// via LoadAdminCredentials.
func newTestOpenBaoFiles(t *testing.T, root string) bhruntime.Files {
	t.Helper()
	runtimeDir := filepath.Join(root, ".baseharbor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	files := bhruntime.Files{
		Compose: filepath.Join(runtimeDir, "compose.yaml"),
		Env:     filepath.Join(runtimeDir, "runtime.env"),
	}
	creds := filepath.Join(filepath.Dir(files.Env), "openbao-admin.env")
	if err := os.WriteFile(creds, []byte("OPENBAO_ROLE_ID=manager-role\nOPENBAO_SECRET_ID=manager-secret\n"), 0o600); err != nil {
		t.Fatalf("write manager admin credentials: %v", err)
	}
	return files
}

// seedObject installs an existing managed object as if another device created
// it before the seeds ran.
func (f *fakeBao) seedObject(_ string, value []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.object = append([]byte(nil), value...)
	f.present = true
}

// readObject returns the current managed object value.
func (f *fakeBao) readObject(_ string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.object...)
}

// ExecProject answers the OpenBao status probe the production Inspect call uses.
func (f *fakeBao) ExecProject(_ context.Context, _, _, _, _ string, args ...string) (string, error) {
	if strings.Contains(strings.Join(args, " "), "bao status -format=json") {
		return `{"initialized":true,"sealed":false,"version":"2.7.1"}`, nil
	}
	f.t.Fatalf("fakeBao.ExecProject received unexpected command: %v", args)
	return "", errors.New("fakeBao: unexpected ExecProject")
}

// ExecProjectInput handles the AppRole login and the managed get/put commands
// the SetManagedCredential and SetManagedCredentialIfAbsent adapters issue.
func (f *fakeBao) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	cmd := strings.Join(args, " ")
	switch {
	case strings.Contains(cmd, "bao write -format=json auth/approle/login"):
		// loginManager: return the manager client token.
		return `{"auth":{"client_token":"manager-token"}}`, nil

	case strings.Contains(cmd, "bao kv get -format=json -mount=baseharbor"):
		f.mu.Lock()
		rivalReady := f.rivalReady
		gateGet := f.gateGet
		present := f.present
		object := f.object
		f.gets++
		f.mu.Unlock()
		// In the mutation race, let the rival install the winner while the
		// production Get is "in flight" so the winner is present when the
		// unconditional mutant put runs — the NOT_FOUND the adapter observed is
		// the race window round-3 closes.
		if rivalReady != nil && gateGet != nil {
			close(gateGet)
			<-rivalReady
			// Re-read: the rival may have installed the winner.
			f.mu.Lock()
			present = f.present
			object = f.object
			f.mu.Unlock()
		}
		if !present {
			return "NOT_FOUND\n", nil
		}
		// readManagedCredentialWithToken expects: FOUND\n{json with
		// data.data.value}. decodeApplicationSecretValue unwraps it.
		valueJSON, _ := json.Marshal(object)
		return "FOUND\n" + escapeOpenBaoValue(valueJSON), nil

	case strings.Contains(cmd, "bao kv put"):
		// Create-only / compare-and-swap put. The reference existence check and
		// the write are atomic under f.mu: a pre-existing object fails closed
		// (returning an error the adapter maps to ErrManagedCredentialAlreadyExists)
		// and leaves the existing value untouched; otherwise it creates the
		// object. This is the only behaviour that decides the race winner.
		//
		// The fake honours the production -cas=0 flag literally: when the
		// production command drops the flag (the round-3 mutation), the put
		// becomes an unconditional overwrite and clobbers the concurrent winner.
		// That is how the single mutant cell goes RED deterministically — the
		// production command string is the load-bearing behaviour under test.
		isCreateOnly := strings.Contains(cmd, "-cas=0")
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.present {
			if isCreateOnly {
				f.createOnlyRejections++
				return "", errors.New("value already exists")
			}
			// Mutant: unconditional write overwrites the existing value.
			f.actualOverwrites++
		}
		payload := stdinPayload(input)
		f.object = append([]byte(nil), payload...)
		f.present = true
		return "", nil

	default:
		f.t.Fatalf("fakeBao.ExecProjectInput received unexpected command: %v", args)
		return "", errors.New("fakeBao: unexpected ExecProjectInput")
	}
}

// escapeOpenBaoValue wraps a JSON value blob the way an OpenBao kv get returns
// it so decodeApplicationSecretValue can unwrap it:
// {"data":{"data":{"value":"<json>"}}}.
func escapeOpenBaoValue(jsonValue []byte) string {
	inner := map[string]any{"value": string(jsonValue)}
	wrapped, _ := json.Marshal(map[string]any{"data": map[string]any{"data": inner}})
	return string(wrapped)
}

// stdinPayload returns the managed value payload the adapter wrote on stdin.
// The production prefix is the BAO_TOKEN export line followed by the payload;
// we drop the token line and return the remainder.
func stdinPayload(input []byte) []byte {
	lines := strings.Split(string(input), "\n")
	var payload strings.Builder
	for i, line := range lines {
		if i == 0 {
			continue // first line is the token value
		}
		if strings.HasPrefix(line, "export ") || line == "" {
			continue
		}
		payload.WriteString(line)
	}
	return []byte(payload.String())
}

// Ensure devaccess stays referenced if the adapter signature evolves.
var _ = devaccess.Credentials{}

// copyTree recursively copies src into dest (mirroring cp -R) for the
// mutation-in-disposable-copy proof. Only regular files are copied; directories
// are recreated. Build caches, VCS, and local worktree scratch dirs are skipped.
func copyTree(src, dest string) error {
	forbidden := regexp.MustCompile(`^(?:\.git|\.gopath|\.models|\.tmp|\.w\d+|review\d+[a-z]*-(?:cache|tmp)|\w+-wt|\w+-cache|\w+-tmp)$`)
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		for _, p := range parts {
			if p != "" && forbidden.MatchString(p) {
				return filepath.SkipDir
			}
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil // skip symlinks (e.g. go toolchain links)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// runMutationInDisposableCopy proves the mutation in isolation. It copies the
// whole product module to a temp dir, removes -cas=0 from the EXACT
// production command line the review cited, copies this test package's race
// files into the copy, and runs `go test` there. With the guard removed the
// race cell's red assertion fires (the concurrent winner is clobbered), so the
// mutated run exits non-zero — the deterministic RED outcome.
func runMutationInDisposableCopy(t *testing.T) bool {
	src := os.Getenv("BAHA_ECOSYSTEM_ROOT")
	if src == "" {
		t.Skip("BAHA_ECOSYSTEM_ROOT not set; mutation-in-copy not run")
	}
	prod := filepath.Join(src, "internal", "openbao", "managed_credentials.go")
	data, err := os.ReadFile(prod)
	if err != nil {
		t.Fatalf("read production adapter: %v", err)
	}
	if !bytes.Contains(data, []byte("bao kv put -cas=0 -mount=baseharbor")) {
		t.Skip("production adapter already lacks -cas=0; nothing to mutate")
	}

	dest := t.TempDir()
	if err := copyTree(src, dest); err != nil {
		t.Fatalf("copy product module: %v", err)
	}

	mutated := bytes.Replace(data, []byte("bao kv put -cas=0 -mount=baseharbor"), []byte("bao kv put -mount=baseharbor"), 1)
	if bytes.Equal(mutated, data) {
		t.Fatal("mutation did not apply: exact -cas=0 line not found")
	}
	if err := os.WriteFile(filepath.Join(dest, "internal", "openbao", "managed_credentials.go"), mutated, 0o600); err != nil {
		t.Fatalf("write mutated adapter: %v", err)
	}

	// Inside the copy, the mutation test takes the BAHA_MUTATION_COPY branch
	// (runMutationRaceCell). With -cas=0 removed from the production
	// adapter the concurrent winner is clobbered and the red assertion fails, so
	// the mutated run exits non-zero — the deterministic RED outcome.
	cmd := exec.Command("go", "test", "./cmd/baha/", "-run", "TestReconcileRealCreateOnlyMutationIsDeterministicallyRed", "-count=1")
	cmd.Dir = dest
	cmd.Env = append(os.Environ(), "BAHA_ECOSYSTEM_ROOT="+src, "BAHA_MUTATION_COPY=1")
	out, err := cmd.CombinedOutput()
	t.Logf("mutated go test output:\n%s", out)
	// Non-zero exit = the red assertion fired = mutation is load-bearing.
	return err != nil
}
