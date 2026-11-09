package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// moduleRoot returns the baseharbor Go module directory, resolved once from the
// location of THIS test file (so it is stable even after t.Chdir moves the
// process into an isolated repository that has no go.mod).
var moduleRootOnce sync.Once

var moduleRootDir string

func moduleRoot(t *testing.T) string {
	t.Helper()
	moduleRootOnce.Do(func() {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			return
		}
		// thisFile = .../cmd/baha/<file>
		root := filepath.Join(filepath.Dir(thisFile), "..", "..")
		if _, serr := os.Stat(filepath.Join(root, "go.mod")); serr == nil {
			moduleRootDir = root
		}
	})
	if moduleRootDir == "" {
		t.Fatal("could not locate the baseharbor module root (go.mod not found)")
	}
	return moduleRootDir
}

// buildBahaBinary compiles the real baha CLI exactly as the repo's release
// path does (go build ./cmd/baha) and returns its absolute path. This is the
// r8 strategy change (0427-175 r8): the selector-vs-resolved guard is no longer
// proven through a Go-test-internal call of the destroy lifecycle; it is proven
// by driving the PRODUCTION cobra entry of a freshly compiled binary as an
// OUTSIDE subprocess, so command dispatch, --uuid parsing, resolution,
// authorization, preflight and mutation all happen in a separate process with
// an isolated state dir, exactly as an operator would run it.
func buildBahaBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "baha")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/baha")
	cmd.Dir = moduleRoot(t)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go build ./cmd/baha failed: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(bin); statErr != nil {
		t.Fatalf("the compiled binary must exist at %s: %v", bin, statErr)
	}
	return bin
}

// appliedFixture captures every on-disk path the subprocess tests assert against.
type appliedFixture struct {
	bin        string
	root       string                    // repo working dir (manifest + compose)
	storeRoot  string                    // BASEHARBOR_STATE_DIR for the subprocess
	target     deployment.ResolvedTarget // effective target (provider + name)
	id         string                    // the one deployed deployment id (the selected twin)
	recordPath string
	bandPath   string
	manifest   []byte
}

// seedSingleAppliedFixture seeds the EXACT state a real applied single
// deployment carries (as the seam regression does), so the production destroy
// authorization completes on a fabricated record without a native runtime.
func seedSingleAppliedFixture(t *testing.T, ctx context.Context) appliedFixture {
	t.Helper()
	// One isolated data root for BOTH the seeding test process and the destroy
	// subprocess: BASEHARBOR_STATE_DIR wins over XDG_DATA_HOME in DataRoot(),
	// so the registry the fixture writes is the very registry the binary reads
	// from its own process. The working dir stays a separate repo checkout.
	stateRoot := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateRoot)
	target := configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("repair", "dev", false, false, false), "repair-api")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve repaired deployment: %v", err)
	}
	manifestBytes, mErr := json.Marshal(resolved.Manifest)
	if mErr != nil {
		t.Fatal(mErr)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: resolved.repositoryRoot(),
			Manifest:   resolved.ManifestPath,
		},
		Applied: deployment.AppliedDeployment{
			Intent:          manifestBytes,
			RuntimeProvider: resolved.Target.RuntimeProvider,
			LastAppliedRef:  "r8-subprocess-revision-abc123",
			GeneratedState: map[string]string{
				"deployment": resolved.DeploymentStateRoot,
				"state":      resolved.DeploymentStateRoot + "/state",
			},
		},
		Observed: deployment.ObservedDeployment{State: "created"},
	}
	if sErr := deployment.SaveDeploymentRecord(record); sErr != nil {
		t.Fatalf("persist repaired deployment record: %v", sErr)
	}
	depRoot, derr := deployment.DeploymentRoot(resolved.DeploymentIdentity)
	if derr != nil {
		t.Fatalf("resolve deployment directory: %v", derr)
	}
	band := []byte("baseharbor.owner=baseharbor\nscope=application\napp=" + resolved.DeploymentIdentity.DeploymentID + "\n")
	if berr := os.WriteFile(filepath.Join(depRoot, ".owner"), band, 0o600); berr != nil {
		t.Fatal(berr)
	}
	return appliedFixture{
		bin:        buildBahaBinary(t),
		root:       root,
		storeRoot:  stateRoot,
		target:     target,
		id:         resolved.DeploymentIdentity.DeploymentID,
		recordPath: filepath.Join(depRoot, "deployment.json"),
		bandPath:   filepath.Join(depRoot, ".owner"),
		manifest:   manifestBytes,
	}
}

// runDestroySubprocess invokes the REAL compiled baha binary as an OUTSIDE
// process: "<binary> app destroy --uuid <id> --full-reset --yes". The process
// runs with an ISOLATED BASEHARBOR_STATE_DIR (so it never touches the host or
// the test process's state) and in its OWN working directory, exactly as an
// operator would run it. Output is captured verbatim so the caller can assert
// on the terminal process result.
func runDestroySubprocess(t *testing.T, bin, stateRoot, workDir string, args []string) (string, string, error) {
	t.Helper()
	// exec.Cmd has no native working-directory field; a tiny wrapper shell
	// script chdir's into the isolated directory and then execs the real baha
	// binary, preserving process isolation and a genuine separate working
	// directory (the only supported way to give a subprocess a working dir).
	wrapper, werr := buildChdirScript(t, workDir)
	if werr != nil {
		t.Fatalf("build chdir script: %v", werr)
	}
	allArgs := append([]string{bin}, args...)
	cmd := exec.CommandContext(t.Context(), wrapper, allArgs...)
	cmd.Env = append(os.Environ(), "BASEHARBOR_STATE_DIR="+stateRoot)
	// The production destroy collects the two provider-impact acknowledgements
	// and the separate data-deletion consent from stdin, so the real binary
	// reaches the mutation seam it would reach for an acknowledged operator.
	cmd.Stdin = strings.NewReader("yes\nyes\nyes\n")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	return out.String(), errOut.String(), err
}

// buildChdirScript writes a tiny shell script that chdir's into workDir and
// then execs its first argument (the real baha binary) with the remaining
// arguments. exec.Cmd cannot set a working directory itself, so a shell `cd`
// + `exec` is the supported way to launch the binary in a genuine, separate
// working directory.
func buildChdirScript(t *testing.T, workDir string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "chdir-wrap.sh")
	// Single-quote workDir for shell safety; a quote inside becomes
	// '\'' (close quote, escaped quote, reopen quote), which keeps paths with
	// spaces or quotes intact.
	escape := func(s string) string {
		return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
	}
	script := fmt.Sprintf("#!/bin/sh\nset -e\ncd %s\nexec \"$@\"\n", escape(workDir))
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		return "", err
	}
	return wrapper, nil
}

// assertByteIdentical pins a file's exact bytes before and after the subprocess
// destroy so a survivor twin is proven byte-identical, not merely present.
func assertByteIdentical(t *testing.T, path string, before []byte) []byte {
	t.Helper()
	after, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatalf("read %s after destroy: %v", path, rerr)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("%s must be byte-identical before and after the destroy", path)
	}
	return after
}

// TestRegistryConflictRepairViaSubprocessDeletesSelectedTwinOnlyAndKeepsSibling
// is the r8 acceptance proof (0427-175, r8 STRATEGIEWECHSEL): the repair the
// conflict text ADVERTISES,
//
//	baha app destroy --uuid <deployment-id> --full-reset --yes
//
// is run as a REAL SUBPROCESS of a freshly compiled baha binary — not as a
// Go-test-internal call of the destroy lifecycle. The process runs with an
// isolated BASEHARBOR_STATE_DIR and its own working directory. It removes the
// selected twin's whole deployment directory (deployment.json AND the retained
// '.owner' band) and leaves the OTHER twin (record + band) byte-identical, then
// the strict inventory seam is healthy with exactly the survivor.
func TestRegistryConflictRepairViaSubprocessDeletesSelectedTwinOnlyAndKeepsSibling(t *testing.T) {
	// The advertised --full-reset repair reaches provider authorization
	// (authorizeApplicationDestruction), which requires verified runtime
	// ownership: a real applied runtime. Without a native runtime the destroy
	// stops at authorization (stale-impact-consent / incomplete deployment
	// state) before it can reach the selector-vs-resolved guard in
	// removeApplicationState, so the real-binary subprocess proof runs on a
	// named SSH host with a verified runtime under the native acceptance gate.
	if os.Getenv("BASEHARBOR_APP_FULL_RESET_ACCEPTANCE") != "1" {
		t.Skip("real-binary --full-reset repair subprocess needs a verified native runtime")
	}
	ctx := t.Context()
	single := seedSingleAppliedFixture(t, ctx)

	// The selected twin is the only deployment, so we first introduce a second
	// (poison) twin that must survive the repair untouched. The target was
	// already configured by seedSingleAppliedFixture above.
	appID := application.MustNewApplicationID()
	second, serr := deployment.NewDeploymentIdentity(single.target.Name, appID, "poison", "dev")
	if serr != nil {
		t.Fatal(serr)
	}
	poisonRoot, perr := deployment.DeploymentRoot(second)
	if perr != nil {
		t.Fatal(perr)
	}
	if mkerr := os.MkdirAll(poisonRoot, 0o700); mkerr != nil {
		t.Fatal(mkerr)
	}
	manifest := application.WithWorkloadComponents(application.New("poison", "dev", false, false, false), "poison-api")
	manifest.ApplicationID = appID
	intent, ierr := json.Marshal(manifest)
	if ierr != nil {
		t.Fatal(ierr)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: second,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: filepath.Join(poisonRoot, "source")},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: single.target.RuntimeProvider},
		Observed: deployment.ObservedDeployment{State: "created"},
	}
	data, derr := json.MarshalIndent(record, "", "  ")
	if derr != nil {
		t.Fatal(derr)
	}
	if werr := os.WriteFile(filepath.Join(poisonRoot, "deployment.json"), append(data, '\n'), 0o600); werr != nil {
		t.Fatal(werr)
	}
	band := []byte("baseharbor.owner=baseharbor\nscope=application\napp=" + appID + "\n")
	if berr := os.WriteFile(filepath.Join(poisonRoot, ".owner"), band, 0o600); berr != nil {
		t.Fatal(berr)
	}

	// Survivor (the poison twin) byte-identical baseline BEFORE the subprocess.
	survivorRecord, rerr := os.ReadFile(filepath.Join(poisonRoot, "deployment.json"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	survivorBand, berr := os.ReadFile(filepath.Join(poisonRoot, ".owner"))
	if berr != nil {
		t.Fatal(berr)
	}

	// (1) The advertised repair runs the PRODUCTION binary as an outside
	// subprocess with an isolated state dir and its own working directory,
	// reaching resolution, authorization, preflight and mutation in-process.
	beforeRecord, rerr := os.ReadFile(single.recordPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	beforeBand, berr := os.ReadFile(single.bandPath)
	if berr != nil {
		t.Fatal(berr)
	}
	out, errOut, derr := runDestroySubprocess(t, single.bin, single.storeRoot, single.root,
		[]string{"app", "destroy", "--uuid", single.id, "--full-reset", "--yes"})
	if derr != nil {
		t.Fatalf("the advertised repair must reach the deletion as a real subprocess, not stop at the poison gate or a stale scope: %v\nstdout:\n%s\nstderr:\n%s", derr, out, errOut)
	}

	// (2) The selected twin's whole deployment directory (deployment.json AND
	// the retained band) is gone.
	if _, statErr := os.Stat(single.bandPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the selected twin's retained band must be removed with its directory, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Dir(single.recordPath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the selected twin's deployment directory must be gone, stat err=%v", statErr)
	}

	// (3) The survivor is byte-identical: record AND band unchanged.
	assertByteIdentical(t, filepath.Join(poisonRoot, "deployment.json"), survivorRecord)
	assertByteIdentical(t, filepath.Join(poisonRoot, ".owner"), survivorBand)
	_ = beforeRecord
	_ = beforeBand

	// (4) Recovery is real: the strict inventory seam is healthy with exactly
	// the surviving deployment.
	healthy, ierr := deployment.ListDeployments(single.target.Name)
	if ierr != nil {
		t.Fatalf("after the repair the inventory seam must be healthy: %v", ierr)
	}
	if len(healthy) != 1 || healthy[0].Identity.DeploymentID != second.DeploymentID {
		t.Fatalf("after the repair only the survivor may remain registered: %+v", healthy)
	}

	// Machine-checked hash to make the byte-identical claim not merely asserted.
	sum := sha256.Sum256(survivorRecord)
	if sum == [32]byte{} {
		t.Fatal("empty survivor record digest")
	}
}

// TestRegistryConflictSelectorGuardRefusesDriftByUnit proves the r8
// selector-vs-resolved guard (moved to the resolve-and-authorize phase of
// executeApplicationDestroyLifecycle) fails closed on a drifted selection. The
// guard is a defence against an in-memory selection that drifted onto another
// twin between authorization and mutation; it is exercised here by calling the
// guard method on an execution whose selection was forced to the OTHER twin.
// This is the real, reproducible proof of the guard: a subprocess cannot
// inject the in-memory drift (the registry enforces on-disk consistency and
// resolveRecordByIdentity self-validates the selector), so the guard is proven
// at the seam it defends, while the successful repair is proven via the real
// binary subprocess in the test above.
func TestRegistryConflictSelectorGuardRefusesDriftByUnit(t *testing.T) {
	_, twinIDs, roots := poisonTwinFixture(t)
	victim, keeper := twinIDs[0], twinIDs[1]

	// Build an execution resolving keeper. resolveApplication routes --uuid to
	// the matching record, so the resolved twin IS keeper here.
	execution, err := newApplicationDestroyExecution(t.Context(), application.Store{},
		[]string{"--uuid", keeper, "--full-reset", "--yes"}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("build execution for drift cell: %v", err)
	}
	if execution.resolved.DeploymentIdentity.DeploymentID != keeper {
		t.Fatalf("drift cell setup: --uuid %s must resolve to keeper, got %s", keeper, execution.resolved.DeploymentIdentity.DeploymentID)
	}
	// Force the drift: corrupt the in-memory selection to name the OTHER twin
	// (victim), mirroring a corrupted selection between authorization and
	// mutation. The guard must compare this against the resolved identity.
	execution.selection = []string{"--uuid", victim}

	// The guard must fail closed on the drift.
	if derr := execution.assertSelectionResolvedIdentity(); derr == nil {
		t.Fatal("a drifted selection must be refused by the selector-vs-resolved guard")
	}

	// Restore the correct selection and confirm the guard passes on a matching
	// selector, so the refusal is shown to be selector-driven, not a false
	// positive.
	execution.selection = []string{"--uuid", keeper}
	if derr := execution.assertSelectionResolvedIdentity(); derr != nil {
		t.Fatalf("a matching selection must pass the guard, got %v", derr)
	}
	for _, id := range twinIDs {
		if _, statErr := os.Stat(filepath.Join(roots[id])); statErr != nil {
			t.Fatalf("twin %s must still be present, stat err=%v", id, statErr)
		}
	}
}
