package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/development"
)

var appWorkspaceInput io.Reader = os.Stdin

func runAppWorkspaceWizard(ctx context.Context, out io.Writer) error {
	manifestPath, manifest, err := resolveWorkspaceManifest(".")
	if err != nil {
		return err
	}
	if _, _, err := development.LoadSourceModel(manifestPath); err == nil {
		return usageError("workspace source model already exists", "Run 'baha app workspace show' to inspect it, or use deterministic workspace commands to update mappings.")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	reader := bufio.NewReader(appWorkspaceInput)
	fmt.Fprintln(out, "Configure application workspace")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Application: %s\n", manifest.Name)
	fmt.Fprintf(out, "Canonical manifest: %s\n", manifestPath)
	fmt.Fprintln(out, "Local checkout paths stay in developer-local XDG workspace state.")

	candidates := detectedWorkspaceCandidates(filepath.Dir(manifestPath))
	if len(candidates) > 0 {
		fmt.Fprintln(out, "\nDetected local repositories/worktrees:")
		for i, candidate := range candidates {
			fmt.Fprintf(out, "  %d. %s\n", i+1, displayUserPath(candidate))
		}
	}

	model := development.SourceModel{SchemaVersion: development.SourceModelVersion, Application: manifest.Name}
	mapping := development.WorkspaceMapping{
		SchemaVersion: development.WorkspaceMappingVersion,
		Application:   manifest.Name,
		Manifest:      manifestPath,
		Sources:       map[string]string{},
	}

	for componentIndex := 0; ; componentIndex++ {
		defaultComponent := "app"
		if componentIndex > 0 {
			defaultComponent = fmt.Sprintf("component-%d", componentIndex+1)
		}
		component, err := promptLine(reader, out, "Component", defaultComponent)
		if err != nil {
			return err
		}
		component = slugifyAppName(component)
		if component == "" {
			return usageError("component name is required", "Use a stable logical component name such as frontend, api or worker.")
		}

		sourceID, subPath, err := guidedWorkspaceSource(ctx, reader, out, component, candidates, &model, &mapping)
		if err != nil {
			return err
		}
		model.Components = append(model.Components, development.ComponentSource{Component: component, Source: sourceID, SubPath: subPath})

		more, err := promptYesNo(reader, out, "Add another component?", false)
		if err != nil {
			return err
		}
		if !more {
			break
		}
	}

	if err := model.Validate(); err != nil {
		return err
	}
	resolved, err := development.ResolveWorkspace(manifestPath, model, mapping)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "\nWorkspace summary")
	fmt.Fprintf(out, "  Canonical manifest  %s\n", manifestPath)
	for _, component := range resolved.Components {
		if component.Type == development.SourceOCI {
			fmt.Fprintf(out, "  %-18s OCI %s\n", component.Component, component.Image)
		} else {
			fmt.Fprintf(out, "  %-18s %s -> %s\n", component.Component, component.Identity, displayUserPath(component.Root))
		}
	}
	confirm, err := promptYesNo(reader, out, "Write workspace configuration?", true)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Fprintln(out, "No changes were made.")
		return nil
	}

	sourcePath, err := development.WriteSourceModel(manifestPath, model)
	if err != nil {
		return err
	}
	workspacePath, err := development.SaveWorkspaceMapping(manifestPath, mapping)
	if err != nil {
		_ = os.Remove(sourcePath)
		return err
	}

	fmt.Fprintf(out, "Source model: %s\n", sourcePath)
	fmt.Fprintf(out, "Workspace state: %s\n", workspacePath)
	fmt.Fprintln(out, "Resolution: SATISFIED")
	fmt.Fprintln(out, "Next: baha app workspace resolve")
	return nil
}

func guidedWorkspaceSource(ctx context.Context, reader *bufio.Reader, out io.Writer, component string, candidates []string, model *development.SourceModel, mapping *development.WorkspaceMapping) (string, string, error) {
	if len(model.Sources) > 0 {
		fmt.Fprintln(out, "\nSource")
		for i, source := range model.Sources {
			label := source.Repository
			if source.Type == development.SourceOCI {
				label = source.Image
			}
			fmt.Fprintf(out, "  %d. Reuse %s (%s)\n", i+1, source.ID, label)
		}
		fmt.Fprintf(out, "  %d. New local repository/worktree\n", len(model.Sources)+1)
		fmt.Fprintf(out, "  %d. OCI image\n", len(model.Sources)+2)
		choice, err := promptNumber(reader, out, "Selection", 1, len(model.Sources)+2, len(model.Sources)+1)
		if err != nil {
			return "", "", err
		}
		if choice <= len(model.Sources) {
			source := model.Sources[choice-1]
			if source.Type == development.SourceOCI {
				return source.ID, "", nil
			}
			subPath, err := promptLine(reader, out, "Component path inside checkout", ".")
			if err != nil {
				return "", "", err
			}
			if strings.TrimSpace(subPath) == "." {
				subPath = ""
			}
			return source.ID, subPath, nil
		}
		if choice == len(model.Sources)+2 {
			return guidedWorkspaceOCI(reader, out, component, model)
		}
	} else {
		fmt.Fprintln(out, "\nSource")
		fmt.Fprintln(out, "  1. Local repository/worktree")
		fmt.Fprintln(out, "  2. OCI image")
		choice, err := promptNumber(reader, out, "Selection", 1, 2, 1)
		if err != nil {
			return "", "", err
		}
		if choice == 2 {
			return guidedWorkspaceOCI(reader, out, component, model)
		}
	}

	defaultPath := ""
	if len(candidates) > 0 {
		defaultPath = candidates[0]
	}
	checkout, err := promptDirectoryPathFrom(reader, out, "Local checkout", defaultPath, appWorkspaceInput)
	if err != nil {
		return "", "", err
	}
	checkout, err = expandUserPath(checkout)
	if err != nil {
		return "", "", err
	}
	checkout, err = filepath.Abs(checkout)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(checkout)
	if err != nil || !info.IsDir() {
		return "", "", usageError("local checkout is not an existing directory", "Choose an already-present repository/worktree. Automatic Git checkout is deferred to v0.4.20.")
	}

	sourceID, err := promptLine(reader, out, "Source id", component+"-source")
	if err != nil {
		return "", "", err
	}
	sourceID = slugifyAppName(sourceID)
	if sourceID == "" {
		return "", "", usageError("source id is required", "Use a stable source id such as frontend-source.")
	}
	if sourceModelHasID(*model, sourceID) {
		return "", "", usageError("source id already exists", "Choose a unique source id or reuse the existing source.")
	}

	origin := strings.TrimSpace(workspaceGitValue(ctx, checkout, "config", "--get", "remote.origin.url"))
	repository, err := promptLine(reader, out, "Repository identity", origin)
	if err != nil {
		return "", "", err
	}
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return "", "", usageError("repository identity is required", "Provide a stable repository URL/identity; the local filesystem path is intentionally not portable identity.")
	}
	ref := strings.TrimSpace(workspaceGitValue(ctx, checkout, "branch", "--show-current"))
	ref, err = promptLine(reader, out, "Repository ref", ref)
	if err != nil {
		return "", "", err
	}
	subPath, err := promptLine(reader, out, "Component path inside checkout", ".")
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(subPath) == "." {
		subPath = ""
	}

	model.Sources = append(model.Sources, development.SourceDefinition{ID: sourceID, Type: development.SourceRepository, Repository: repository, Ref: strings.TrimSpace(ref)})
	mapping.Sources[sourceID] = checkout
	return sourceID, strings.TrimSpace(subPath), nil
}

func guidedWorkspaceOCI(reader *bufio.Reader, out io.Writer, component string, model *development.SourceModel) (string, string, error) {
	sourceID, err := promptLine(reader, out, "Source id", component+"-source")
	if err != nil {
		return "", "", err
	}
	sourceID = slugifyAppName(sourceID)
	if sourceID == "" || sourceModelHasID(*model, sourceID) {
		return "", "", usageError("source id is missing or already exists", "Choose a unique stable source id.")
	}
	image, err := promptLine(reader, out, "OCI image", "")
	if err != nil {
		return "", "", err
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return "", "", usageError("OCI image is required", "Provide an OCI image reference, preferably an immutable digest for release workflows.")
	}
	model.Sources = append(model.Sources, development.SourceDefinition{ID: sourceID, Type: development.SourceOCI, Image: image})
	return sourceID, "", nil
}

func sourceModelHasID(model development.SourceModel, id string) bool {
	for _, source := range model.Sources {
		if source.ID == id {
			return true
		}
	}
	return false
}

func workspaceGitValue(ctx context.Context, root string, args ...string) string {
	value, err := gitOutput(ctx, root, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func detectedWorkspaceCandidates(manifestDir string) []string {
	seen := map[string]struct{}{}
	var candidates []string
	add := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if _, ok := seen[abs]; ok {
			return
		}
		if info, err := os.Stat(filepath.Join(abs, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			seen[abs] = struct{}{}
			candidates = append(candidates, abs)
		}
	}
	add(manifestDir)
	parent := filepath.Dir(manifestDir)
	entries, err := os.ReadDir(parent)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				add(filepath.Join(parent, entry.Name()))
			}
		}
	}
	sort.Strings(candidates)
	return candidates
}
