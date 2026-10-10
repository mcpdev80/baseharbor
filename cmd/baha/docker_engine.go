package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type dockerEngineBinding struct {
	Schema      string                            `json:"schema"`
	Target      string                            `json:"target"`
	Observation bhruntime.DockerEngineObservation `json:"engine"`
}

func targetDockerEngineContext(ctx context.Context, target deployment.ResolvedTarget) context.Context {
	if target.DockerEndpoint == "" && target.DockerContext == "" {
		if root, err := targetRuntimeStateRoot(target); err == nil {
			path := filepath.Join(root, "docker-engine.json")
			if record, err := readDockerEngineBinding(path, target.Name); err == nil {
				target.DockerEndpoint = record.Observation.Endpoint
			}
		}
	}
	return bhruntime.WithDockerEngineSelection(ctx, bhruntime.DockerEngineSelection{Endpoint: target.DockerEndpoint, Context: target.DockerContext, Mode: target.DockerMode})
}

func validateTargetDockerBinding(ctx context.Context, target deployment.ResolvedTarget, provider bhruntime.RuntimeProvider, create bool) error {
	engine := bhruntime.DockerEngineForProvider(provider)
	if engine == nil {
		return nil
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "docker-engine.json")
	_, err = os.Lstat(path)
	if err == nil {
		record, err := readDockerEngineBinding(path, target.Name)
		if err != nil {
			return err
		}
		if record.Observation.Endpoint != engine.Endpoint || record.Observation.Mode != engine.Mode || record.Observation.DaemonID != engine.DaemonID {
			return fmt.Errorf("Target %s belongs to Docker %s (%s, daemon %s), but selected engine is %s (%s, daemon %s); no migration, duplicate bootstrap or cleanup performed; configure the Target to its recorded engine", target.Name, record.Observation.Endpoint, record.Observation.Mode, record.Observation.DaemonID, engine.Endpoint, engine.Mode, engine.DaemonID)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Existing protected provider definitions cannot be reused on a different
	// engine simply because Docker reports no containers at the new endpoint.
	entries, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	legacy := false
	for _, entry := range entries {
		if entry.Name() != "docker-engine.json" {
			legacy = true
			break
		}
	}
	if legacy {
		owned, err := provider.ListOwnedProjectResources(ctx, targetRuntimeProjectName(target))
		if err != nil || len(owned) == 0 {
			return fmt.Errorf("Target %s has existing Core state without a Docker engine binding, but ownership at %s is not proven; existing rootful resources are preserved; explicitly configure the original Docker endpoint/context and mode before continuing", target.Name, engine.Endpoint)
		}
	}
	if !create {
		return nil
	}
	if engine.Mode == "rootless" && engine.Endpoint != "unix:///var/run/docker.sock" {
		// This is discovery only. A reachable system daemon with an already
		// owned same-Target project must not be hidden by a fresh XDG root.
		otherCtx := bhruntime.WithDockerEngineSelection(ctx, bhruntime.DockerEngineSelection{Endpoint: "unix:///var/run/docker.sock", Mode: "rootful"})
		if other, err := bhruntime.ResolveDockerEngine(otherCtx, "docker"); err == nil && other.DaemonID != engine.DaemonID {
			backend := bhruntime.NewDockerCLIBackend("docker", other)
			resources, err := backend.ListOwnedProjectResources(ctx, targetRuntimeProjectName(target))
			if err != nil {
				return fmt.Errorf("System Docker is reachable but existing Target ownership could not be inspected; no Core bootstrap performed")
			}
			if len(resources) > 0 {
				return fmt.Errorf("Target %s already has owned resources on System Docker at %s; no duplicate Core bootstrap or migration performed; explicitly select the original engine or a new Target", target.Name, other.Endpoint)
			}
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return validateTargetDockerBinding(ctx, target, provider, false)
	}
	if err != nil {
		return err
	}
	writeErr := json.NewEncoder(f).Encode(dockerEngineBinding{Schema: "baseharbor.docker-engine/v1", Target: target.Name, Observation: *engine})
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func ensureTargetDockerBinding(ctx context.Context, target deployment.ResolvedTarget) error {
	if target.RuntimeProvider != "docker" || (target.AccessProvider != "" && target.AccessProvider != "local") {
		return nil
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	return validateTargetDockerBinding(ctx, target, provider, true)
}

func inspectTargetDockerEngine(ctx context.Context, target deployment.ResolvedTarget) (*bhruntime.DockerEngineObservation, error) {
	if target.RuntimeProvider != "docker" || (target.AccessProvider != "" && target.AccessProvider != "local") {
		return nil, nil
	}
	engine, err := bhruntime.ResolveDockerEngine(targetDockerEngineContext(ctx, target), "docker")
	return &engine, err
}

func renderDockerEngine(out io.Writer, engine *bhruntime.DockerEngineObservation) {
	if engine != nil {
		fmt.Fprintf(out, "Docker endpoint  %s\nDocker mode      %s\nDocker daemon    %s\n", engine.Endpoint, engine.Mode, engine.DaemonID)
	}
}

func readDockerEngineBinding(path, target string) (dockerEngineBinding, error) {
	var record dockerEngineBinding
	info, err := os.Lstat(path)
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return record, errors.New("protected Docker engine binding is invalid; preserve installation state and inspect ownership")
	}
	f, err := os.Open(path)
	if err != nil {
		return record, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return record, errors.New("Docker engine binding changed while opening")
	}
	decoder := json.NewDecoder(io.LimitReader(f, 16385))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || record.Schema != "baseharbor.docker-engine/v1" || record.Target != target || !record.Observation.Verified || record.Observation.DaemonID == "" {
		return record, errors.New("protected Docker engine binding could not be verified")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return record, errors.New("protected Docker engine binding has trailing data")
	}
	return record, nil
}
