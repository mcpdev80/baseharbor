package openbao

import (
	"context"
	"encoding/json"
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
	UpgradePath    func(context.Context, string, string) error
	InspectMembers func(context.Context) ([]State, error)
	Backup         func(context.Context, string) (providerupgrade.BackupRef, error)
	VerifyBackup   func(context.Context, providerupgrade.BackupRef) error
	Apply          func(context.Context, string, string, string) error
	Unseal         func(context.Context) error
	VerifyAuth     func(context.Context) error
	VerifyApps     func(context.Context) error
	Restore        func(context.Context, providerupgrade.BackupRef, string) error
}

// NativeOps binds the existing BaseHarbor OpenBao lifecycle to the isolated
// provider adapter. The central Core orchestrator owns quiescence and SQL data
// volume snapshotting; this implementation never guesses an owned volume.
type NativeOps struct {
	Executor native.Executor
	Files    bhruntime.Files
	Owner    string
	Hooks    RuntimeHooks
}

var _ Ops = (*NativeOps)(nil)

func (n *NativeOps) Inspect(ctx context.Context) (State, error) {
	if n == nil || n.Executor == nil || n.Files.Compose == "" || n.Files.Env == "" {
		return State{}, errors.New("OpenBao runtime executor and Core files are required")
	}
	state, err := native.Inspect(ctx, n.Executor, n.Files)
	if err != nil {
		return State{}, fmt.Errorf("inspect managed OpenBao: %w", err)
	}
	members := n.Files.OpenBaoMembers()
	if len(members) == 0 {
		return State{}, errors.New("OpenBao member inventory is empty")
	}
	topology := "single"
	if len(members) > 1 {
		topology = "ha"
		probe := n.Hooks.InspectMembers
		if probe == nil {
			probe = n.inspectMembersNative
		}
		states, err := probe(ctx)
		if err != nil {
			return State{}, fmt.Errorf("verify OpenBao HA members: %w", err)
		}
		if err := verifyHAMembers(states, len(members), state.Version); err != nil {
			return State{}, err
		}
	}
	return State{
		Version:     strings.TrimSpace(state.Version),
		Initialized: state.Initialized,
		Sealed:      state.Sealed,
		Healthy:     state.Initialized && !state.Sealed,
		Owner:       n.Owner,
		Topology:    topology,
	}, nil
}

func (n *NativeOps) CheckUpgradePath(ctx context.Context, from, to string) error {
	if n.Hooks.UpgradePath == nil {
		return errors.New("provider upgrade compatibility evidence is unavailable")
	}
	return n.Hooks.UpgradePath(ctx, from, to)
}
func (n *NativeOps) CreateBackup(ctx context.Context, version string) (providerupgrade.BackupRef, error) {
	if n.Hooks.Backup == nil {
		return providerupgrade.BackupRef{}, errors.New("owned OpenBao SQL snapshot hook is unavailable")
	}
	return n.Hooks.Backup(ctx, version)
}
func (n *NativeOps) VerifyBackup(ctx context.Context, backup providerupgrade.BackupRef) error {
	if n.Hooks.VerifyBackup == nil {
		return errors.New("OpenBao backup verification hook is unavailable")
	}
	return n.Hooks.VerifyBackup(ctx, backup)
}
func (n *NativeOps) ApplyTarget(ctx context.Context, version, image, digest string) error {
	if n.Hooks.Apply == nil {
		return errors.New("OpenBao pinned reconcile hook is unavailable")
	}
	return n.Hooks.Apply(ctx, version, image, digest)
}
func (n *NativeOps) WaitHealthy(ctx context.Context) error {
	state, err := n.Inspect(ctx)
	if err != nil {
		return err
	}
	if !state.Healthy {
		return errors.New("OpenBao is not healthy and unsealed")
	}
	return nil
}
func (n *NativeOps) EnsureUnsealed(ctx context.Context) error {
	state, err := n.Inspect(ctx)
	if err != nil {
		return err
	}
	if state.Initialized && !state.Sealed {
		return nil
	}
	if n.Hooks.Unseal == nil {
		return errors.New("OpenBao unseal requires an authorized recovery hook")
	}
	if err := n.Hooks.Unseal(ctx); err != nil {
		return err
	}
	state, err = n.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("verify OpenBao state after unseal: %w", err)
	}
	return verifyRecoveredState(state)
}

func verifyRecoveredState(state State) error {
	if !state.Initialized || state.Sealed || !state.Healthy {
		return errors.New("OpenBao remained sealed, uninitialized or unhealthy after recovery")
	}
	return nil
}

func (n *NativeOps) VerifyManagerAuth(ctx context.Context) error {
	if n.Executor == nil {
		return errors.New("OpenBao executor is unavailable")
	}
	return native.CheckManager(ctx, n.Executor, n.Files)
}
func (n *NativeOps) VerifyAuthConfiguration(ctx context.Context) error {
	if n.Hooks.VerifyAuth == nil {
		return errors.New("OpenBao AppRole/policy validation hook is unavailable")
	}
	return n.Hooks.VerifyAuth(ctx)
}
func (n *NativeOps) VerifyApplicationAccess(ctx context.Context) error {
	if n.Hooks.VerifyApps == nil {
		return errors.New("OpenBao application credential validation hook is unavailable")
	}
	return n.Hooks.VerifyApps(ctx)
}
func (n *NativeOps) RestoreBackup(ctx context.Context, backup providerupgrade.BackupRef, version string) error {
	if n.Hooks.Restore == nil {
		return errors.New("owned OpenBao recovery hook is unavailable")
	}
	return n.Hooks.Restore(ctx, backup, version)
}

// verifyHAMembers prevents a single reachable active member from masking an
// unavailable, sealed or mixed-version HA replica during a provider upgrade.
func verifyHAMembers(states []State, expected int, version string) error {
	if expected < 2 || len(states) != expected {
		return errors.New("OpenBao HA member count does not match managed topology")
	}
	for _, member := range states {
		if member.Version != version || !member.Initialized || member.Sealed || !member.Healthy {
			return errors.New("OpenBao HA member version, initialization, seal or health differs")
		}
	}
	return nil
}

func (n *NativeOps) inspectMembersNative(ctx context.Context) ([]State, error) {
	members := n.Files.OpenBaoMembers()
	if len(members) == 0 {
		return nil, errors.New("managed OpenBao member inventory is empty")
	}
	states := make([]State, 0, len(members))
	for _, service := range members {
		// Exit 2 means an initialized but sealed OpenBao node; preserve its
		// JSON state and reject it in verifyHAMembers, not as a shell failure.
		script := "rc=0; BAO_ADDR=https://127.0.0.1:8200 bao status -format=json || rc=$?; if [ \"$rc\" -eq 0 ] || [ \"$rc\" -eq 2 ]; then exit 0; fi; exit \"$rc\""
		out, err := n.Executor.ExecProject(ctx, n.Files.Project, n.Files.Compose, n.Files.Env, service, "sh", "-c", script)
		if err != nil {
			return nil, errors.New("OpenBao HA member probe failed")
		}
		var st struct {
			Version     string `json:"version"`
			Initialized bool   `json:"initialized"`
			Sealed      bool   `json:"sealed"`
		}
		if err := json.Unmarshal([]byte(out), &st); err != nil {
			return nil, errors.New("OpenBao HA member returned invalid status")
		}
		states = append(states, State{Version: st.Version, Initialized: st.Initialized, Sealed: st.Sealed, Healthy: st.Initialized && !st.Sealed})
	}
	return states, nil
}
