package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appEditLifecycleRunner is the concrete Apply lifecycle connection used by the
// production Edit executor. The mutate callback runs inside the existing Target
// lifecycle lock, immediately before the first deployment state write; the
// default binds directly to executeApplicationApplyLifecycleResults so the
// receipt path is never an exclusively injected callback. Tests may substitute
// it only to observe the single invocation.
type appEditLifecycleRunner func(context.Context, application.Store, []string, application.ProviderImpactPlan, func(context.Context, resolvedApplication) error, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error)

// appEditSourceWriter persists confirmed Edit intent through the existing
// source-of-truth owner INSIDE the Target lifecycle lock. Repository
// applications keep their own manifest file behind an exact-byte compare and
// swap; store-managed applications recompare the internal copy before
// synchronization. Untouched manifest intent is carried unchanged by the
// hydrated desired state.
type appEditSourceWriter func(context.Context, resolvedApplication, application.Manifest) error

// errAppEditPlanDrifted aborts a confirmed Edit when the live Target state no
// longer produces the exact approved plan, before any source or state write.
var errAppEditPlanDrifted = errors.New("provider impact plan changed after confirmation")

// defaultAppEditLifecycleRunner is the real, non-injected lifecycle connection.
func defaultAppEditLifecycleRunner(ctx context.Context, store application.Store, args []string, plan application.ProviderImpactPlan, mutate func(context.Context, resolvedApplication) error, out, errOut io.Writer) ([]application.ProviderExecutionOutcome, error) {
	return executeApplicationApplyLifecycleResults(ctx, store, args, plan, mutate, out, errOut)
}

// defaultAppEditSourceWriter writes confirmed intent to the authoritative source
// without ever clobbering a concurrent or foreign edit. Repository writes are
// guarded by an exact-byte compare and swap against the hydrated original,
// rechecked immediately before the atomic replace. It must only ever run while
// the Target lifecycle lock is held, which serializes every BaseHarbor process
// that could touch this application's state.
func defaultAppEditSourceWriter(originalSource []byte) appEditSourceWriter {
	return func(_ context.Context, resolved resolvedApplication, manifest application.Manifest) error {
		if resolved.FromRepository && resolved.ManifestPath != "" {
			current, err := os.ReadFile(resolved.ManifestPath)
			if err != nil {
				return fmt.Errorf("read application source for edit: %w", err)
			}
			if !bytes.Equal(current, originalSource) {
				return errAppEditSourceForeignEdit
			}
			return atomicWriteApplicationManifest(resolved.ManifestPath, originalSource, []byte(manifest.AuthoredYAML()))
		}
		path := filepath.Join(resolved.Store.Root, manifest.Name, application.RepositoryManifestName)
		current, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read stored application source for edit: %w", err)
		}
		if err == nil && !bytes.Equal(current, originalSource) {
			return errAppEditSourceForeignEdit
		}
		if _, err := resolved.Store.Sync(manifest); err != nil {
			return fmt.Errorf("synchronize edited application manifest: %w", err)
		}
		return nil
	}
}

// appEditRuntimeExecuteAuthority builds the production Edit executor. The
// confirmed source is persisted exactly once, and only after a fresh Target
// read, replan and consent revalidation succeed inside the existing lifecycle
// lock. A source-write or revalidation failure prevents any lifecycle mutation;
// a lifecycle failure still returns every effect genuinely observed before it.
func appEditRuntimeExecuteAuthority(store application.Store, args []string, resolved resolvedApplication, originalSource []byte, authorities appEditAuthorities, out, errOut io.Writer, writeSource appEditSourceWriter, runLifecycle appEditLifecycleRunner) appEditExecuteAuthority {
	if writeSource == nil {
		writeSource = defaultAppEditSourceWriter(originalSource)
	}
	if runLifecycle == nil {
		runLifecycle = defaultAppEditLifecycleRunner
	}
	if authorities.Read == nil || authorities.Choices == nil || authorities.Plan == nil {
		return func(context.Context, appEditDesired, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
			return nil, errors.New("app edit mutation authorities are incomplete")
		}
	}
	return func(ctx context.Context, desired appEditDesired, plan application.ProviderImpactPlan, consent application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if desired.Target != plan.Target || desired.Target != resolved.Target.Name {
			return nil, errAppApplyResultTargetMismatch
		}
		// Nothing above this point may write source or state. The fresh plan,
		// consent and source check all happen inside the lifecycle lock below,
		// immediately before the first state write.
		mutate := func(ctx context.Context, locked resolvedApplication) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if locked.Target.Name != desired.Target {
				return errAppApplyResultTargetMismatch
			}
			view, err := authorities.Read(ctx, desired.Target)
			if err != nil {
				return err
			}
			if view.Target != desired.Target || view.ApplicationID == "" || view.ApplicationID != desired.ApplicationID {
				return errAppApplyResultTargetMismatch
			}
			request := appMachineDesiredRequest{
				Target:           desired.Target,
				SnapshotRevision: view.SnapshotRevision,
				Requirements:     append([]capability.Requirement(nil), desired.Requirements...),
			}
			for _, preference := range desired.Preferences {
				request.Choices = append(request.Choices, appMachineDesiredChoice{
					Requirement: preference.Requirement,
					InstanceID:  preference.InstanceID,
				})
			}
			proposal, err := appMachinePropose(ctx, request, authorities.Read, authorities.Choices, authorities.Plan)
			if err != nil {
				return err
			}
			if proposal.ApplicationID != desired.ApplicationID || !reflect.DeepEqual(proposal.Plan, plan) {
				return errAppEditPlanDrifted
			}
			if err := application.ValidateProviderChangeConsent(plan, consent); err != nil {
				return classifyProviderChangeConsent(err)
			}
			// Persist the confirmed source exactly once, under the lock, after
			// the plan, consent and current ownership all revalidated.
			if err := writeSource(ctx, locked, desired.Manifest); err != nil {
				return fmt.Errorf("persist confirmed application edit: %w", err)
			}
			return nil
		}
		outcomes, runErr := runLifecycle(ctx, store, args, plan, mutate, out, errOut)
		if runErr != nil {
			return outcomes, runErr
		}
		return outcomes, nil
	}
}

// atomicWriteApplicationManifest replaces a repository manifest while preserving
// the existing file permissions and never leaving a partial document. The
// original bytes are rechecked immediately before the atomic rename so a change
// that raced with the compare and staging step still aborts the write. The
// residual window (an external non-cooperating writer renaming between the last
// comparison and the rename) is closed for BaseHarbor processes by the Target
// lifecycle lock that serializes every mutation of this source.
func atomicWriteApplicationManifest(path string, original, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat application manifest before edit: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".baseharbor-edit-*.tmp")
	if err != nil {
		return fmt.Errorf("stage edited application manifest: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write edited application manifest: %w", err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("preserve application manifest permissions: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("flush edited application manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close edited application manifest: %w", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("recheck application source before replace: %w", err)
	}
	if !bytes.Equal(current, original) {
		return errAppEditSourceForeignEdit
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace application manifest: %w", err)
	}
	return nil
}

// defaultAppEditAuthorities assembles the production Edit authorities from the
// accepted read, footprint, choice and impact planning owners. No authority
// persists anything; only the Execute connection mutates state, once, through
// the existing Apply lifecycle.
func defaultAppEditAuthorities(store application.Store, args []string) appEditAuthorities {
	reresolve := func(ctx context.Context, target string) (resolvedApplication, error) {
		resolved, err := resolveApplicationFunc(ctx, store, args, "edit")
		if err != nil {
			return resolvedApplication{}, err
		}
		if resolved.Target.Name != target {
			return resolvedApplication{}, &appMachineError{Code: "conflict"}
		}
		return resolved, nil
	}
	read := func(ctx context.Context, target string) (GuidedReadContext, error) {
		resolved, err := reresolve(ctx, target)
		if err != nil {
			return GuidedReadContext{}, err
		}
		return readGuidedApplicationContext(ctx, resolved)
	}
	choices := func(ctx context.Context, target string, requirements []capability.Requirement) ([]appProviderChoice, error) {
		resolved, err := reresolve(ctx, target)
		if err != nil {
			return nil, err
		}
		snapshot, _, err := applicationLifecycleSnapshot(resolved, observeApplicationProviderFactsFunc(ctx, resolved))
		if err != nil {
			return nil, err
		}
		return appProviderChoices(resolved.Manifest, snapshot, newAppCapabilitySelection(appCapabilitySelectionInput{Current: requirements}))
	}
	planner := func(ctx context.Context, target string, requirements []capability.Requirement, preferences []application.ProviderPreference) (application.ProviderImpactPlan, error) {
		resolved, err := reresolve(ctx, target)
		if err != nil {
			return application.ProviderImpactPlan{}, err
		}
		desired, err := appEditProjectRequirements(resolved.Manifest, requirements)
		if err != nil {
			return application.ProviderImpactPlan{}, err
		}
		if err := desired.Validate(); err != nil {
			return application.ProviderImpactPlan{}, err
		}
		desiredResolved := resolved
		desiredResolved.Manifest = desired
		snapshot, _, err := applicationLifecycleSnapshot(resolved, observeApplicationProviderFactsFunc(ctx, resolved))
		if err != nil {
			return application.ProviderImpactPlan{}, err
		}
		return planApplicationProviderImpactForDesired(ctx, desiredResolved, snapshot, preferences)
	}
	return appEditAuthorities{
		Read:    read,
		Choices: choices,
		Plan:    planner,
		Preview: func(ctx context.Context, desired appEditDesired) (GuidedReadContext, error) {
			resolved, err := reresolve(ctx, desired.Target)
			if err != nil {
				return GuidedReadContext{}, err
			}
			desiredResolved := resolved
			desiredResolved.Manifest = desired.Manifest
			return readGuidedApplicationContext(ctx, desiredResolved)
		},
		Authorize: func(ctx context.Context, target, applicationID string) error {
			view, err := read(ctx, target)
			if err != nil {
				return err
			}
			if view.Target != target || view.ApplicationID != applicationID {
				return &appMachineError{Code: "conflict"}
			}
			return nil
		},
	}
}

// planApplicationProviderImpactForDesired routes the desired manifest through
// the existing request and impact owners without a second executor. An empty
// pending-change set yields the zero plan; callers treat that as drift.
func planApplicationProviderImpactForDesired(ctx context.Context, resolved resolvedApplication, snapshot application.ResolutionSnapshot, preferences []application.ProviderPreference) (application.ProviderImpactPlan, error) {
	union, err := resolveApplicationProviderPlanFromSnapshot(resolved, snapshot, preferences)
	if err != nil {
		return application.ProviderImpactPlan{}, err
	}
	requests := applicationProviderChangeRequests(union)
	requests, err = nativeBackendChangeRequests(resolved, requests)
	if err != nil {
		return application.ProviderImpactPlan{}, err
	}
	requests, err = standaloneProviderVersionChangeRequests(ctx, resolved, requests)
	if err != nil {
		return application.ProviderImpactPlan{}, err
	}
	requests, err = backendBindingChangeRequests(resolved, union, requests)
	if err != nil {
		return application.ProviderImpactPlan{}, err
	}
	requests, err = externalReceiverChangeRequests(resolved, requests)
	if err != nil {
		return application.ProviderImpactPlan{}, err
	}
	requests, err = externalIdentityChangeRequests(resolved, requests)
	if err != nil {
		return application.ProviderImpactPlan{}, err
	}
	if len(requests) == 0 {
		return application.ProviderImpactPlan{}, nil
	}
	return resolveApplicationProviderImpact(ctx, resolved, snapshot, union, requests)
}

// appEditSourceBytes returns the exact authoritative source bytes for a
// resolved application without reformatting them.
func appEditSourceBytes(resolved resolvedApplication) ([]byte, error) {
	if resolved.FromRepository && resolved.ManifestPath != "" {
		data, err := os.ReadFile(resolved.ManifestPath)
		if err != nil {
			return nil, fmt.Errorf("read application source: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(filepath.Join(resolved.Store.Root, resolved.Manifest.Name, application.RepositoryManifestName))
	if err != nil {
		return nil, fmt.Errorf("read stored application source: %w", err)
	}
	return data, nil
}
