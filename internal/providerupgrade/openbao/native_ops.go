package openbao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	native "github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RuntimeHooks provide the application-specific, owned recovery and access checks.
// Missing hooks fail closed. Secrets and unseal keys must never be included in
// arguments, error messages or log output.
type RuntimeHooks struct {
	UpgradePath func(context.Context, string, string) error
	Backup func(context.Context, string) (providerupgrade.BackupRef, error)
	VerifyBackup func(context.Context, providerupgrade.BackupRef) error
	Apply func(context.Context, string, string, string) error
	Unseal func(context.Context) error
	VerifyAuth func(context.Context) error
	VerifyApps func(context.Context) error
	Restore func(context.Context, providerupgrade.BackupRef, string) error
}

// NativeOps binds the existing BaseHarbor OpenBao lifecycle to the isolated
// provider adapter. The central Core orchestrator owns quiescence and SQL data
// volume snapshotting; this implementation never guesses an owned volume.
type NativeOps struct {
	Executor native.Executor
	Files bhruntime.Files
	Owner string
	Hooks RuntimeHooks
}

var _ Ops = (*NativeOps)(nil)

func (n *NativeOps) Inspect(ctx context.Context) (State, error) {
	if n == nil || n.Executor == nil || n.Files.Compose == "" || n.Files.Env == "" {
		return State{}, errors.New("OpenBao runtime executor and Core files are required")
	}
	state, err := native.Inspect(ctx, n.Executor, n.Files)
	if err != nil { return State{}, fmt.Errorf("inspect managed OpenBao: %w", err) }
	members := n.Files.OpenBaoMembers()
	if len(members) == 0 { return State{}, errors.New("OpenBao member inventory is empty") }
	topology := "single"
	if len(members) > 1 { topology = "ha" }
	return State{
		Version: strings.TrimSpace(state.Version),
		Initialized: state.Initialized,
		Sealed: state.Sealed,
		Healthy: state.Initialized && !state.Sealed,
		Owner: n.Owner,
		Topology: topology,
	}, nil
}

func (n *NativeOps) CheckUpgradePath(ctx context.Context, from, to string) error {
	if n.Hooks.UpgradePath == nil { return errors.New("provider upgrade compatibility evidence is unavailable") }
	return n.Hooks.UpgradePath(ctx, from, to)
}
func (n *NativeOps) CreateBackup(ctx context.Context, version string) (providerupgrade.BackupRef, error) {
	if n.Hooks.Backup == nil { return providerupgrade.BackupRef{}, errors.New("owned OpenBao SQL snapshot hook is unavailable") }
	return n.Hooks.Backup(ctx, version)
}
func (n *NativeOps) VerifyBackup(ctx context.Context, backup providerupgrade.BackupRef) error {
	if n.Hooks.VerifyBackup == nil { return errors.New("OpenBao backup verification hook is unavailable") }
	return n.Hooks.VerifyBackup(ctx, backup)
}
func (n *NativeOps) ApplyTarget(ctx context.Context, version, image, digest string) error {
	if n.Hooks.Apply == nil { return errors.New("OpenBao pinned reconcile hook is unavailable") }
	return n.Hooks.Apply(ctx, version, image, digest)
}
func (n *NativeOps) WaitHealthy(ctx context.Context) error {
	state, err := n.Inspect(ctx)
	if err != nil { return err }
	if !state.Healthy { return errors.New("OpenBao is not healthy and unsealed") }
	return nil
}
func (n *NativeOps) EnsureUnsealed(ctx context.Context) error {
	state, err := n.Inspect(ctx)
	if err != nil { return err }
	if state.Initialized && !state.Sealed { return nil }
	if n.Hooks.Unseal == nil { return errors.New("OpenBao unseal requires an authorized recovery hook") }
	return n.Hooks.Unseal(ctx)
}
func (n *NativeOps) VerifyManagerAuth(ctx context.Context) error {
	if n.Executor == nil { return errors.New("OpenBao executor is unavailable") }
	return native.CheckManager(ctx, n.Executor, n.Files)
}
func (n *NativeOps) VerifyAuthConfiguration(ctx context.Context) error {
	if n.Hooks.VerifyAuth == nil { return errors.New("OpenBao AppRole/policy validation hook is unavailable") }
	return n.Hooks.VerifyAuth(ctx)
}
func (n *NativeOps) VerifyApplicationAccess(ctx context.Context) error {
	if n.Hooks.VerifyApps == nil { return errors.New("OpenBao application credential validation hook is unavailable") }
	return n.Hooks.VerifyApps(ctx)
}
func (n *NativeOps) RestoreBackup(ctx context.Context, backup providerupgrade.BackupRef, version string) error {
	if n.Hooks.Restore == nil { return errors.New("owned OpenBao recovery hook is unavailable") }
	return n.Hooks.Restore(ctx, backup, version)
}
