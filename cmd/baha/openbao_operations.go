package main

import (
	"context"
	"errors"
	"fmt"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
)

type openBaoOperationResult struct {
	Initialized  bool `json:"initialized"`
	Unsealed     bool `json:"unsealed"`
	ManagerReady bool `json:"manager_ready"`
}

func inspectManagedOpenBao(ctx context.Context) (openBaoOperationResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "openbao.status", "", "", ""); err != nil {
		return openBaoOperationResult{}, err
	}
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		return openBaoOperationResult{}, err
	}
	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return openBaoOperationResult{}, err
	}
	result := openBaoOperationResult{Initialized: state.Initialized, Unsealed: state.Initialized && !state.Sealed}
	if result.Unsealed {
		result.ManagerReady = platformopenbao.CheckManager(ctx, compose, files) == nil
	}
	return result, nil
}
func bootstrapManagedOpenBao(ctx context.Context, recoveryPath string) error {
	if err := authorizeCurrentMCPContext(ctx, "openbao.bootstrap", "", "", ""); err != nil {
		return err
	}
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{files.Compose, files.Env} {
		if err := ownerOnly(path); err != nil {
			return fmt.Errorf("OpenBao bootstrap preflight: %w", err)
		}
	}
	if err := platformopenbao.Bootstrap(ctx, compose, files, recoveryPath); err != nil {
		return err
	}
	if err := persistTargetRecoveryFileReference(ctx, recoveryPath); err != nil {
		return fmt.Errorf("persist OpenBao recovery-file reference: %w", err)
	}
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files, recoveryPath); err != nil {
		return fmt.Errorf("reconcile control-plane service access after OpenBao bootstrap: %w", err)
	}
	return nil
}

func unsealManagedOpenBao(ctx context.Context, recoveryPath string) error {
	if err := authorizeCurrentMCPContext(ctx, "openbao.unseal", "", "", ""); err != nil {
		return err
	}
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		return err
	}
	if err := platformopenbao.Unseal(ctx, compose, files, recoveryPath); err != nil {
		return err
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return errors.New("OpenBao unsealed but manager authentication verification failed")
	}
	if err := reconcileControlPlaneServiceAccess(ctx, compose, files, recoveryPath); err != nil {
		return fmt.Errorf("reconcile control-plane service access after OpenBao unseal: %w", err)
	}
	return nil
}

func rotateManagedOpenBao(ctx context.Context, recoveryPath string) error {
	if err := authorizeCurrentMCPContext(ctx, "openbao.rotate", "", "", ""); err != nil {
		return err
	}
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		return err
	}
	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return err
	}
	if !state.Initialized {
		return platformopenbao.ErrNotInitialized
	}
	if state.Sealed {
		return platformopenbao.ErrSealed
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return fmt.Errorf("verify OpenBao manager before rotation: %w", err)
	}

	if err := rotateControlPlaneDatabaseCredentials(ctx, compose, files, recoveryPath); err != nil {
		return fmt.Errorf("rotate control-plane database credentials: %w", err)
	}
	if err := platformopenbao.RotateManagerCredentials(ctx, compose, files); err != nil {
		return fmt.Errorf("rotate OpenBao manager credential: %w", err)
	}
	if err := rotateControlPlaneServiceCA(ctx, compose, files, recoveryPath); err != nil {
		return fmt.Errorf("rotate control-plane managed service CA: %w", err)
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		return fmt.Errorf("verify OpenBao manager after rotation: %w", err)
	}
	if err := verifyOpenBaoManagementUI(ctx, files); err != nil {
		return fmt.Errorf("verify OpenBao management UI after rotation: %w", err)
	}

	return nil
}
