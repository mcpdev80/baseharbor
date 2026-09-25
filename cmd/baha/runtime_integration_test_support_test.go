package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
)

var (
	runtimeIntegrationTrustPlaneOnce sync.Once
	runtimeIntegrationTrustPlaneErr  error
)

func ensureRuntimeIntegrationTrustPlane(t *testing.T, ctx context.Context) {
	t.Helper()
	runtimeIntegrationTrustPlaneOnce.Do(func() {
		configHome, err := os.MkdirTemp("", "baseharbor-ci-config-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		dataHome, err := os.MkdirTemp("", "baseharbor-ci-data-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("XDG_CONFIG_HOME", configHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("XDG_DATA_HOME", dataHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		_ = os.Unsetenv("BASEHARBOR_TARGET")

		var out bytes.Buffer
		if err := runWithIO(ctx, []string{"up", "--control-plane-only", "--yes"}, &out, &out); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}

		compose, files, err := openBaoRuntime(ctx)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		state, err := platformopenbao.Inspect(ctx, compose, files)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if state.Initialized {
			if state.Sealed {
				runtimeIntegrationTrustPlaneErr = os.ErrPermission
				return
			}
			runtimeIntegrationTrustPlaneErr = platformopenbao.CheckManager(ctx, compose, files)
			return
		}

		dir, err := os.MkdirTemp("", "baseharbor-ci-openbao-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		recoveryPath := filepath.Join(dir, "recovery.json")
		out.Reset()
		runtimeIntegrationTrustPlaneErr = runWithIO(ctx, []string{"openbao", "bootstrap", "--recovery-file", recoveryPath}, &out, &out)
	})
	if runtimeIntegrationTrustPlaneErr != nil {
		t.Fatalf("prepare BaseHarbor integration trust plane: %v", runtimeIntegrationTrustPlaneErr)
	}
	if _, err := existingTargetRuntimeFiles(ctx); err != nil {
		t.Fatalf("BaseHarbor integration trust plane state missing: %v", err)
	}
}
