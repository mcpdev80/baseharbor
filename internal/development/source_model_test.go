package development

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestManifest(t *testing.T, root string) string {
	t.Helper()
	manifest := filepath.Join(root, "baseharbor.yaml")
	if err := os.WriteFile(manifest, []byte("version: 1\napp:\n  name: shop\n  environment: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestSourceModelAllowsOptionalUniqueComponents(t *testing.T) {
	// Zero components validate: an initialized model may gain components later.
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources:       []SourceDefinition{{ID: "api-source", Type: SourceRepository, Repository: "https://git.example/api.git"}},
	}
	if err := model.Validate(); err != nil {
		t.Fatalf("zero-component model rejected: %v", err)
	}
	// Multiple components may share one source and each name stays unique.
	model.Components = []ComponentSource{
		{Component: "api", Source: "api-source", SubPath: "services/api"},
		{Component: "worker", Source: "api-source", SubPath: "workers/job"},
	}
	if err := model.Validate(); err != nil {
		t.Fatalf("multi-component model rejected: %v", err)
	}
	model.Components = append(model.Components, ComponentSource{Component: "api", Source: "api-source"})
	if err := model.Validate(); err == nil || !strings.Contains(err.Error(), "mapped more than once") {
		t.Fatalf("duplicate component accepted: %v", err)
	}
}

func TestSourceModelLocalDirectorySource(t *testing.T) {
	dir := t.TempDir()
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources:       []SourceDefinition{{ID: "legacy", Type: SourceLocal, Path: dir}},
		Components:    []ComponentSource{{Component: "app", Source: "legacy"}},
	}
	if err := model.Validate(); err != nil {
		t.Fatalf("valid local source rejected: %v", err)
	}
	if err := UpsertSourceDefinition(&model, SourceDefinition{ID: "mixed", Type: SourceLocal, Path: dir, Image: "ghcr.io/example/nope@sha256:deadbeef"}); err == nil {
		t.Fatal("local source declaring image accepted")
	}
	relative := SourceDefinition{ID: "bad", Type: SourceLocal, Path: "relative/dir"}
	if err := UpsertSourceDefinition(&model, relative); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative local path accepted: %v", err)
	}
	missing := SourceDefinition{ID: "gone", Type: SourceLocal, Path: filepath.Join(dir, "not-there")}
	if err := UpsertSourceDefinition(&model, missing); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing local directory accepted: %v", err)
	}
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	asFile := SourceDefinition{ID: "fileish", Type: SourceLocal, Path: filePath}
	if err := UpsertSourceDefinition(&model, asFile); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("local source pointing at a file accepted: %v", err)
	}
}

func TestResolveWorkspaceLocalDirectoryWithSubPaths(t *testing.T) {
	root := t.TempDir()
	manifest := writeTestManifest(t, root)
	sourceDir := filepath.Join(root, "monorepo")
	for _, sub := range []string{"services/api", "workers/job"} {
		if err := os.MkdirAll(filepath.Join(sourceDir, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{
			{ID: "mono", Type: SourceLocal, Path: sourceDir},
			{ID: "external", Type: SourceOCI, Image: "ghcr.io/example/external@sha256:deadbeef"},
		},
		Components: []ComponentSource{
			{Component: "api", Source: "mono", SubPath: "services/api"},
			{Component: "worker", Source: "mono", SubPath: "workers/job"},
			{Component: "root-app", Source: "mono"},
			{Component: "external", Source: "external"},
		},
	}
	mapping := WorkspaceMapping{SchemaVersion: WorkspaceMappingVersion, Application: "shop", Manifest: manifest, Sources: map[string]string{}}
	resolved, err := ResolveWorkspace(manifest, model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ResolvedComponentSource{}
	for _, component := range resolved.Components {
		byID[component.Component] = component
	}
	if byID["api"].Root != filepath.Join(sourceDir, "services", "api") || byID["api"].Identity != filepath.Clean(sourceDir) {
		t.Fatalf("api = %#v", byID["api"])
	}
	if byID["worker"].Root != filepath.Join(sourceDir, "workers", "job") {
		t.Fatalf("worker = %#v", byID["worker"])
	}
	if byID["root-app"].Root != filepath.Clean(sourceDir) {
		t.Fatalf("root-app = %#v", byID["root-app"])
	}
	if byID["api"].Type != SourceLocal {
		t.Fatalf("api type = %q", byID["api"].Type)
	}
	if byID["external"].Root != "" || byID["external"].Image == "" {
		t.Fatalf("OCI component = %#v", byID["external"])
	}
	// A missing subpath inside the local directory fails before mutation.
	model.Components = []ComponentSource{{Component: "ghost", Source: "mono", SubPath: "services/ghost"}}
	if _, err := ResolveWorkspace(manifest, model, mapping); err == nil {
		t.Fatal("missing local subpath accepted")
	}
	// A deleted local directory surfaces as a missing-source failure.
	gone := filepath.Join(root, "deleted")
	model.Sources = []SourceDefinition{{ID: "mono", Type: SourceLocal, Path: gone}}
	model.Components = []ComponentSource{{Component: "api", Source: "mono"}}
	_, err = ResolveWorkspace(manifest, model, mapping)
	if err == nil {
		t.Fatal("missing local source directory accepted")
	}
	var sourceErr *WorkspaceSourceError
	if errors.As(err, &sourceErr) {
		t.Fatalf("missing local directory should be a validation failure, not a mapping error: %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unexpected local source error: %v", err)
	}
}

func TestResolveWorkspaceRejectsSubpathSymlinkEscapes(t *testing.T) {
	for _, sourceType := range []SourceKind{SourceLocal, SourceRepository} {
		t.Run(string(sourceType), func(t *testing.T) {
			base := t.TempDir()
			manifest := writeTestManifest(t, base)
			root := filepath.Join(base, "source")
			if err := os.MkdirAll(filepath.Join(root, "services"), 0o755); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(base, "outside")
			if err := os.MkdirAll(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "services", "api")); err != nil {
				t.Skipf("symlink not supported: %v", err)
			}
			source := SourceDefinition{ID: "backend", Type: sourceType}
			if sourceType == SourceLocal {
				source.Path = root
			} else {
				source.Repository = "https://git.example/backend.git"
			}
			model := SourceModel{
				SchemaVersion: SourceModelVersion,
				Application:   "shop",
				Sources:       []SourceDefinition{source},
				Components:    []ComponentSource{{Component: "api", Source: source.ID, SubPath: "services/api"}},
			}
			mapping := WorkspaceMapping{
				SchemaVersion: WorkspaceMappingVersion,
				Application:   "shop",
				Manifest:      manifest,
				Sources:       map[string]string{source.ID: root},
			}
			if _, err := ResolveWorkspace(manifest, model, mapping); err == nil || !strings.Contains(err.Error(), "symbolic link") {
				t.Fatalf("subpath symlink escape accepted: %v", err)
			}
		})
	}
}

func TestRemoveSourceRejectsLastSourceAndReportsReferences(t *testing.T) {
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{
			{ID: "keep", Type: SourceRepository, Repository: "https://git.example/keep.git"},
			{ID: "drop", Type: SourceRepository, Repository: "https://git.example/drop.git"},
		},
		Components: []ComponentSource{
			{Component: "api", Source: "keep"},
			{Component: "job", Source: "drop"},
			{Component: "other-job", Source: "drop"},
		},
	}
	if names := SourceComponentNames(model, "drop"); strings.Join(names, ",") != "job,other-job" {
		t.Fatalf("SourceComponentNames = %v", names)
	}
	if _, err := RemoveSourceDefinition(&model, "drop", true); err == nil || !strings.Contains(err.Error(), "referenced by component") {
		t.Fatalf("preserve-mode removal accepted: %v", err)
	}
	detached, err := RemoveSourceDefinition(&model, "drop", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(detached, ",") != "job,other-job" {
		t.Fatalf("detached = %v", detached)
	}
	if len(model.Components) != 1 || model.Components[0].Component != "api" {
		t.Fatalf("components after detach = %#v", model.Components)
	}
	if err := model.Validate(); err != nil {
		t.Fatalf("model after detach invalid: %v", err)
	}
	if _, err := RemoveSourceDefinition(&model, "keep", false); !errors.Is(err, ErrWorkspaceLastSource) {
		t.Fatalf("removing the last source accepted: %v", err)
	}
	if _, err := RemoveSourceDefinition(&model, "unknown", false); err == nil || !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("unknown source removal accepted: %v", err)
	}
}

func TestSourceAndComponentUpsertReplaceWithoutDuplicates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	manifest := writeTestManifest(t, root)
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources:       []SourceDefinition{{ID: "api-source", Type: SourceRepository, Repository: "https://git.example/api.git"}},
		Components:    []ComponentSource{{Component: "api", Source: "api-source"}},
	}
	if err := UpsertSourceDefinition(&model, SourceDefinition{ID: "api-source", Type: SourceRepository, Repository: "https://git.example/api-v2.git", Ref: "main"}); err != nil {
		t.Fatal(err)
	}
	if len(model.Sources) != 1 || model.Sources[0].Repository != "https://git.example/api-v2.git" {
		t.Fatalf("sources after upsert = %#v", model.Sources)
	}
	if err := UpsertSourceDefinition(&model, SourceDefinition{ID: "api-source", Type: SourceOCI, Image: "ghcr.io/example/api@sha256:deadbeef"}); err == nil {
		t.Fatal("type switch silently replaced a referenced source")
	}
	if err := UpsertSourceDefinition(&model, SourceDefinition{ID: "new-source", Type: SourceRepository, Repository: "https://git.example/new.git"}); err != nil {
		t.Fatalf("adding an unreferenced source failed: %v", err)
	}
	if len(model.Sources) != 2 {
		t.Fatalf("sources after adding = %#v", model.Sources)
	}
	if err := UpsertComponentMapping(&model, ComponentSource{Component: "job", Source: "ghost"}); err == nil {
		t.Fatal("component referencing an undeclared source accepted")
	}
	if err := UpsertComponentMapping(&model, ComponentSource{Component: "job", Source: "new-source"}); err != nil {
		t.Fatalf("component for declared source rejected: %v", err)
	}
	if err := UpsertSourceDefinition(&model, SourceDefinition{ID: "api-source", Type: SourceLocal, Path: other}); err == nil {
		t.Fatal("referenced repository source switched to local without detaching components")
	}
	if err := UpsertComponentMapping(&model, ComponentSource{Component: "api", Source: "api-source", SubPath: "services/api"}); err != nil {
		t.Fatal(err)
	}
	apiMappings := 0
	for _, component := range model.Components {
		if component.Component == "api" {
			apiMappings++
			if component.SubPath != "services/api" {
				t.Fatalf("api upsert = %#v", component)
			}
		}
	}
	if apiMappings != 1 {
		t.Fatalf("component upsert duplicated the unique mapping: %#v", model.Components)
	}
	if _, err := WriteSourceModel(manifest, model); err != nil {
		t.Fatal(err)
	}
	if err := RemoveComponentMapping(&model, "api"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveComponentMapping(&model, "api"); err == nil {
		t.Fatal("removing an unmapped component accepted")
	}
	if _, err := WriteSourceModel(manifest, model); err != nil {
		t.Fatalf("zero-component model not persistable: %v", err)
	}
	reloaded, _, err := LoadSourceModel(manifest)
	if err != nil {
		t.Fatalf("reloading a component-less model failed: %v", err)
	}
	if len(reloaded.Components) != 1 || reloaded.Components[0].Component != "job" || len(reloaded.Sources) != 2 {
		t.Fatalf("reloaded = %#v", reloaded)
	}
	if err := UpsertSourceDefinition(&reloaded, SourceDefinition{ID: "new-source", Type: SourceOCI, Image: "ghcr.io/example/api@sha256:deadbeef"}); err == nil {
		t.Fatal("type switch accepted while component 'job' still references 'new-source'")
	}
	if err := RemoveComponentMapping(&reloaded, "job"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertSourceDefinition(&reloaded, SourceDefinition{ID: "new-source", Type: SourceOCI, Image: "ghcr.io/example/api@sha256:deadbeef"}); err != nil {
		t.Fatalf("type switch allowed once no component references the source: %v", err)
	}
	if len(reloaded.Sources) != 2 || reloaded.Sources[1].Type != SourceOCI {
		t.Fatalf("sources after detached type switch = %#v", reloaded.Sources)
	}
}

func TestSaveRemovableSourceDetachesMappingOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	manifest := writeTestManifest(t, root)
	checkout := filepath.Join(root, "drop-checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "keepme.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{
			{ID: "keep", Type: SourceRepository, Repository: "https://git.example/keep.git"},
			{ID: "drop", Type: SourceRepository, Repository: "https://git.example/drop.git"},
		},
		Components: []ComponentSource{{Component: "api", Source: "keep"}, {Component: "job", Source: "drop"}},
	}
	if _, err := WriteSourceModel(manifest, model); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveWorkspaceMapping(manifest, WorkspaceMapping{
		SchemaVersion: WorkspaceMappingVersion,
		Application:   "shop",
		Manifest:      manifest,
		Sources:       map[string]string{"keep": filepath.Join(root, "keep-checkout"), "drop": checkout},
	}); err != nil {
		t.Fatal(err)
	}
	sourcePath, mappingPath, detached, removed, err := SaveRemovableSource(manifest, "shop", model, "drop")
	if err != nil {
		t.Fatal(err)
	}
	if sourcePath == "" || mappingPath == "" || !removed || strings.Join(detached, ",") != "job" {
		t.Fatalf("detach result = %q %q %v %v", sourcePath, mappingPath, detached, removed)
	}
	if _, err := os.Stat(filepath.Join(checkout, "keepme.txt")); err != nil {
		t.Fatalf("local checkout contents were deleted: %v", err)
	}
	reloaded, _, err := LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Sources) != 1 || reloaded.Sources[0].ID != "keep" || len(reloaded.Components) != 1 {
		t.Fatalf("reloaded model = %#v", reloaded)
	}
	mapping, _, err := LoadWorkspaceMapping(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := mapping.Sources["drop"]; exists {
		t.Fatalf("detached source kept its local mapping: %#v", mapping.Sources)
	}
	if mapping.Sources["keep"] == "" {
		t.Fatalf("detach removed a foreign mapping: %#v", mapping.Sources)
	}
	// Removing the last source now fails closed and changes nothing.
	if _, _, _, _, err := SaveRemovableSource(manifest, "shop", reloaded, "keep"); !errors.Is(err, ErrWorkspaceLastSource) {
		t.Fatalf("last source removal accepted: %v", err)
	}
	final, _, err := LoadSourceModel(manifest)
	if err != nil || len(final.Sources) != 1 {
		t.Fatalf("failed detach mutated persisted state: %#v %v", final, err)
	}
}

func TestSaveRemovableSourceDoesNotChangeModelWhenMappingCannotBeLoaded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	manifest := writeTestManifest(t, root)
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{
			{ID: "keep", Type: SourceRepository, Repository: "https://git.example/keep.git"},
			{ID: "drop", Type: SourceRepository, Repository: "https://git.example/drop.git"},
		},
		Components: []ComponentSource{{Component: "api", Source: "drop"}},
	}
	if _, err := WriteSourceModel(manifest, model); err != nil {
		t.Fatal(err)
	}
	mappingPath, err := WorkspaceMappingPath(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(mappingPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mappingPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := SaveRemovableSource(manifest, "shop", model, "drop"); err == nil {
		t.Fatal("removal succeeded with an invalid workspace mapping")
	}
	stored, _, err := LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Sources) != 2 || len(stored.Components) != 1 || stored.Components[0].Source != "drop" {
		t.Fatalf("failed removal partially changed the source model: %#v", stored)
	}
}

func TestSaveRemovableSourceRollsBackWhenMappingSaveFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	manifest := writeTestManifest(t, root)
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "shop",
		Sources: []SourceDefinition{
			{ID: "keep", Type: SourceRepository, Repository: "https://git.example/keep.git"},
			{ID: "drop", Type: SourceRepository, Repository: "https://git.example/drop.git"},
		},
		Components: []ComponentSource{{Component: "api", Source: "keep"}, {Component: "job", Source: "drop"}},
	}
	sourcePath, err := WriteSourceModel(manifest, model)
	if err != nil {
		t.Fatal(err)
	}
	mappingPath, err := SaveWorkspaceMapping(manifest, WorkspaceMapping{
		SchemaVersion: WorkspaceMappingVersion,
		Application:   "shop",
		Sources:       map[string]string{"keep": filepath.Join(root, "keep"), "drop": filepath.Join(root, "drop")},
	})
	if err != nil {
		t.Fatal(err)
	}
	originalModel, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	originalMapping, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	injectedFailure := errors.New("injected mapping persistence failure")
	_, _, _, _, err = saveRemovableSource(manifest, "shop", model, "drop", func(manifestPath string, mapping WorkspaceMapping) (string, error) {
		// Simulate a saver that persisted the new mapping and then returned an
		// error, so the rollback path must restore both files.
		path, saveErr := SaveWorkspaceMapping(manifestPath, mapping)
		if saveErr != nil {
			return path, saveErr
		}
		return path, injectedFailure
	})
	if !errors.Is(err, injectedFailure) {
		t.Fatalf("removal error = %v, want injected failure", err)
	}
	modelAfter, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	mappingAfter, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(modelAfter, originalModel) {
		t.Fatal("source model was not restored after mapping save failure")
	}
	if !bytes.Equal(mappingAfter, originalMapping) {
		t.Fatal("workspace mapping was not restored after mapping save failure")
	}
}
