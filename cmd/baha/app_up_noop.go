package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

const repositoryAppliedFingerprintName = "applied-desired-state.sha256"

type repositoryUpDecision string

const (
	repositoryUpApply repositoryUpDecision = "apply"
	repositoryUpStart repositoryUpDecision = "start"
	repositoryUpNoop  repositoryUpDecision = "noop"
)

func decideRepositoryUp(state string, runtimeDefinitionOK, fingerprintMatch bool) repositoryUpDecision {
	switch state {
	case "stopped":
		if runtimeDefinitionOK && fingerprintMatch {
			return repositoryUpStart
		}
	case "running":
		if runtimeDefinitionOK && fingerprintMatch {
			return repositoryUpNoop
		}
	}
	return repositoryUpApply
}

func repositoryAppliedFingerprintPath(files application.RuntimeFiles) string {
	return filepath.Join(files.Dir, repositoryAppliedFingerprintName)
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

	paths, err := repositoryTrackedAndUntrackedFiles(ctx, repoRoot)
	if err != nil {
		return "", err
	}
	for _, rel := range paths {
		path := filepath.Join(repoRoot, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("inspect desired-state input %s: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read desired-state input %s: %w", rel, err)
		}
		write(filepath.ToSlash(rel), data)
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

func recordRepositoryAppliedFingerprint(ctx context.Context, resolved resolvedApplication, files application.RuntimeFiles) error {
	if !resolved.FromRepository {
		return nil
	}
	digest, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil {
		return err
	}
	path := repositoryAppliedFingerprintPath(files)
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
	status, err := collectApplicationStatus(ctx, resolved.Store, nil)
	if err != nil {
		return repositoryUpApply, files, nil
	}
	if status.State == "running" && !status.Ready {
		return repositoryUpApply, files, nil
	}
	return decideRepositoryUp(status.State, runtimeDefinitionOK, fingerprintMatch), files, nil
}
