package openbao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

type State struct {
	Version     string
	Initialized bool
	Sealed      bool
	Healthy     bool
	Owner       string
	Topology    string
}

type Ops interface {
	Inspect(context.Context) (State, error)
	CheckUpgradePath(context.Context, string, string) error
	CreateBackup(context.Context, string) (providerupgrade.BackupRef, error)
	VerifyBackup(context.Context, providerupgrade.BackupRef) error
	ApplyTarget(context.Context, string, string, string) error
	WaitHealthy(context.Context) error
	EnsureUnsealed(context.Context) error
	VerifyManagerAuth(context.Context) error
	VerifyAuthConfiguration(context.Context) error
	VerifyApplicationAccess(context.Context) error
	RestoreBackup(context.Context, providerupgrade.BackupRef, string) error
}

type Adapter struct {
	ops Ops
}

func New(ops Ops) *Adapter {
	return &Adapter{ops: ops}
}

func (a *Adapter) Inventory(ctx context.Context) (providerupgrade.Inventory, error) {
	if a == nil || a.ops == nil {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "openbao inventory", errors.New("operations are required"))
	}
	state, err := a.ops.Inspect(ctx)
	if err != nil {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "openbao inspect", err)
	}
	if strings.TrimSpace(state.Version) == "" {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "openbao inspect", errors.New("version is not observable"))
	}
	return providerupgrade.Inventory{
		Provider: providerupgrade.ProviderOpenBao,
		Version:  state.Version,
		Topology: state.Topology,
		Owner:    state.Owner,
		Healthy:  state.Healthy && state.Initialized && !state.Sealed,
		Details: map[string]string{
			"initialized": fmt.Sprintf("%t", state.Initialized),
			"sealed":      fmt.Sprintf("%t", state.Sealed),
		},
	}, nil
}

func (a *Adapter) Preflight(ctx context.Context, req providerupgrade.Request) (providerupgrade.Assessment, error) {
	if err := req.Validate(); err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "openbao preflight", err)
	}
	inventory, err := a.Inventory(ctx)
	if err != nil {
		return providerupgrade.Assessment{}, err
	}
	if inventory.Owner != "baseharbor" {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "openbao preflight", errors.New("foreign OpenBao realization must not be mutated"))
	}
	if inventory.Version != req.CurrentVersion {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "openbao preflight", fmt.Errorf("observed version %q differs from planned %q", inventory.Version, req.CurrentVersion))
	}
	if !inventory.Healthy {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "openbao preflight", errors.New("OpenBao must be initialized, healthy and unsealed before upgrade"))
	}
	current, err := providerupgrade.ParseVersion(req.CurrentVersion)
	if err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "openbao version", err)
	}
	target, err := providerupgrade.ParseVersion(req.TargetVersion)
	if err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "openbao version", err)
	}
	switch current.Compare(target) {
	case 1:
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "openbao preflight", errors.New("downgrade is not supported"))
	case 0:
		return providerupgrade.Assessment{Classification: providerupgrade.ClassificationNoChange, Reason: "OpenBao version unchanged"}, nil
	}
	if err := a.ops.CheckUpgradePath(ctx, req.CurrentVersion, req.TargetVersion); err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "openbao compatibility", err)
	}
	return providerupgrade.Assessment{
		Classification: providerupgrade.ClassificationSupported,
		Reason:         "provider compatibility accepted; verified backup required",
		BackupRequired: true,
	}, nil
}

func (a *Adapter) Backup(ctx context.Context, req providerupgrade.Request) (providerupgrade.BackupRef, error) {
	assessment, err := a.Preflight(ctx, req)
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if assessment.Classification == providerupgrade.ClassificationNoChange {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupRequired, "openbao backup", errors.New("backup is not required for an unchanged provider"))
	}
	backup, err := a.ops.CreateBackup(ctx, req.CurrentVersion)
	if err != nil {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "openbao create backup", err)
	}
	if backup.Provider == "" {
		backup.Provider = providerupgrade.ProviderOpenBao
	}
	if backup.Version == "" {
		backup.Version = req.CurrentVersion
	}
	if err := backup.Validate(providerupgrade.ProviderOpenBao, req.CurrentVersion); err != nil {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "openbao backup", err)
	}
	if err := a.ops.VerifyBackup(ctx, backup); err != nil {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "openbao verify backup", err)
	}
	return backup, nil
}

func (a *Adapter) Execute(ctx context.Context, req providerupgrade.Request, backup providerupgrade.BackupRef) error {
	assessment, err := a.Preflight(ctx, req)
	if err != nil {
		return err
	}
	if assessment.Classification == providerupgrade.ClassificationNoChange {
		return nil
	}
	if err := backup.Validate(providerupgrade.ProviderOpenBao, req.CurrentVersion); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorBackupRequired, "openbao execute", err)
	}
	if err := a.ops.VerifyBackup(ctx, backup); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "openbao execute", err)
	}
	if err := a.ops.ApplyTarget(ctx, req.TargetVersion, req.TargetImage, req.TargetDigest); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "openbao apply", err)
	}
	if err := a.ops.WaitHealthy(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "openbao health", err)
	}
	if err := a.ops.EnsureUnsealed(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "openbao unseal", err)
	}
	return nil
}

func (a *Adapter) Verify(ctx context.Context, req providerupgrade.Request) error {
	state, err := a.ops.Inspect(ctx)
	if err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "openbao inspect", err)
	}
	if state.Version != req.TargetVersion {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "openbao version", fmt.Errorf("running %q, expected %q", state.Version, req.TargetVersion))
	}
	if !state.Initialized || state.Sealed || !state.Healthy {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "openbao state", errors.New("OpenBao is not healthy, initialized and unsealed"))
	}
	if err := a.ops.VerifyManagerAuth(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "openbao manager auth", err)
	}
	if err := a.ops.VerifyAuthConfiguration(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "openbao auth configuration", err)
	}
	if err := a.ops.VerifyApplicationAccess(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "openbao application secret access", err)
	}
	return nil
}

func (a *Adapter) Recover(ctx context.Context, req providerupgrade.Request, backup providerupgrade.BackupRef) error {
	if err := backup.Validate(providerupgrade.ProviderOpenBao, req.CurrentVersion); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "openbao recover", err)
	}
	if err := a.ops.VerifyBackup(ctx, backup); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "openbao recover", err)
	}
	if err := a.ops.RestoreBackup(ctx, backup, req.CurrentVersion); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "openbao restore backup and prior image", err)
	}
	state, err := a.ops.Inspect(ctx)
	if err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "openbao recovered inspect", err)
	}
	if state.Version != req.CurrentVersion || !state.Initialized || state.Sealed || !state.Healthy {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "openbao recovered state", errors.New("recovered OpenBao is not healthy on the original version"))
	}
	if err := a.ops.VerifyManagerAuth(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "openbao recovered auth", err)
	}
	if err := a.ops.VerifyApplicationAccess(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "openbao recovered application access", err)
	}
	return nil
}
