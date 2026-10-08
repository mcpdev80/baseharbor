package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/runtime/remoteprojection"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// RemoteManagedRuntime binds Core-generated provider material to one live node.
// Core authorization and provider placement precede this internal boundary.
// Source builds and installation credentials never enter its published bundle.
type RemoteManagedRuntime struct {
	runtime   *targetsession.ProjectRuntime
	kind      string
	name      string
	compose   string
	env       string
	source    []targetsession.ProjectFile
	units     []string
	initUnits []string
	project   *targetsession.StagedProject
	published bool
}

func NewRemoteManagedRuntime(transport targetsession.ProjectTransport, scope targetenrollment.Scope, files RuntimeFiles, manifest Manifest) (*RemoteManagedRuntime, error) {
	projection, err := ProjectManagedRuntime(files, manifest)
	if err != nil {
		return nil, err
	}
	return newRemoteProjectedRuntime(transport, scope, projection)
}

func newRemoteProjectedRuntime(transport targetsession.ProjectTransport, scope targetenrollment.Scope, projection ManagedRuntimeProjection) (*RemoteManagedRuntime, error) {
	runtime, err := targetsession.NewProjectRuntime(transport, scope)
	if err != nil {
		return nil, err
	}
	result := &RemoteManagedRuntime{runtime: runtime, kind: scope.Runtime, name: projection.Project, compose: projection.Compose, env: projection.Env}
	for _, file := range projection.Files {
		result.source = append(result.source, targetsession.ProjectFile{Path: file.Path, Data: append([]byte{}, file.Data...), Mode: file.Mode})
	}
	if scope.Runtime == "podman" {
		graph, err := snapshotManagedQuadlets(projection)
		if err != nil {
			return nil, err
		}
		result.units, result.initUnits = graph.Units, graph.InitUnits
		// Compose source and runtime.env are replaced by the compiled units and
		// per-service environments. Referenced protected TLS files retain modes.
		result.source = nil
		for _, file := range projection.Files {
			if file.Path != projection.Compose && file.Path != projection.Env {
				result.source = append(result.source, targetsession.ProjectFile{Path: file.Path, Data: append([]byte{}, file.Data...), Mode: file.Mode})
			}
		}
		var names []string
		for name := range graph.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			result.source = append(result.source, targetsession.ProjectFile{Path: name, Data: []byte(graph.Files[name]), Mode: 0600})
		}
	}
	return result, nil
}

// ApplyServices converges an explicit phase of the same protected publication.
// Provider and workload phases therefore cannot accidentally start each other.
func (r *RemoteManagedRuntime) ApplyServices(ctx context.Context, services []string, repair bool) error {
	if r == nil || r.project == nil || len(services) == 0 {
		return errors.New("remote application phase has no protected publication")
	}
	if r.kind == "docker" {
		return r.runtime.ApplyComposeSelected(ctx, r.project, []string{r.compose}, r.env, services, repair)
	}
	selected, init, err := r.quadletServices(services)
	if err != nil {
		return err
	}
	if len(init) != 0 {
		return r.runtime.ApplyQuadletInitGraph(ctx, r.project, selected, init)
	}
	return r.runtime.ApplyQuadletGraph(ctx, r.project, selected)
}

// Compile only the protected snapshot, not mutable original Core paths. The
// temporary tree is owner-only and is removed before the constructor returns.
func snapshotManagedQuadlets(projection ManagedRuntimeProjection) (remoteprojection.ProjectedQuadletGraph, error) {
	root, err := os.MkdirTemp("", "baseharbor-managed-projection-")
	if err != nil {
		return remoteprojection.ProjectedQuadletGraph{}, err
	}
	defer os.RemoveAll(root)
	var members []string
	for _, file := range projection.Files {
		destination := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return remoteprojection.ProjectedQuadletGraph{}, err
		}
		if err := os.WriteFile(destination, file.Data, os.FileMode(file.Mode)); err != nil {
			return remoteprojection.ProjectedQuadletGraph{}, err
		}
		members = append(members, file.Path)
	}
	return remoteprojection.ProjectRemoteQuadletInitGraph(filepath.Join(root, projection.Compose), filepath.Join(root, projection.Env), projection.Project, members)
}

// Publish happens once on explicit Core request. The Core must durably commit
// the exact receipt before this handle permits activation. An interrupted or
// failed commit cannot trigger automatic publication replay.
func (r *RemoteManagedRuntime) Publish(ctx context.Context, commit func(targetsession.ProjectRecord) error) error {
	if r == nil || r.runtime == nil || r.project != nil || r.published || commit == nil {
		return errors.New("remote managed publication is unavailable")
	}
	r.published = true
	project, err := r.runtime.Stage(ctx, r.name, r.source)
	if err != nil {
		return err
	}
	if err := commit(project.Record()); err != nil {
		return fmt.Errorf("persist remote managed publication before activation: %w", err)
	}
	r.project = project
	return nil
}

// Restore reuses the exact immutable publication without staging new material.
func (r *RemoteManagedRuntime) Restore(record targetsession.ProjectRecord) error {
	if r == nil || r.runtime == nil {
		return errors.New("remote managed restoration is unavailable")
	}
	r.project = nil
	if record.BundleID != r.name {
		return errors.New("persisted managed project identity differs")
	}
	project, err := r.runtime.RestoreProject(record, r.source)
	if err != nil {
		return err
	}
	r.project = project
	return nil
}

func (r *RemoteManagedRuntime) Record() targetsession.ProjectRecord {
	if r == nil {
		return targetsession.ProjectRecord{}
	}
	return r.project.Record()
}

func (r *RemoteManagedRuntime) Apply(ctx context.Context, repair bool) error {
	if r == nil || r.project == nil {
		return errors.New("remote managed project has no protected publication")
	}
	if r.kind == "docker" {
		return r.runtime.ApplyCompose(ctx, r.project, []string{r.compose}, r.env, repair)
	}
	if len(r.initUnits) != 0 {
		return r.runtime.ApplyQuadletInitGraph(ctx, r.project, r.units, r.initUnits)
	}
	return r.runtime.ApplyQuadletGraph(ctx, r.project, r.units)
}

// Ordinary destroy retains provider data. Explicit reset is a separate Core
// decision, not an implicit side effect of this lifecycle operation.
func (r *RemoteManagedRuntime) Destroy(ctx context.Context) error {
	return r.DestroyOwned(ctx, false)
}

// Explicit persistent data reset carries the same immutable source and exact
// node binding. Ordinary teardown never calls this reset path.
func (r *RemoteManagedRuntime) DestroyOwned(ctx context.Context, reset bool) error {
	if r == nil || r.project == nil {
		return errors.New("remote managed project has no protected publication")
	}
	if r.kind == "docker" {
		return r.runtime.DestroyComposeOwned(ctx, r.project, []string{r.compose}, r.env, reset)
	}
	if reset {
		return r.runtime.ResetQuadletGraph(ctx, r.project, r.units)
	}
	return r.runtime.DestroyQuadletGraph(ctx, r.project, r.units)
}

func (r *RemoteManagedRuntime) Observe(ctx context.Context) ([]targetsession.ProjectService, error) {
	if r == nil || r.project == nil {
		return nil, errors.New("remote managed project has no protected publication")
	}
	return r.runtime.ObserveProject(ctx, r.name)
}

func (r *RemoteManagedRuntime) ExecService(ctx context.Context, project, service string, argv ...string) (string, error) {
	if r == nil || r.project == nil || project != r.name {
		return "", errors.New("remote managed probe selection differs")
	}
	return r.runtime.ExecService(ctx, r.name, service, argv...)
}
