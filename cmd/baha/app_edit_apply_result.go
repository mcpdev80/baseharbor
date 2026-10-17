package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// appEditLifecycleRunner is the concrete Apply lifecycle connection used by the
// production Edit executor. The default value binds directly to
// executeApplicationApplyLifecycleResults so the receipt path is not an
// exclusively injected callback; tests may substitute it only to observe the
// single invocation and preserve compatibility.
type appEditLifecycleRunner func(context.Context, application.Store, []string, application.ProviderImpactPlan, io.Writer, io.Writer) ([]application.ProviderExecutionOutcome, error)

// appEditSourceWriter persists confirmed Edit intent through the existing
// source-of-truth owner. Repository applications keep their own manifest file;
// store-managed applications use the accepted internal-copy synchronization.
// Untouched manifest intent is carried unchanged by the hydrated desired state.
type appEditSourceWriter func(context.Context, resolvedApplication, application.Manifest) error

// defaultAppEditLifecycleRunner is the real, non-injected lifecycle connection.
func defaultAppEditLifecycleRunner(ctx context.Context, store application.Store, args []string, plan application.ProviderImpactPlan, out, errOut io.Writer) ([]application.ProviderExecutionOutcome, error) {
	return executeApplicationApplyLifecycleResults(ctx, store, args, plan, out, errOut)
}

// defaultAppEditSourceWriter writes confirmed intent to the authoritative source
// without ever clobbering a concurrent or foreign edit. Repository writes are
// guarded by an exact-byte compare-and-swap against the hydrated original.
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
			return atomicWriteApplicationManifest(resolved.ManifestPath, []byte(manifest.AuthoredYAML()))
		}
		if _, err := resolved.Store.Sync(manifest); err != nil {
			return fmt.Errorf("synchronize edited application manifest: %w", err)
		}
		return nil
	}
}

// appEditRuntimeExecuteAuthority builds the production Edit executor. It runs the
// existing lifecycle exactly once per confirmed apply: the confirmed source is
// persisted first, then the accepted lifecycle converges and verifies it, and the
// actual native observations are bound to the approved plan. A source-write or
// pre-mutation failure prevents the lifecycle from running; a lifecycle failure
// still returns every effect genuinely observed before it.
func appEditRuntimeExecuteAuthority(store application.Store, args []string, resolved resolvedApplication, originalSource []byte, out, errOut io.Writer, writeSource appEditSourceWriter, runLifecycle appEditLifecycleRunner) appEditExecuteAuthority {
	if writeSource == nil {
		writeSource = defaultAppEditSourceWriter(originalSource)
	}
	if runLifecycle == nil {
		runLifecycle = defaultAppEditLifecycleRunner
	}
	return func(ctx context.Context, desired appEditDesired, plan application.ProviderImpactPlan, consent application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if desired.Target != plan.Target || desired.Target != resolved.Target.Name {
			return nil, errAppApplyResultTargetMismatch
		}
		if err := application.ValidateProviderChangeConsent(plan, consent); err != nil {
			return nil, classifyProviderChangeConsent(err)
		}
		// Persist the confirmed source exactly once before the lifecycle reads it.
		if err := writeSource(ctx, resolved, desired.Manifest); err != nil {
			return nil, fmt.Errorf("persist confirmed application edit: %w", err)
		}
		outcomes, runErr := runLifecycle(ctx, store, args, plan, out, errOut)
		if runErr != nil {
			return outcomes, runErr
		}
		return outcomes, nil
	}
}

// atomicWriteApplicationManifest replaces a repository manifest in place while
// preserving the existing file permissions and never leaving a partial document.
func atomicWriteApplicationManifest(path string, data []byte) error {
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
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace application manifest: %w", err)
	}
	return nil
}
