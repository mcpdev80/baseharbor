package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Opt-in native fixtures own isolated installation state. Full fixture cleanup
// now explicitly acknowledges its current plan, instead of using --yes as an
// implicit data grant. Restore only the test's Target selectors for later phases;
// this helper is not product recovery or a claimed consumer-continuity path.
func approvedNativeFixtureCleanup(t *testing.T, ctx context.Context, out io.Writer) error {
	t.Helper()
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return err
	}
	config, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	targets, discovery := discoverFullDestroyTargets()
	if countFullDestroyBlockers(discovery) > 0 {
		return invalidProviderConsumerInventory(io.ErrUnexpectedEOF)
	}
	plans, err := collectFullDestroyInventory(ctx, targets)
	if err != nil {
		return err
	}
	preserved := []fullDestroyResult{}
	for _, target := range targets {
		item, err := preservedTargetRecovery(ctx, target)
		if err != nil {
			return err
		}
		if item.Status != "" {
			preserved = append(preserved, item)
		}
	}
	p, err := buildRuntimeDestroyImpact(ctx, plans, preserved, true)
	if err != nil {
		return err
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
	var destroyOutput bytes.Buffer
	if err := destroyInstallation(withProviderChangeConsent(machineNoninteractiveContext(ctx), ack), true, &destroyOutput, &destroyOutput); err != nil {
		logNativeFixtureCleanupMismatch(t, p, err)
		if diagnostic := nativeFixtureCleanupDiagnostic(err); diagnostic != nil {
			t.Logf("[native-cleanup-diagnostic] caller=%s owner=%s classification=%s",
				diagnostic.Caller, diagnostic.Owner, diagnostic.Classification)
		}
		return err
	}
	if config == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(configPath, config, 0600)
}
