package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestApplicationMaintenanceCausalMutantsKeepEveryGuardRequired fixes each
// safety guard of the maintenance client to its weakest plausible alternative
// and requires a red signal, by driving the guard's own production input through
// a test seam. A guard no mutant can break is not load-bearing, so the assertion
// itself would be the thing to remove; a mutant that survives proves the
// corresponding path is not actually enforced.
func TestApplicationMaintenanceCausalMutantsKeepEveryGuardRequired(t *testing.T) {
	t.Run("authority lookup is required before routing", func(t *testing.T) {
		// Mutant: route the operation before checking that an installation owns the
		// selected deployment.
		resolved, _ := maintenanceInstalledDeployment(t)
		root, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, "deployment.json")); err != nil {
			t.Fatal(err)
		}
		reached := false
		previous := maintenanceLifecycleOwner
		t.Cleanup(func() { maintenanceLifecycleOwner = previous })
		maintenanceLifecycleOwner = func(context.Context, string, resolvedApplication, []string, io.Writer, io.Writer) error {
			reached = true
			return nil
		}
		err = submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
			Name: resolved.Manifest.Name, Operation: "backup", Confirmed: true,
			PasswordFile: maintenancePasswordFile(t, maintenanceClientHome(t)),
			Artifact:     maintenanceWrittenArchive(t),
		}, io.Discard, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Code != machine.ErrorOwnershipAmbiguous {
			t.Fatalf("the ownership guard did not refuse the submission: %v", err)
		}
		if reached {
			t.Fatal("routing ahead of the authority check is sufficient: the ownership guard is not load-bearing")
		}
	})

	t.Run("client-local path guard is required", func(t *testing.T) {
		// Mutant: hand the client's reference straight to the lifecycle owner.
		resolved, _ := maintenanceInstalledDeployment(t)
		reached := false
		previous := maintenanceLifecycleOwner
		t.Cleanup(func() { maintenanceLifecycleOwner = previous })
		maintenanceLifecycleOwner = func(context.Context, string, resolvedApplication, []string, io.Writer, io.Writer) error {
			reached = true
			return nil
		}
		err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
			Name: resolved.Manifest.Name, Operation: "restore", Artifact: "laptop-archive.bhbackup",
			PasswordFile: maintenancePasswordFile(t, maintenanceClientHome(t)), Confirmed: true,
		}, io.Discard, io.Discard)
		if err == nil {
			t.Fatal("a laptop filename reached the lifecycle owner with no guard at all")
		}
		if reached {
			t.Fatal("the lifecycle owner accepted an unvalidated client path: the path guard is not load-bearing")
		}
	})

	t.Run("remote capability guard is required", func(t *testing.T) {
		// Mutant: treat any selected Target as maintenance-capable.
		ctx, core, node := remoteApplicationCoreFixture(t)
		ctx = withCoreAuthority(ctx, core)
		resolved := maintenanceRemoteDeployment(t, core, node)
		previous := maintenanceLifecycleOwner
		t.Cleanup(func() { maintenanceLifecycleOwner = previous })
		maintenanceLifecycleOwner = func(context.Context, string, resolvedApplication, []string, io.Writer, io.Writer) error {
			t.Fatal("a remote backup reached a lifecycle owner")
			return nil
		}
		err := submitApplicationMaintenance(ctx, resolved, applicationMaintenanceOptions{
			Name: resolved.Manifest.Name, Operation: "backup", Confirmed: true,
			PasswordFile: maintenancePasswordFile(t, maintenanceClientHome(t)),
		}, io.Discard, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Code != machine.ErrorCapabilityMissing {
			t.Fatalf("remote backup was not refused as unsupported: %v", err)
		}
	})

	t.Run("durable authority is required before an input is opened", func(t *testing.T) {
		// Mutant: validate the client's references before establishing that an
		// installation owns the deployment. The order is load-bearing: a submission
		// for a deployment nobody owns must be refused as an ownership failure, not
		// reframed as a path-usage error about a file the client was never entitled
		// to open in the first place.
		resolved, _ := maintenanceInstalledDeployment(t)
		root, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, "deployment.json")); err != nil {
			t.Fatal(err)
		}
		reached := false
		previous := maintenanceLifecycleOwner
		t.Cleanup(func() { maintenanceLifecycleOwner = previous })
		maintenanceLifecycleOwner = func(context.Context, string, resolvedApplication, []string, io.Writer, io.Writer) error {
			reached = true
			return nil
		}
		archive := maintenanceWrittenArchive(t)
		err = submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
			Name: resolved.Manifest.Name, Operation: "restore", Artifact: archive,
			PasswordFile: maintenancePasswordFile(t, maintenanceClientHome(t)), Confirmed: true,
		}, io.Discard, io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Code != machine.ErrorOwnershipAmbiguous {
			t.Fatalf("an unowned deployment was not refused as an ownership failure: %v", err)
		}
		if reached {
			t.Fatal("the client routed maintenance for a deployment no installation owns: the ownership guard is not load-bearing")
		}
	})

	t.Run("durable window guard is required", func(t *testing.T) {
		// Mutant: let a submission whose mutation never completed advance the window.
		resolved, _ := maintenanceInstalledDeployment(t)
		if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", "sha256:one"); err != nil {
			t.Fatal(err)
		}
		_, found, err := application.LoadMaintenanceAuthority(resolved.stateRoot())
		if err != nil || !found {
			t.Fatalf("the durable window was not written: %v", err)
		}
		// The same operation and bytes must resolve to the same durable operation;
		// if a mutant ever re-stamps the revision, this is where it shows.
		if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", "sha256:one"); !errors.Is(err, errMaintenanceOperationAlreadyRecorded) {
			t.Fatalf("a replayed submission advanced the durable window: %v", err)
		}
		cancelled, stop := context.WithCancel(context.Background())
		stop()
		if err := commitApplicationMaintenanceWindow(cancelled, resolved, "restore", ""); err == nil {
			t.Fatal("a cancelled mutation still recorded a durable window")
		}
	})
}

func maintenanceWrittenArchive(t *testing.T) string {
	t.Helper()
	path := filepath.Join(maintenanceClientHome(t), "written.bhbackup")
	if err := os.WriteFile(path, []byte("recovery unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
