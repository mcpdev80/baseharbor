package development

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspaceSupportsMultiRepoSubpathsAndOCI(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "baseharbor.yaml")
	if err := os.WriteFile(manifest, []byte("version: 1\napp:\n  name: shop\n  environment: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	frontend := filepath.Join(root, "frontend")
	backend := filepath.Join(root, "backend")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(backend, "services", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(backend, "workers", "job"), 0o755); err != nil {
		t.Fatal(err)
	}

	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{
			{ID: "frontend-source", Type: SourceRepository, Repository: "https://git.example/frontend.git", Ref: "main"},
			{ID: "backend-source", Type: SourceRepository, Repository: "https://git.example/backend.git"},
			{ID: "external-source", Type: SourceOCI, Image: "ghcr.io/example/external@sha256:deadbeef"},
		},
		Components: []ComponentSource{
			{Component: "frontend", Source: "frontend-source"},
			{Component: "api", Source: "backend-source", SubPath: "services/api"},
			{Component: "worker", Source: "backend-source", SubPath: "workers/job"},
			{Component: "external", Source: "external-source"},
		},
	}
	mapping := WorkspaceMapping{
		SchemaVersion: WorkspaceMappingVersion,
		Application:   "shop",
		Manifest:      manifest,
		Sources: map[string]string{
			"frontend-source": frontend,
			"backend-source":  backend,
		},
	}
	resolved, err := ResolveWorkspace(manifest, model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Components) != 4 {
		t.Fatalf("components = %#v", resolved.Components)
	}
	byID := map[string]ResolvedComponentSource{}
	for _, component := range resolved.Components {
		byID[component.Component] = component
	}
	if byID["frontend"].Root != frontend {
		t.Fatalf("frontend root = %q", byID["frontend"].Root)
	}
	if byID["api"].Root != filepath.Join(backend, "services", "api") {
		t.Fatalf("api root = %q", byID["api"].Root)
	}
	if byID["worker"].Root != filepath.Join(backend, "workers", "job") {
		t.Fatalf("worker root = %q", byID["worker"].Root)
	}
	if byID["external"].Image == "" || byID["external"].Root != "" {
		t.Fatalf("OCI component = %#v", byID["external"])
	}
}

func TestResolveWorkspaceMissingWorktreeFailsBeforeMutation(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "baseharbor.yaml")
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{
			{ID: "api-source", Type: SourceRepository, Repository: "https://git.example/api.git"},
		},
		Components: []ComponentSource{{Component: "api", Source: "api-source"}},
	}
	mapping := WorkspaceMapping{
		SchemaVersion: WorkspaceMappingVersion,
		Application:   "shop",
		Sources: map[string]string{"api-source": filepath.Join(root, "missing")},
	}
	if _, err := ResolveWorkspace(manifest, model, mapping); err == nil {
		t.Fatal("expected missing worktree error")
	}
}

func TestWorkspaceMappingStoredOutsidePortableProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	manifest := filepath.Join(root, "baseharbor.yaml")
	path, err := SaveWorkspaceMapping(manifest, WorkspaceMapping{
		Application: "shop",
		Sources:     map[string]string{"api-source": filepath.Join(root, "api")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) == root || filepath.Dir(filepath.Dir(path)) == root {
		t.Fatalf("workspace mapping leaked into portable project: %s", path)
	}
	loaded, _, err := LoadWorkspaceMapping(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Sources["api-source"] == "" {
		t.Fatalf("workspace mapping lost source path: %#v", loaded)
	}
}

func TestSourceModelRejectsEscapingSubPath(t *testing.T) {
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{{ID: "api-source", Type: SourceRepository, Repository: "https://git.example/api.git"}},
		Components: []ComponentSource{{Component: "api", Source: "api-source", SubPath: "../outside"}},
	}
	if err := model.Validate(); err == nil {
		t.Fatal("expected escaping subPath to fail")
	}
}
