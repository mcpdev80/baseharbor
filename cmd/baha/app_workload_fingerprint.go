package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const repositoryWorkloadBuildStateFile = "workload-build-state.json"

type repositoryWorkloadBuildConfig struct {
	Context    string         `json:"context"`
	Dockerfile string         `json:"dockerfile"`
	Args       map[string]any `json:"args"`
	Target     string         `json:"target"`
}

func resolveRepositoryWorkloadBuildFingerprints(
	ctx context.Context,
	compose bhruntime.Compose,
	workload application.WorkloadFiles,
	environment map[string]string,
	selected []string,
	composeFiles []string,
) (map[string]string, error) {
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return nil, fmt.Errorf("render repository Compose: %w", err)
	}
	var model struct {
		Services map[string]struct {
			Build json.RawMessage `json:"build"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(rendered), &model); err != nil {
		return nil, fmt.Errorf("decode rendered repository Compose: %w", err)
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, service := range selected {
		selectedSet[service] = struct{}{}
	}

	result := map[string]string{}
	for service, definition := range model.Services {
		if _, ok := selectedSet[service]; !ok {
			continue
		}
		if len(definition.Build) == 0 || string(definition.Build) == "null" {
			continue
		}
		build, err := decodeRepositoryWorkloadBuildConfig(definition.Build)
		if err != nil {
			return nil, fmt.Errorf("service %s build configuration: %w", service, err)
		}
		fingerprint, err := fingerprintRepositoryWorkloadBuild(workload, service, definition.Build, build)
		if err != nil {
			return nil, err
		}
		result[service] = fingerprint
	}
	return result, nil
}

func decodeRepositoryWorkloadBuildConfig(raw json.RawMessage) (repositoryWorkloadBuildConfig, error) {
	var contextPath string
	if err := json.Unmarshal(raw, &contextPath); err == nil && strings.TrimSpace(contextPath) != "" {
		return repositoryWorkloadBuildConfig{Context: contextPath}, nil
	}
	var build repositoryWorkloadBuildConfig
	if err := json.Unmarshal(raw, &build); err != nil {
		return repositoryWorkloadBuildConfig{}, err
	}
	if strings.TrimSpace(build.Context) == "" {
		build.Context = "."
	}
	return build, nil
}

func fingerprintRepositoryWorkloadBuild(workload application.WorkloadFiles, service string, rawBuild json.RawMessage, build repositoryWorkloadBuildConfig) (string, error) {
	contextDir := strings.TrimSpace(build.Context)
	if !filepath.IsAbs(contextDir) {
		contextDir = filepath.Join(workload.RepositoryRoot, contextDir)
	}
	contextDir, err := filepath.Abs(contextDir)
	if err != nil {
		return "", fmt.Errorf("resolve build context for %s: %w", service, err)
	}
	info, err := os.Stat(contextDir)
	if err != nil {
		return "", fmt.Errorf("inspect build context for %s: %w", service, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("build context for %s is not a directory", service)
	}

	dockerfile := strings.TrimSpace(build.Dockerfile)
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(contextDir, dockerfile)
	}
	dockerfile, err = filepath.Abs(dockerfile)
	if err != nil {
		return "", fmt.Errorf("resolve Dockerfile for %s: %w", service, err)
	}
	if _, err := os.Stat(dockerfile); err != nil {
		return "", fmt.Errorf("inspect Dockerfile for %s: %w", service, err)
	}

	rules, ignoreBytes, err := loadRepositoryDockerIgnore(contextDir)
	if err != nil {
		return "", fmt.Errorf("read .dockerignore for %s: %w", service, err)
	}

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "service=%s\n", service)
	_, _ = h.Write(rawBuild)
	_, _ = h.Write([]byte("\n"))
	if len(ignoreBytes) > 0 {
		_, _ = h.Write([]byte(".dockerignore\x00"))
		_, _ = h.Write(ignoreBytes)
	}
	dockerfileBytes, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", fmt.Errorf("read Dockerfile for %s: %w", service, err)
	}
	_, _ = h.Write([]byte("Dockerfile\x00"))
	_, _ = h.Write(dockerfileBytes)

	var paths []string
	err = filepath.WalkDir(contextDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(contextDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".baseharbor" || strings.HasPrefix(rel, ".baseharbor/") {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if repositoryDockerIgnoreMatches(rules, rel) {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk build context for %s: %w", service, err)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		path := filepath.Join(contextDir, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("inspect build input %s for %s: %w", rel, service, err)
		}
		_, _ = fmt.Fprintf(h, "%s\x00%o\x00", rel, info.Mode())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", fmt.Errorf("read build symlink %s for %s: %w", rel, service, err)
			}
			_, _ = h.Write([]byte(target))
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read build input %s for %s: %w", rel, service, err)
		}
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type repositoryDockerIgnoreRule struct {
	Pattern string
	Negate  bool
}

func loadRepositoryDockerIgnore(contextDir string) ([]repositoryDockerIgnoreRule, []byte, error) {
	path := filepath.Join(contextDir, ".dockerignore")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var rules []repositoryDockerIgnoreRule
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := repositoryDockerIgnoreRule{}
		if strings.HasPrefix(line, "!") {
			rule.Negate = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
		}
		line = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(line)), "./")
		line = strings.Trim(line, "/")
		if line == "" || line == "." {
			continue
		}
		rule.Pattern = line
		rules = append(rules, rule)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return rules, data, nil
}

func repositoryDockerIgnoreMatches(rules []repositoryDockerIgnoreRule, rel string) bool {
	ignored := false
	for _, rule := range rules {
		if repositoryDockerIgnorePatternMatches(rule.Pattern, rel) {
			ignored = !rule.Negate
		}
	}
	return ignored
}

func repositoryDockerIgnorePatternMatches(pattern, rel string) bool {
	pattern = filepath.ToSlash(pattern)
	rel = filepath.ToSlash(rel)
	if pattern == rel || strings.HasPrefix(rel, pattern+"/") {
		return true
	}
	if ok, _ := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(rel)); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		for _, part := range strings.Split(rel, "/") {
			if ok, _ := filepath.Match(pattern, part); ok {
				return true
			}
		}
	}
	return false
}

func loadRepositoryWorkloadBuildState(files application.RuntimeFiles) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(files.Dir, repositoryWorkloadBuildStateFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state map[string]string
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = map[string]string{}
	}
	return state, nil
}

func changedRepositoryWorkloadBuildServices(current, previous map[string]string) []string {
	var changed []string
	for service, fingerprint := range current {
		if previous[service] != fingerprint {
			changed = append(changed, service)
		}
	}
	sort.Strings(changed)
	return changed
}

func persistRepositoryWorkloadBuildState(files application.RuntimeFiles, state map[string]string) error {
	if len(state) == 0 {
		return nil
	}
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(files.Dir, repositoryWorkloadBuildStateFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
