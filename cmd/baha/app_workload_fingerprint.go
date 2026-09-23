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

const repositoryWorkloadBuildFingerprintFile = "workload-build-fingerprints.json"

type repositoryBuildConfig struct {
	Context    string         `json:"context"`
	Dockerfile string         `json:"dockerfile"`
	Args       map[string]any `json:"args"`
	Target     string         `json:"target"`
}

func repositoryWorkloadBuildFingerprints(
	ctx context.Context,
	compose bhruntime.Compose,
	workload application.WorkloadFiles,
	environment map[string]string,
	composeFiles []string,
	selected []string,
) (map[string]string, error) {
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return nil, fmt.Errorf("render repository Compose for build fingerprint: %w", err)
	}
	var model struct {
		Services map[string]struct {
			Build json.RawMessage `json:"build"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(rendered), &model); err != nil {
		return nil, fmt.Errorf("decode repository Compose for build fingerprint: %w", err)
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
		raw := definition.Build
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		build, err := decodeRepositoryBuildConfig(raw)
		if err != nil {
			return nil, fmt.Errorf("decode build configuration for service %s: %w", service, err)
		}
		if strings.TrimSpace(build.Context) == "" {
			continue
		}
		fingerprint, err := fingerprintRepositoryBuild(workload, service, build)
		if err != nil {
			return nil, err
		}
		result[service] = fingerprint
	}
	return result, nil
}

func decodeRepositoryBuildConfig(raw json.RawMessage) (repositoryBuildConfig, error) {
	var object repositoryBuildConfig
	if err := json.Unmarshal(raw, &object); err == nil && strings.TrimSpace(object.Context) != "" {
		return object, nil
	}
	var contextPath string
	if err := json.Unmarshal(raw, &contextPath); err == nil && strings.TrimSpace(contextPath) != "" {
		return repositoryBuildConfig{Context: contextPath}, nil
	}
	return repositoryBuildConfig{}, errors.New("unsupported or empty Compose build configuration")
}

func fingerprintRepositoryBuild(workload application.WorkloadFiles, service string, build repositoryBuildConfig) (string, error) {
	contextDir := strings.TrimSpace(build.Context)
	if !filepath.IsAbs(contextDir) {
		contextDir = filepath.Join(filepath.Dir(workload.Compose), contextDir)
	}
	contextDir, err := filepath.Abs(contextDir)
	if err != nil {
		return "", fmt.Errorf("resolve build context for service %s: %w", service, err)
	}
	info, err := os.Stat(contextDir)
	if err != nil {
		return "", fmt.Errorf("inspect build context for service %s: %w", service, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("build context for service %s is not a directory", service)
	}

	ignore, err := loadDockerIgnore(contextDir)
	if err != nil {
		return "", fmt.Errorf("read .dockerignore for service %s: %w", service, err)
	}
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "service=%s\n", service)
	configJSON, _ := json.Marshal(build)
	_, _ = h.Write(configJSON)
	_, _ = h.Write([]byte("\n"))

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
		if rel == ".baseharbor" || strings.HasPrefix(rel, ".baseharbor/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if ignore.ignored(rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk build context for service %s: %w", service, err)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		path := filepath.Join(contextDir, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("inspect build input %s for service %s: %w", rel, service, err)
		}
		_, _ = fmt.Fprintf(h, "%s\x00%o\x00", rel, info.Mode())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", fmt.Errorf("read build symlink %s for service %s: %w", rel, service, err)
			}
			_, _ = h.Write([]byte(target))
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read build input %s for service %s: %w", rel, service, err)
		}
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type dockerIgnoreRule struct {
	pattern string
	negate  bool
	dirOnly bool
}

type dockerIgnoreRules []dockerIgnoreRule

func loadDockerIgnore(contextDir string) (dockerIgnoreRules, error) {
	file, err := os.Open(filepath.Join(contextDir, ".dockerignore"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var rules dockerIgnoreRules
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := dockerIgnoreRule{}
		if strings.HasPrefix(line, "!") {
			rule.negate = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
		}
		rule.dirOnly = strings.HasSuffix(line, "/")
		line = strings.Trim(line, "/")
		if line == "" || line == "." {
			continue
		}
		rule.pattern = filepath.ToSlash(line)
		rules = append(rules, rule)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

func (rules dockerIgnoreRules) ignored(rel string, isDir bool) bool {
	ignored := false
	for _, rule := range rules {
		if rule.dirOnly && !isDir && !strings.HasPrefix(rel, rule.pattern+"/") {
			continue
		}
		if dockerIgnorePatternMatches(rule.pattern, rel) {
			ignored = !rule.negate
		}
	}
	return ignored
}

func dockerIgnorePatternMatches(pattern, rel string) bool {
	if pattern == rel || strings.HasPrefix(rel, pattern+"/") {
		return true
	}
	if !strings.Contains(pattern, "/") {
		for _, part := range strings.Split(rel, "/") {
			if ok, _ := filepath.Match(pattern, part); ok {
				return true
			}
		}
	}
	if ok, _ := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(rel)); ok {
		return true
	}
	return false
}

func loadRepositoryWorkloadBuildFingerprints(files application.RuntimeFiles) (map[string]string, error) {
	path := filepath.Join(files.Dir, repositoryWorkloadBuildFingerprintFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state map[string]string
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode workload build fingerprint state: %w", err)
	}
	if state == nil {
		state = map[string]string{}
	}
	return state, nil
}

func changedRepositoryBuildServices(current, previous map[string]string) []string {
	var changed []string
	for service, fingerprint := range current {
		if previous[service] != fingerprint {
			changed = append(changed, service)
		}
	}
	sort.Strings(changed)
	return changed
}

func persistRepositoryWorkloadBuildFingerprints(files application.RuntimeFiles, state map[string]string) error {
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
	path := filepath.Join(files.Dir, repositoryWorkloadBuildFingerprintFile)
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
