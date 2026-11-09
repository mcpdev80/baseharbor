package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestConcurrentInitNeverCreatesDoubleApplicationIDRegistration is the
// in-process race cell for the bug hunt hp4 finding BF-01 (card 0427-175):
// two 'baha app init' runs that allocate distinct deployment uuids for ONE
// application_id used to both pass the duplicate check in the same window and
// permanently poison the Target. The registration seam now serializes first
// claims, so no interleaving may leave more than one record per identity.
func TestConcurrentInitNeverCreatesDoubleApplicationIDRegistration(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	appID := application.MustNewApplicationID()

	const racers = 8
	var wg sync.WaitGroup
	errs := make([]error, racers)
	ids := make([]deployment.DeploymentIdentity, racers)
	for i := 0; i < racers; i++ {
		id, err := deployment.NewDeploymentIdentity(target.Name, appID, selection.Manifest.Name+"-"+strconv.Itoa(i), "dev")
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
				Version:  deployment.DeploymentRecordVersion,
				Identity: ids[i],
				Source:   deployment.DeploymentSource{Kind: "repository", Repository: selection.RepositoryRoot, Manifest: selection.ManifestPath},
				Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker"},
			})
		}(i)
	}
	wg.Wait()

	var successes int
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, deployment.ErrDuplicateApplicationID),
			errors.Is(err, deployment.ErrRegistryConcurrentWrite),
			errors.Is(err, deployment.ErrRegistryConflict):
			// A fail-closed refusal is a correct outcome for a racer.
		default:
			t.Fatalf("unexpected registration error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("exactly one racing registration may persist, got %d", successes)
	}
	records, err := deployment.ListDeployments(target.Name)
	if err != nil {
		t.Fatalf("the registry must be healthy after the race — the race poisoned it: %v", err)
	}
	var claims int
	for _, record := range records {
		if record.Identity.ApplicationID == appID {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("%d deployment records claim one application_id; the Target is poisoned", claims)
	}
}

// TestConcurrentInitAcrossRealProcessesIsSerializedOrRefused is the
// two-OS-process cell (the shape hp4 actually ran: two `baha app init`
// processes). A re-exec of the test binary registers the identity while this
// test's save is parked inside the registry critical section via the
// SaveDeploymentRecord test seam. The other process must be refused — either
// by the busy lock or by the duplicate scan — never allowed to persist a
// second record. Style follows internal/stableid/two_process_lock_test.go
// (re-exec, marker files).
func TestConcurrentInitAcrossRealProcessesIsSerializedOrRefused(t *testing.T) {
	// The child-mode dispatch lives in TestMain (see
	// concurrentInitChildDispatch) so it runs before the package fixtures.
	if os.Getenv("BAHA_175_REG_CHILD") == "1" {
		t.Skip("child-mode body runs through TestMain dispatch")
	}
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	appID := application.MustNewApplicationID()
	markerDir := t.TempDir()
	held := filepath.Join(markerDir, "held")
	release := filepath.Join(markerDir, "released")

	// Process 1 (this goroutine) enters the registration seam and parks after
	// the claim is checked but before it persists: the exact window hp4 hit.
	parked := make(chan struct{})
	deployment.TestHookAfterRegistryClaimEnter = func() {
		if err := os.WriteFile(held, nil, 0o600); err != nil {
			t.Error(err)
			return
		}
		close(parked)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(release); err == nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	t.Cleanup(func() { deployment.TestHookAfterRegistryClaimEnter = nil })
	saveDone := make(chan error, 1)
	id, err := deployment.NewDeploymentIdentity(target.Name, appID, selection.Manifest.Name, "dev")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		saveDone <- deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
			Version:  deployment.DeploymentRecordVersion,
			Identity: id,
			Source:   deployment.DeploymentSource{Kind: "repository", Repository: selection.RepositoryRoot, Manifest: selection.ManifestPath},
			Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker"},
		})
	}()
	<-parked

	// Process 2: the second racing init, a REAL OS process. While the seam is
	// parked it must be refused (busy lock) or, after release, must converge
	// on the winner's record (duplicate scan) — in both cases it may NOT add
	// a second registration.
	child := exec.Command(os.Args[0], "-test.run=TestConcurrentInitAcrossRealProcessesIsSerializedOrRefused")
	child.Env = append(os.Environ(),
		"BAHA_175_REG_CHILD=1",
		"BAHA_175_TARGET="+target.Name,
		"BAHA_175_REPO="+selection.RepositoryRoot,
		"BAHA_175_MANIFEST="+selection.ManifestPath,
		"BAHA_175_APP_ID="+appID,
		"BAHA_175_APP_NAME="+selection.Manifest.Name,
	)
	out, childErr := child.CombinedOutput()
	_ = os.WriteFile(release, nil, 0o600)
	if saveErr := <-saveDone; saveErr != nil {
		t.Fatalf("parked first registration failed: %v", saveErr)
	}

	if childErr == nil {
		// The child "succeeded" only legitimately if it CONVERGED — i.e. it
		// did not persist a second record. The record count below is the
		// authority either way; a zero exit while it created a twin is the
		// defect, so just assert on the registry.
		_ = out
	}

	records, lerr := deployment.ListDeployments(target.Name)
	if lerr != nil {
		t.Fatalf("registry listing after the two-process race: %v\nchild output: %s", lerr, out)
	}
	var claims int
	for _, record := range records {
		if record.Identity.ApplicationID == appID {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("two-process double registration persisted %d records for one application_id; child output:\n%s", claims, out)
	}
}

// concurrentInitChildDispatch is the child-mode body of
// TestConcurrentInitAcrossRealProcessesIsSerializedOrRefused: it registers
// the SAME application_id under a fresh deployment uuid through the REAL
// storage seam — exactly what the second racing 'baha app init' process
// does — and exits 3 on a fail-closed refusal, 1 on any unexpected error,
// 0 when it persisted (which the parent only tolerates if it converged
// without creating a twin; the registry count is the authority).
func concurrentInitChildDispatch() {
	target := os.Getenv("BAHA_175_TARGET")
	repo := os.Getenv("BAHA_175_REPO")
	manifest := os.Getenv("BAHA_175_MANIFEST")
	appID := os.Getenv("BAHA_175_APP_ID")
	name := os.Getenv("BAHA_175_APP_NAME")
	id, err := deployment.NewDeploymentIdentity(target, appID, name+"-child", "dev")
	if err != nil {
		panic(err)
	}
	err = deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: manifest},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker"},
	})
	switch {
	case err == nil:
		os.Exit(0)
	case errors.Is(err, deployment.ErrDuplicateApplicationID),
		errors.Is(err, deployment.ErrRegistryConcurrentWrite),
		strings.Contains(err.Error(), "resource temporarily unavailable"):
		// The correct racing outcome: refused, nothing written.
		os.Exit(3)
	default:
		panic(err)
	}
}
