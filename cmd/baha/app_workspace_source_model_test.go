package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func writeSourceModelAppManifest(t *testing.T, root string) string {
	t.Helper()
	manifest := "version: 1\napp:\n  name: shop\n  environment: dev\nworkload:\n  components:\n    - api\n"
	path := filepath.Join(root, "baseharbor.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runWorkspaceSourceModelCLI(t *testing.T, args ...string) (bytes.Buffer, bytes.Buffer, error) {
	return runWorkspaceSourceModelCLIContext(t, context.Background(), args...)
}

func runWorkspaceSourceModelCLIContext(t *testing.T, ctx context.Context, args ...string) (bytes.Buffer, bytes.Buffer, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := runWithIO(ctx, append([]string{"app", "workspace"}, args...), &out, &errOut)
	return out, errOut, err
}

func TestWorkspaceSourceModelWritesRequireUpdatePermission(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion, Application: "shop",
		Sources: []development.SourceDefinition{
			{ID: "backend", Type: development.SourceRepository, Repository: "https://git.example/backend.git"},
			{ID: "legacy", Type: development.SourceLocal, Path: filepath.Join(root, "legacy")},
		},
		Components: []development.ComponentSource{{Component: "api", Source: "backend"}},
	}
	if err := os.MkdirAll(filepath.Join(root, "legacy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := development.WriteSourceModel(manifest, model); err != nil {
		t.Fatal(err)
	}
	ctxFor := func(role string) context.Context {
		ctx := operatorauth.WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "operator", IssuerVerified: true})
		return tenancy.WithContext(ctx, &tenancy.Context{TenantID: "tenant-a", ExternalIdentityID: "operator", Roles: []string{role}})
	}
	viewer := ctxFor("viewer")
	var out, errOut bytes.Buffer
	if err := appWorkspaceComponentListCommand().Run(viewer, []string{"--manifest", manifest}, &out, &errOut); err != nil {
		t.Fatalf("viewer could not read component mappings: %v", err)
	}
	deniedMutations := []struct {
		name string
		run  func() error
	}{
		{"source add", func() error {
			return appWorkspaceSourceAddCommand().Run(viewer, []string{"--manifest", manifest, "--source", "extra=https://git.example/extra.git"}, &out, &errOut)
		}},
		{"source remove", func() error {
			return appWorkspaceSourceRemoveCommand().Run(viewer, []string{"backend", "--manifest", manifest}, &out, &errOut)
		}},
		{"component add", func() error {
			return appWorkspaceComponentAddCommand().Run(viewer, []string{"worker", "backend", "--manifest", manifest}, &out, &errOut)
		}},
		{"component remove", func() error {
			return appWorkspaceComponentRemoveCommand().Run(viewer, []string{"api", "--manifest", manifest}, &out, &errOut)
		}},
		{"repository path update", func() error {
			return appWorkspacePathCommand().Run(viewer, []string{"backend", t.TempDir(), "--manifest", manifest}, &out, &errOut)
		}},
		{"local source path update", func() error {
			return appWorkspacePathCommand().Run(viewer, []string{"legacy", filepath.Join(root, "new-legacy"), "--manifest", manifest}, &out, &errOut)
		}},
	}
	for _, mutation := range deniedMutations {
		t.Run(mutation.name, func(t *testing.T) {
			if err := mutation.run(); err == nil {
				t.Fatalf("viewer was allowed to perform %s", mutation.name)
			}
		})
	}
	after, _, err := development.LoadSourceModel(manifest)
	if err != nil || len(after.Sources) != 2 || after.Sources[0].ID != "backend" || after.Sources[1].ID != "legacy" || len(after.Components) != 1 {
		t.Fatalf("denied viewer mutation changed source model: %#v %v", after, err)
	}
	if _, _, err := development.LoadWorkspaceMapping(manifest, "shop"); err == nil {
		t.Fatal("denied viewer mutation wrote a workspace mapping")
	}
	if err := appWorkspaceSourceAddCommand().Run(ctxFor("editor"), []string{"--manifest", manifest, "--source", "extra=https://git.example/extra.git"}, &out, &errOut); err != nil {
		t.Fatalf("editor could not add a source: %v", err)
	}
	after, _, err = development.LoadSourceModel(manifest)
	if err != nil || len(after.Sources) != 3 {
		t.Fatalf("editor mutation was not persisted: %#v %v", after, err)
	}
	checkout := t.TempDir()
	editor := ctxFor("editor")
	if err := appWorkspacePathCommand().Run(editor, []string{"backend", checkout, "--manifest", manifest}, &out, &errOut); err != nil {
		t.Fatalf("editor could not update repository checkout mapping: %v", err)
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifest, "shop")
	if err != nil || mapping.Sources["backend"] != checkout {
		t.Fatalf("editor mapping mutation was not persisted: %#v %v", mapping, err)
	}
	updatedLocal := filepath.Join(root, "new-legacy")
	if err := os.MkdirAll(updatedLocal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := appWorkspacePathCommand().Run(editor, []string{"legacy", updatedLocal, "--manifest", manifest}, &out, &errOut); err != nil {
		t.Fatalf("editor could not update local source path: %v", err)
	}
	after, _, err = development.LoadSourceModel(manifest)
	if err != nil || after.Sources[1].Path != updatedLocal {
		t.Fatalf("editor local source mutation was not persisted: %#v %v", after, err)
	}
}

func TestWorkspaceSourceAddMissingIdentityValueReturnsUsageError(t *testing.T) {
	for _, option := range []string{"--source", "--local", "--oci"} {
		t.Run(option, func(t *testing.T) {
			_, _, err := runWorkspaceSourceModelCLI(t, "source", "add", option)
			if err == nil {
				t.Fatalf("source add %s without a value succeeded", option)
			}
			if !strings.Contains(err.Error(), "requires a value") {
				t.Fatalf("source add %s returned unexpected error: %v", option, err)
			}
		})
	}
}

func TestWorkspaceInitAcceptsLocalDirectorySource(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	dir := filepath.Join(root, "legacy-app")
	if err := os.MkdirAll(filepath.Join(dir, "services", "api"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--local", "legacy="+dir, "--component", "api=legacy@services/api")
	if err != nil {
		t.Fatalf("init --local failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	model, _, err := development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Sources) != 1 || model.Sources[0].Type != development.SourceLocal || model.Sources[0].Path != dir {
		t.Fatalf("sources = %#v", model.Sources)
	}
	resolved, err := development.ResolveWorkspace(manifest, model, development.WorkspaceMapping{
		SchemaVersion: development.WorkspaceMappingVersion, Application: "shop", Manifest: manifest, Sources: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Components) != 1 || resolved.Components[0].Root != filepath.Join(dir, "services", "api") {
		t.Fatalf("resolved = %#v", resolved.Components)
	}

	// A missing directory fails closed with an actionable message.
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", filepath.Join(root, "nope", "baseharbor.yaml"),
		"--local", "legacy=/does/not/exist", "--component", "api=legacy"); err == nil {
		t.Fatal("init accepted a missing local directory")
	}
}

func TestWorkspaceInitRejectsArtificialComponents(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	_, errOut, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest, "--source", "api-src=https://git.example/api.git")
	if err == nil {
		t.Fatal("init succeeded without any --component mapping")
	}
	if !strings.Contains(errOut.String()+err.Error(), "--component") {
		t.Fatalf("error does not point at --component: %v (%s)", err, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".baseharbor", "sources.yaml")); !os.IsNotExist(err) {
		t.Fatalf("rejected init still wrote a source model: %v", err)
	}
}

func TestWorkspaceSourceAddAndRemoveDetachSafely(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--source", "api-src=https://git.example/api.git", "--component", "api=api-src"); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runWorkspaceSourceModelCLI(t, "source", "add", "--manifest", manifest, "--local", "legacy="+legacy, "-o", "json")
	if err != nil {
		t.Fatalf("source add failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	var addResult sourceMutationResult
	if err := json.Unmarshal(out.Bytes(), &addResult); err != nil {
		t.Fatalf("machine result: %v (%s)", err, out.String())
	}
	if addResult.Source != "legacy" || addResult.Type != "local" || addResult.SourceCount != 2 {
		t.Fatalf("add result = %#v", addResult)
	}
	model, _, err := development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Components) != 1 {
		t.Fatalf("source add invented components: %#v", model.Components)
	}

	// Removing a still-referenced source lists its ownership and changes nothing.
	if _, errOut, err := runWorkspaceSourceModelCLI(t, "source", "remove", "api-src", "--manifest", manifest, "--keep-components"); err == nil {
		t.Fatal("removing a referenced source succeeded with --keep-components")
	} else if !strings.Contains(errOut.String()+err.Error(), "api") {
		t.Fatalf("referenced-source error lacks component names: %v (%s)", err, errOut.String())
	}

	out, errOut, err = runWorkspaceSourceModelCLI(t, "source", "remove", "api-src", "--manifest", manifest, "-o", "json")
	if err != nil {
		t.Fatalf("source remove failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	var removeResult sourceMutationResult
	if err := json.Unmarshal(out.Bytes(), &removeResult); err != nil {
		t.Fatal(err)
	}
	if !removeResult.Removed || strings.Join(removeResult.DetachedComponents, ",") != "api" {
		t.Fatalf("remove result = %#v", removeResult)
	}

	// The last remaining source cannot be removed.
	if _, _, err := runWorkspaceSourceModelCLI(t, "source", "remove", "legacy", "--manifest", manifest); err == nil {
		t.Fatal("removing the last source succeeded")
	}
	final, _, err := development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Sources) != 1 || final.Sources[0].ID != "legacy" {
		t.Fatalf("final sources = %#v", final.Sources)
	}
}

func TestWorkspaceComponentAddListRemove(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	mono := filepath.Join(root, "monorepo")
	for _, sub := range []string{"services/api", "workers/job"} {
		if err := os.MkdirAll(filepath.Join(mono, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--local", "mono="+mono, "--source", "extra=https://git.example/extra.git", "--component", "api=mono@services/api"); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runWorkspaceSourceModelCLI(t, "component", "add", "worker", "mono@workers/job", "--manifest", manifest, "-o", "json")
	if err != nil {
		t.Fatalf("component add failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	var addResult sourceMutationResult
	if err := json.Unmarshal(out.Bytes(), &addResult); err != nil {
		t.Fatal(err)
	}
	if addResult.Component != "worker" || addResult.ComponentCount != 2 {
		t.Fatalf("component add result = %#v", addResult)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "component", "add", "ghost", "nowhere", "--manifest", manifest); err == nil {
		t.Fatal("component referencing unknown source accepted")
	}

	out, errOut, err = runWorkspaceSourceModelCLI(t, "component", "list", "--manifest", manifest, "--source", "mono", "-o", "json")
	if err != nil {
		t.Fatalf("component list failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	var list struct {
		Components []struct {
			Component string `json:"component"`
			Source    string `json:"source"`
			Type      string `json:"type"`
			SubPath   string `json:"sub_path"`
		} `json:"components"`
	}
	if err := json.Unmarshal(out.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Components) != 2 || list.Components[0].Component != "api" || list.Components[1].Component != "worker" {
		t.Fatalf("sorted filtered list = %#v", list.Components)
	}
	if list.Components[0].Type != "local" || list.Components[0].SubPath != "services/api" {
		t.Fatalf("list view = %#v", list.Components[0])
	}

	out, errOut, err = runWorkspaceSourceModelCLI(t, "component", "remove", "worker", "--manifest", manifest)
	if err != nil {
		t.Fatalf("component remove failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "detached component worker") {
		t.Fatalf("remove output = %s", out.String())
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "component", "remove", "worker", "--manifest", manifest); err == nil {
		t.Fatal("removing an unmapped component accepted")
	}
}

func TestWorkspacePathSetsAndShowsSourcePaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	origin := filepath.Join(root, "api-origin.git")
	if output, err := runGitCommand("", "init", "-q", "--bare", origin); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, output)
	}
	checkout := filepath.Join(root, "api-checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "user.name", "BaseHarbor Test"},
		{"remote", "add", "origin", origin},
	} {
		if output, err := runGitCommand(checkout, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	legacy := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--source", "api-src="+origin, "--local", "legacy="+legacy, "--component", "api=api-src"); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runWorkspaceSourceModelCLI(t, "path", "api-src", checkout, "--manifest", manifest, "-o", "json")
	if err != nil {
		t.Fatalf("path set failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	var setResult sourceMutationResult
	if err := json.Unmarshal(out.Bytes(), &setResult); err != nil {
		t.Fatal(err)
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Sources["api-src"] != checkout {
		t.Fatalf("mapping = %#v", mapping.Sources)
	}
	out, errOut, err = runWorkspaceSourceModelCLI(t, "path", "api-src", "--manifest", manifest, "--show-current")
	if err != nil {
		t.Fatalf("path show failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), displayUserPath(checkout)) {
		t.Fatalf("path show output = %s", out.String())
	}

	// A non-existing directory is refused and nothing local is created.
	if _, _, err := runWorkspaceSourceModelCLI(t, "path", "api-src", filepath.Join(root, "not-there"), "--manifest", manifest); err == nil {
		t.Fatal("path accepted a missing directory")
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "path", "nope", checkout, "--manifest", manifest); err == nil {
		t.Fatal("path accepted an unknown source")
	}
	if _, err := os.Stat(filepath.Join(root, "not-there")); !os.IsNotExist(err) {
		t.Fatalf("path created a directory: %v", err)
	}

	// Retargeting a local directory source updates the portable model only.
	relocated := filepath.Join(root, "legacy-v2")
	if err := os.MkdirAll(relocated, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "path", "legacy", relocated, "--manifest", manifest); err != nil {
		t.Fatal(err)
	}
	model, _, err := development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range model.Sources {
		if source.ID == "legacy" && source.Path != relocated {
			t.Fatalf("local path not updated: %#v", source)
		}
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "path", "legacy", filepath.Join(root, "gone"), "--manifest", manifest); err == nil {
		t.Fatal("local source retarget to missing directory accepted")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("the local source directory was not preserved: %v", err)
	}
	if _, err := os.Stat(relocated); err != nil {
		t.Fatalf("the new local source directory was not preserved: %v", err)
	}
	// A refused retarget keeps the previous portable path.
	model, _, err = development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range model.Sources {
		if source.ID == "legacy" && source.Path != relocated {
			t.Fatalf("refused retarget changed the portable path: %#v", source)
		}
	}
	// OCI sources have no local path.
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--oci", "img=ghcr.io/example/x@sha256:deadbeef", "--component", "viewer=img"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "path", "img", "/tmp", "--manifest", manifest); err == nil {
		t.Fatal("OCI source path edit accepted")
	}
}

func TestWorkspacePathRejectsForeignCheckoutOwnership(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	shared := filepath.Join(root, "shared-checkout")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--source", "one=https://git.example/one.git", "--source", "two=https://git.example/two.git", "--component", "api=one"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "path", "one", shared, "--manifest", manifest); err != nil {
		t.Fatal(err)
	}
	if _, errOut, err := runWorkspaceSourceModelCLI(t, "path", "two", shared, "--manifest", manifest); err == nil {
		t.Fatal("one checkout was mapped to two sources")
	} else if !strings.Contains(errOut.String()+err.Error(), "source one") {
		t.Fatalf("ownership error lacks the owning source: %v (%s)", err, errOut.String())
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Sources["one"] != shared || mapping.Sources["two"] != "" {
		t.Fatalf("refused ownership change altered mappings: %#v", mapping.Sources)
	}
	// Retargeting the same source to the same path stays a no-op success.
	if _, _, err := runWorkspaceSourceModelCLI(t, "path", "one", shared, "--manifest", manifest); err != nil {
		t.Fatalf("same-source repeat failed: %v", err)
	}
}

func TestWorkspaceRootNamespaceMirrorsSourceModelCommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	dir := filepath.Join(root, "mono")
	if err := os.MkdirAll(filepath.Join(dir, "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"workspace", "init", "--manifest", manifest, "--local", "mono=" + dir, "--component", "api=mono@svc"}, &out, &errOut); err != nil {
		t.Fatalf("root-namespace init failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	out.Reset()
	if err := runWithIO(context.Background(), []string{"workspace", "component", "list", "--manifest", manifest}, &out, &errOut); err != nil {
		t.Fatalf("root-namespace component list failed: %v\n%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "api -> mono (local @svc)") {
		t.Fatalf("root-namespace list output = %s", out.String())
	}
	out.Reset()
	if err := runWithIO(context.Background(), []string{"workspace", "path", "mono", "--manifest", manifest}, &out, &errOut); err != nil {
		t.Fatalf("root-namespace path failed: %v\n%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), filepath.Base(dir)) {
		t.Fatalf("root-namespace path output = %s", out.String())
	}
	out.Reset()
	if err := runWithIO(context.Background(), []string{"workspace", "source", "add", "--manifest", manifest, "--oci", "img=ghcr.io/example/x@sha256:deadbeef"}, &out, &errOut); err != nil {
		t.Fatalf("root-namespace source add failed: %v\n%s", err, errOut.String())
	}
	if err := runWithIO(context.Background(), []string{"workspace", "component", "remove", "api", "--manifest", manifest}, &out, &errOut); err != nil {
		t.Fatalf("root-namespace component remove failed: %v\n%s", err, errOut.String())
	}
	if err := runWithIO(context.Background(), []string{"workspace", "source", "remove", "mono", "--manifest", manifest}, &out, &errOut); err != nil {
		t.Fatalf("root-namespace source remove failed: %v\n%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "removed source mono") {
		t.Fatalf("root-namespace remove output = %s", out.String())
	}
	// Refusing the last source stays actionable in both namespaces.
	if err := runWithIO(context.Background(), []string{"workspace", "source", "remove", "img", "--manifest", manifest}, &out, &errOut); err == nil {
		t.Fatal("root namespace removed the last source")
	}
}

func TestWorkspaceSourceModelMachineParity(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	// Deterministic repeat: the same state renders identical JSON twice.
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--source", "api-src=https://git.example/api.git", "--component", "api=api-src"); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	var first bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "workspace", "show", "--manifest", manifest, "-o", "json"}, &first, &second); err != nil {
		t.Fatal(err)
	}
	var again bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "workspace", "show", "--manifest", manifest, "-o", "json"}, &again, &second); err != nil {
		t.Fatal(err)
	}
	if first.String() != again.String() {
		t.Fatal("machine output is not deterministic")
	}
	var payload map[string]any
	if err := json.Unmarshal(first.Bytes(), &payload); err != nil {
		t.Fatalf("machine output is not valid JSON: %v", err)
	}
	// CLI human and machine views describe the same single component/source.
	model, _, err := development.LoadSourceModel(manifest)
	if err != nil || len(model.Components) != 1 || model.Components[0].Component != "api" {
		t.Fatalf("source model = %#v %v", model, err)
	}
}

func TestWorkspaceLocalSourceMappingOverridesPortablePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	portable := filepath.Join(root, "portable-dir")
	if err := os.MkdirAll(portable, 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(root, "local-dir")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--local", "mono="+portable, "--component", "api=mono"); err != nil {
		t.Fatal(err)
	}
	// map records a developer-local override for a local directory source
	// exactly like it does for repository checkouts; the portable path stays
	// untouched in .baseharbor/sources.yaml (card 0428-059).
	if _, errOut, err := runWorkspaceSourceModelCLI(t, "map", "mono", local, "--manifest", manifest); err != nil {
		t.Fatalf("map on local source failed: %v\n%s", err, errOut.String())
	}
	model, _, err := development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	resolvedAfterMap, err := development.ResolveWorkspace(manifest, model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolvedAfterMap.Components) != 1 || resolvedAfterMap.Components[0].Root != local {
		t.Fatalf("resolve ignored the map-only local override: %#v", resolvedAfterMap.Components)
	}
	if _, errOut, err := runWorkspaceSourceModelCLI(t, "path", "mono", local, "--manifest", manifest); err != nil {
		t.Fatalf("path override failed: %v\n%s", err, errOut.String())
	}
	mapping, _, err = development.LoadWorkspaceMapping(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Sources["mono"] != local {
		t.Fatalf("explicit override not stored: %#v", mapping.Sources)
	}
	model, _, err = development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := development.ResolveWorkspace(manifest, model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Components) != 1 || resolved.Components[0].Root != local {
		t.Fatalf("resolution should prefer the local override: %#v", resolved.Components)
	}
	// The Git inspection prefers the same explicit local override.
	mappingForGit, _, err := development.LoadWorkspaceMapping(manifest, "shop")
	if err != nil {
		t.Fatal(err)
	}
	status, err := development.InspectWorkspaceGit(context.Background(), model, mappingForGit, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range status.Repositories {
		if item.Source == "mono" && item.Path != local {
			t.Fatalf("Git inspection ignored the local override: %#v", item)
		}
	}
}

func TestWorkspaceSourceModelPreservesExistingMachineSemantics(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--source", "api-src=https://git.example/api.git", "--component", "api=api-src"); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"agent", "describe"}, &out, &errOut); err != nil {
		t.Fatalf("agent describe failed: %v\n%s", err, errOut.String())
	}
	for _, required := range []string{"baseharbor.workspace.init", "baseharbor.workspace.map", "baseharbor.workspace.show", "baseharbor.workspace.resolve", "baseharbor.workspace.status", "baseharbor.workspace.update"} {
		if !strings.Contains(out.String(), required) {
			t.Fatalf("agent describe lost workspace operation %s", required)
		}
	}
}

func TestWorkspaceGitStatusIncludesLocalGitSources(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	manifest := writeSourceModelAppManifest(t, root)
	origin := filepath.Join(root, "origin.git")
	if output, err := runGitCommand("", "init", "-q", "--bare", origin); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, output)
	}
	gitDir := filepath.Join(root, "git-dir")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "user.name", "BaseHarbor Test"},
		{"remote", "add", "origin", origin},
	} {
		if output, err := runGitCommand(gitDir, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "compose.yaml"), []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := runGitCommand(gitDir, "add", "."); err != nil {
		t.Fatalf("git add: %v\n%s", err, output)
	}
	if output, err := runGitCommand(gitDir, "commit", "-qm", "seed"); err != nil {
		t.Fatalf("git commit: %v\n%s", err, output)
	}
	if output, err := runGitCommand(gitDir, "push", "-q", "-u", "origin", "main"); err != nil {
		t.Fatalf("git push: %v\n%s", err, output)
	}
	plain := filepath.Join(root, "plain-dir")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runWorkspaceSourceModelCLI(t, "init", "--manifest", manifest,
		"--local", "repo-dir="+gitDir, "--local", "plain="+plain, "--component", "api=repo-dir"); err != nil {
		t.Fatal(err)
	}
	model, _, err := development.LoadSourceModel(manifest)
	if err != nil {
		t.Fatal(err)
	}
	status, err := development.InspectWorkspaceGit(context.Background(), model, development.WorkspaceMapping{
		SchemaVersion: development.WorkspaceMappingVersion, Application: "shop", Manifest: manifest, Sources: map[string]string{},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]development.WorkspaceGitRepositoryStatus{}
	for _, item := range status.Repositories {
		seen[item.Source] = item
	}
	if len(seen) != 1 {
		t.Fatalf("status repositories = %#v", status.Repositories)
	}
	if seen["repo-dir"].State != development.WorkspaceGitCurrent || seen["repo-dir"].CurrentRevision == "" {
		t.Fatalf("local git source = %#v", seen["repo-dir"])
	}
	// A plain directory is skipped like an OCI source: never reported as an
	// updatable Git repository and never changed by the update loop.
	if _, tracked := seen["plain"]; tracked {
		t.Fatalf("plain directory entered the Git inspection: %#v", seen["plain"])
	}
	if _, err := os.Stat(plain); err != nil {
		t.Fatalf("plain directory missing: %v", err)
	}
}

func runGitCommand(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	return output, err
}
