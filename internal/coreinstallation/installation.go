// Package coreinstallation owns the repository-independent Core lifecycle.
package coreinstallation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

type MachineRole string

const (
	Development MachineRole = "development"
	Deployment  MachineRole = "deployment"
)

type Spec struct {
	Target      string      `json:"target"`
	Runtime     string      `json:"runtime"`
	MachineRole MachineRole `json:"machine_role"`
	HA          bool        `json:"ha"`
}
type Defaults struct {
	LocalWorkspaces     bool `json:"local_workspaces"`
	ExplicitSources     bool `json:"explicit_sources"`
	ProtectedManagement bool `json:"protected_management"`
}

func (r MachineRole) Defaults() Defaults {
	return Defaults{LocalWorkspaces: r == Development, ExplicitSources: r == Deployment, ProtectedManagement: r == Deployment}
}

type State struct {
	Version        string          `json:"version"`
	ID             string          `json:"installation_id"`
	Owner          string          `json:"owner"`
	Spec           Spec            `json:"spec"`
	Defaults       Defaults        `json:"defaults"`
	Placement      string          `json:"placement"`
	Phase          string          `json:"phase"`
	Ready          bool            `json:"ready"`
	Capabilities   map[string]bool `json:"capabilities"`
	IdentityIssuer string          `json:"identity_issuer,omitempty"`
}
type Steps struct {
	Preflight func(context.Context, State) error
	SQL       func(context.Context, State) error
	Secrets   func(context.Context, State) error
	Identity  func(context.Context, State) (string, error)
	Verify    func(context.Context, State) error
}

func validate(spec Spec) error {
	if spec.Target == "" || spec.Runtime == "" || (spec.MachineRole != Development && spec.MachineRole != Deployment) {
		return errors.New("invalid Core installation selection or machine role")
	}
	return nil
}
func Load(root string) (State, error) {
	var state State
	path := filepath.Join(root, "installation.json")
	info, err := os.Lstat(path)
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return state, errors.New("Core installation state must be a protected regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return state, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return State{}, errors.New("invalid Core installation state")
	}
	if decoder.Decode(new(any)) != io.EOF || state.Version != "baseharbor.core-installation/v1" || state.Owner != "baseharbor" || stableid.ValidateUUIDv4("installation", state.ID) != nil || state.Placement != "shared" || validate(state.Spec) != nil || state.Defaults != state.Spec.MachineRole.Defaults() || len(state.Capabilities) != 3 {
		return State{}, errors.New("foreign or incompatible Core installation state")
	}
	for _, name := range []string{"sql", "secrets", "identity"} {
		if _, ok := state.Capabilities[name]; !ok || (state.Ready && !state.Capabilities[name]) {
			return State{}, errors.New("Core state lacks a mandatory capability")
		}
	}
	if state.Ready && state.Phase != "ready" {
		return State{}, errors.New("Core readiness state is inconsistent")
	}
	if state.Capabilities["identity"] {
		if err := validateIssuer(state.IdentityIssuer); err != nil {
			return State{}, err
		}
	}
	return state, nil
}

func validateIssuer(issuer string) error {
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("Core Identity requires an HTTPS discovery issuer")
	}
	return nil
}
func save(root string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".installation-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, filepath.Join(root, "installation.json"))
}

// Run records identity before provisioning and verifies again on every retry.
// A failed stage is durable; no retry manufactures a new installation identity.
func Run(ctx context.Context, root string, spec Spec, steps Steps) (State, error) {
	if err := validate(spec); err != nil {
		return State{}, err
	}
	if steps.Preflight == nil || steps.SQL == nil || steps.Secrets == nil || steps.Identity == nil || steps.Verify == nil {
		return State{}, errors.New("complete Core lifecycle steps are required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return State{}, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return State{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return State{}, errors.New("Core state directory must be protected and owned")
	}
	unlock, err := lock(root)
	if err != nil {
		return State{}, err
	}
	defer unlock()
	state, err := Load(root)
	if errors.Is(err, os.ErrNotExist) {
		// Existing pre-freeze runtime state cannot be silently reinterpreted/adopted.
		for _, name := range []string{"compose.yaml", "runtime.env", "topology.json"} {
			if _, e := os.Lstat(filepath.Join(root, name)); e == nil {
				return State{}, errors.New("existing runtime has no authoritative Core installation identity")
			} else if !errors.Is(e, os.ErrNotExist) {
				return State{}, e
			}
		}
		id, e := stableid.NewUUIDv4("installation")
		if e != nil {
			return State{}, e
		}
		state = State{Version: "baseharbor.core-installation/v1", ID: id, Owner: "baseharbor", Spec: spec, Defaults: spec.MachineRole.Defaults(), Placement: "shared", Phase: "preflight", Capabilities: map[string]bool{"sql": false, "secrets": false, "identity": false}}
	} else if err != nil {
		return State{}, err
	}
	if state.Spec != spec {
		return state, errors.New("Core selection conflicts with the owned installation; no replacement performed")
	}
	if err = steps.Preflight(ctx, state); err != nil {
		return state, err
	}
	state.Ready = false
	if err = save(root, state); err != nil {
		return state, err
	}
	actions := []struct {
		name string
		run  func(context.Context, State) error
	}{{"sql", steps.SQL}, {"secrets", steps.Secrets}, {"identity", func(ctx context.Context, s State) error {
		issuer, e := steps.Identity(ctx, s)
		if e == nil {
			e = validateIssuer(issuer)
		}
		if e == nil {
			state.IdentityIssuer = issuer
		}
		return e
	}}, {"verify", steps.Verify}}
	for _, action := range actions {
		if err = ctx.Err(); err != nil {
			return state, err
		}
		state.Phase = action.name
		if err = save(root, state); err != nil {
			return state, err
		}
		if err = action.run(ctx, state); err != nil {
			state.Phase = action.name + "_failed"
			state.Ready = false
			if e := save(root, state); e != nil {
				return state, errors.Join(err, e)
			}
			return state, fmt.Errorf("Core %s failed: %w", action.name, err)
		}
		if action.name != "verify" {
			state.Capabilities[action.name] = true
		}
	}
	state.Phase = "ready"
	state.Ready = true
	return state, save(root, state)
}
