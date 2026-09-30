package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

const (
	repositoryAppliedFingerprintName        = "applied-desired-state.sha256"
	repositoryAppliedControlFingerprintName = "applied-control-state.sha256"
)

type repositoryUpDecision string

const (
	repositoryUpApply         repositoryUpDecision = "apply"
	repositoryUpWorkloadApply repositoryUpDecision = "workload-apply"
	repositoryUpStart         repositoryUpDecision = "start"
	repositoryUpNoop          repositoryUpDecision = "noop"
)

func decideRepositoryUp(state string, runtimeDefinitionOK, fingerprintMatch, controlFingerprintMatch bool) repositoryUpDecision {
	switch state {
	case "stopped":
		if runtimeDefinitionOK && fingerprintMatch {
			return repositoryUpStart
		}
	case "running":
		if runtimeDefinitionOK && fingerprintMatch {
			return repositoryUpNoop
		}
		if runtimeDefinitionOK && controlFingerprintMatch {
			return repositoryUpWorkloadApply
		}
	}
	return repositoryUpApply
}

func repositoryAppliedFingerprintPath(files application.RuntimeFiles) string {
	return filepath.Join(files.Dir, repositoryAppliedFingerprintName)
}

func repositoryAppliedControlFingerprintPath(files application.RuntimeFiles) string {
	return filepath.Join(files.Dir, repositoryAppliedControlFingerprintName)
}

func repositoryControlStateFingerprint(resolved resolvedApplication) (string, error) {
	if !resolved.FromRepository {
		return "", nil
	}
	h := sha256.New()
	write := func(name string, data []byte) {
		_, _ = io.WriteString(h, name)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}

	manifestPath := strings.TrimSpace(resolved.ManifestPath)
	if manifestPath == "" {
		return "", errors.New("repository application manifest path is unavailable")
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("read application contract for control fingerprint: %w", err)
	}
	write("baseharbor.yaml", manifest)
	if _, sourceModelPath, err := development.LoadSourceModel(resolved.ManifestPath); err == nil {
		data, readErr := os.ReadFile(sourceModelPath)
		if readErr != nil {
			return "", fmt.Errorf("read source identity model for control fingerprint: %w", readErr)
		}
		write(".baseharbor/sources.yaml", data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("load source identity model for control fingerprint: %w", err)
	}

	if data, err := os.ReadFile(repositoryInitEnvPathFromStateRoot(resolved.stateRoot())); err == nil {
		write(".baseharbor/init.env", data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read repository deployment control state: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func repositoryDesiredStateFingerprint(ctx context.Context, resolved resolvedApplication) (string, error) {
	if !resolved.FromRepository {
		return "", nil
	}
	repoRoot := resolved.repositoryRoot()
	h := sha256.New()
	write := func(name string, data []byte) {
		_, _ = io.WriteString(h, name)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}

	if err := writeRepositoryTreeFingerprint(ctx, write, "canonical", repoRoot); err != nil {
		return "", err
	}
	if err := fingerprintResolvedWorkspace(ctx, resolved, write); err != nil {
		return "", fmt.Errorf("fingerprint multi-repository workspace: %w", err)
	}

	if data, err := os.ReadFile(repositoryInitEnvPathFromStateRoot(resolved.stateRoot())); err == nil {
		write(".baseharbor/init.env", data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read repository deployment state: %w", err)
	}
	if data, err := os.ReadFile(filepath.Join(repoRoot, ".env")); err == nil {
		write(".env", data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read repository compose environment: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeRepositoryTreeFingerprint(ctx context.Context, write func(string, []byte), logicalPrefix, root string) error {
	paths, err := repositoryTrackedAndUntrackedFiles(ctx, root)
	if err != nil {
		return err
	}
	for _, rel := range paths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("inspect desired-state input %s: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read desired-state input %s: %w", rel, err)
		}
		label := filepath.ToSlash(rel)
		if strings.TrimSpace(logicalPrefix) != "" {
			label = strings.TrimSuffix(logicalPrefix, "/") + "/" + label
		}
		write(label, data)
	}
	return nil
}

func fingerprintResolvedWorkspace(ctx context.Context, resolved resolvedApplication, write func(string, []byte)) error {
	model, sourceModelPath, err := development.LoadSourceModel(resolved.ManifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if model.Application != resolved.Manifest.Name {
		return fmt.Errorf("source model application %q does not match application manifest %q", model.Application, resolved.Manifest.Name)
	}
	data, err := os.ReadFile(sourceModelPath)
	if err != nil {
		return fmt.Errorf("read canonical source model: %w", err)
	}
	write("portable/.baseharbor/sources.yaml", data)

	mapping, _, err := development.LoadWorkspaceMapping(resolved.ManifestPath, resolved.Manifest.Name)
	if err != nil {
		return err
	}
	workspace, err := development.ResolveWorkspace(resolved.ManifestPath, model, mapping)
	if err != nil {
		return err
	}
	for _, component := range workspace.Components {
		identity, err := json.Marshal(struct {
			Component string                 `json:"component"`
			Source    string                 `json:"source"`
			Type      development.SourceKind `json:"type"`
			Identity  string                 `json:"identity"`
			Ref       string                 `json:"ref,omitempty"`
			SubPath   string                 `json:"sub_path,omitempty"`
			Image     string                 `json:"image,omitempty"`
		}{
			Component: component.Component,
			Source:    component.Source,
			Type:      component.Type,
			Identity:  component.Identity,
			Ref:       component.Ref,
			SubPath:   component.SubPath,
			Image:     component.Image,
		})
		if err != nil {
			return err
		}
		write("component/"+component.Component+"/source-identity.json", identity)
		if component.Type == development.SourceRepository {
			if err := writeRepositoryTreeFingerprint(ctx, write, "component/"+component.Component+"/source", component.Root); err != nil {
				return fmt.Errorf("fingerprint component %s source: %w", component.Component, err)
			}
		}
	}
	return nil
}

func repositoryTrackedAndUntrackedFiles(ctx context.Context, repoRoot string) ([]string, error) {
	out, err := gitOutput(ctx, repoRoot, "ls-files", "--cached", "--others", "--exclude-standard")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		paths := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, ".baseharbor/") {
				continue
			}
			paths = append(paths, filepath.ToSlash(line))
		}
		sort.Strings(paths)
		return paths, nil
	}

	var paths []string
	err = filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".baseharbor", "node_modules", ".next", "dist", "build", "target", ".venv", "venv", "__pycache__":
				if rel != "." {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if entry.Type().IsRegular() {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("enumerate repository desired-state inputs: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func repositoryFingerprintMatches(ctx context.Context, resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	expected, err := os.ReadFile(repositoryAppliedFingerprintPath(files))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	current, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(expected)) == current, nil
}

func repositoryControlFingerprintMatches(resolved resolvedApplication, files application.RuntimeFiles) (bool, error) {
	expected, err := os.ReadFile(repositoryAppliedControlFingerprintPath(files))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	current, err := repositoryControlStateFingerprint(resolved)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(expected)) == current, nil
}

func writeRepositoryFingerprint(path, digest string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(digest+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func recordRepositoryAppliedFingerprint(ctx context.Context, resolved resolvedApplication, files application.RuntimeFiles) error {
	if !resolved.FromRepository {
		return nil
	}
	digest, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil {
		return err
	}
	controlDigest, err := repositoryControlStateFingerprint(resolved)
	if err != nil {
		return err
	}
	if err := writeRepositoryFingerprint(repositoryAppliedFingerprintPath(files), digest); err != nil {
		return err
	}
	if err := writeRepositoryFingerprint(repositoryAppliedControlFingerprintPath(files), controlDigest); err != nil {
		return err
	}
	return nil
}

func repositoryUpCurrentDecision(ctx context.Context, resolved resolvedApplication) (repositoryUpDecision, application.RuntimeFiles, error) {
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if errors.Is(err, application.ErrRuntimeNotApplied) {
		return repositoryUpApply, application.RuntimeFiles{}, nil
	}
	if err != nil {
		return repositoryUpApply, application.RuntimeFiles{}, err
	}

	runtimeDefinitionOK := application.CheckManagedRuntimeDefinition(files, resolved.Manifest) == nil
	if application.RequiresRuntimeBroker(resolved.Manifest) && runtimebroker.IsMutableDevelopmentImage(os.Getenv("BASEHARBOR_RUNTIME_IMAGE")) {
		// A moving development tag cannot be proven current without consulting
		// the registry. Force the normal convergence path; it performs the pull,
		// recreation and compatibility verification before reporting READY.
		return repositoryUpApply, files, nil
	}
	fingerprintMatch, err := repositoryFingerprintMatches(ctx, resolved, files)
	if err != nil {
		return repositoryUpApply, files, err
	}
	controlFingerprintMatch, err := repositoryControlFingerprintMatches(resolved, files)
	if err != nil {
		return repositoryUpApply, files, err
	}
	status, err := collectApplicationStatus(ctx, resolved.Store, nil)
	if err != nil {
		return repositoryUpApply, files, nil
	}
	if status.State == "running" && !status.Ready {
		return repositoryUpApply, files, nil
	}
	return decideRepositoryUp(status.State, runtimeDefinitionOK, fingerprintMatch, controlFingerprintMatch), files, nil
}
