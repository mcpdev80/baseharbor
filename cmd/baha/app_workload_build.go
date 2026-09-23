package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const repositoryWorkloadBuildStateName = "workload-build-fingerprints.json"

type repositoryWorkloadBuildState struct {
	Version  int               `json:"version"`
	Services map[string]string `json:"services"`
}

type renderedWorkloadBuildConfig struct {
	Services map[string]struct {
		Build json.RawMessage `json:"build"`
	} `json:"services"`
}

type repositoryBuildDefinition struct {
	Context    string
	Dockerfile string
	Canonical  []byte
}

type dockerIgnoreRule struct {
	re       *regexp.Regexp
	negate   bool
	dirOnly  bool
	basename bool
}

func repositoryWorkloadBuildStatePath(files application.RuntimeFiles) string {
	return filepath.Join(files.Dir, repositoryWorkloadBuildStateName)
}

func resolveRepositoryWorkloadBuildFingerprints(
	ctx context.Context,
	compose bhruntime.Compose,
	workload application.WorkloadFiles,
	environment map[string]string,
	services []string,
	composeFiles []string,
) (map[string]string, error) {
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return nil, fmt.Errorf("render workload for build fingerprint: %w", err)
	}
	var config renderedWorkloadBuildConfig
	if err := json.Unmarshal([]byte(rendered), &config); err != nil {
		return nil, fmt.Errorf("decode workload build configuration: %w", err)
	}
	selected := make(map[string]struct{}, len(services))
	for _, service := range services {
		selected[service] = struct{}{}
	}
	result := map[string]string{}
	for service, definition := range config.Services {
		if _, ok := selected[service]; !ok {
			continue
		}
		build, found, err := parseRepositoryBuildDefinition(definition.Build)
		if err != nil {
			return nil, fmt.Errorf("resolve build inputs for service %s: %w", service, err)
		}
		if !found {
			continue
		}
		contextDir := build.Context
		if !filepath.IsAbs(contextDir) {
			contextDir = filepath.Join(workload.RepositoryRoot, contextDir)
		}
		contextDir, err = filepath.Abs(contextDir)
		if err != nil {
			return nil, fmt.Errorf("resolve build context for service %s: %w", service, err)
		}
		digest, err := fingerprintRepositoryBuildContext(contextDir, build.Dockerfile, build.Canonical)
		if err != nil {
			return nil, fmt.Errorf("fingerprint build context for service %s: %w", service, err)
		}
		result[service] = digest
	}
	return result, nil
}

func parseRepositoryBuildDefinition(raw json.RawMessage) (repositoryBuildDefinition, bool, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return repositoryBuildDefinition{}, false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return repositoryBuildDefinition{}, false, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return repositoryBuildDefinition{}, false, err
	}
	switch typed := value.(type) {
	case string:
		contextDir := strings.TrimSpace(typed)
		if contextDir == "" {
			return repositoryBuildDefinition{}, false, errors.New("build context is empty")
		}
		return repositoryBuildDefinition{Context: contextDir, Dockerfile: "Dockerfile", Canonical: canonical}, true, nil
	case map[string]any:
		contextDir, _ := typed["context"].(string)
		contextDir = strings.TrimSpace(contextDir)
		if contextDir == "" {
			return repositoryBuildDefinition{}, false, errors.New("build context is missing")
		}
		dockerfile, _ := typed["dockerfile"].(string)
		dockerfile = strings.TrimSpace(dockerfile)
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		return repositoryBuildDefinition{Context: contextDir, Dockerfile: dockerfile, Canonical: canonical}, true, nil
	default:
		return repositoryBuildDefinition{}, false, errors.New("unsupported rendered build definition")
	}
}

func fingerprintRepositoryBuildContext(contextDir, dockerfile string, canonicalBuild []byte) (string, error) {
	root, err := filepath.Abs(contextDir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("build context is not a directory")
	}
	dockerfilePath := dockerfile
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(root, dockerfilePath)
	}
	dockerfilePath, err = filepath.Abs(dockerfilePath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dockerfilePath); err != nil {
		return "", fmt.Errorf("inspect Dockerfile/Containerfile: %w", err)
	}

	rules, err := loadDockerIgnoreRules(root)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	writeBytes := func(label string, value []byte) {
		_, _ = io.WriteString(h, label)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(value)
		_, _ = h.Write([]byte{0})
	}
	writeBytes("build-definition", canonicalBuild)

	var paths []string
	err = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".baseharbor":
				return filepath.SkipDir
			}
			return nil
		}
		absolute, err := filepath.Abs(filePath)
		if err != nil {
			return err
		}
		forceInclude := absolute == dockerfilePath || rel == ".dockerignore"
		if !forceInclude && dockerIgnoreExcludes(rules, rel, entry.IsDir()) {
			return nil
		}
		if entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		filePath := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(filePath)
		if err != nil {
			return "", err
		}
		writeBytes("path", []byte(rel))
		writeBytes("mode", []byte(info.Mode().String()))
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filePath)
			if err != nil {
				return "", err
			}
			writeBytes("symlink", []byte(target))
			continue
		}
		file, err := os.Open(filePath)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(h, file); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func loadDockerIgnoreRules(root string) ([]dockerIgnoreRule, error) {
	file, err := os.Open(filepath.Join(root, ".dockerignore"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var rules []dockerIgnoreRule
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := strings.HasPrefix(line, "!")
		if negate {
			line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
		}
		line = strings.ReplaceAll(line, "\\", "/")
		line = strings.TrimPrefix(line, "/")
		dirOnly := strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		if line == "" || line == "." {
			continue
		}
		re, err := dockerIgnorePatternRegexp(line)
		if err != nil {
			// Unknown patterns are treated conservatively as included input:
			// a false-positive rebuild is safer than silently keeping stale code.
			continue
		}
		rules = append(rules, dockerIgnoreRule{re: re, negate: negate, dirOnly: dirOnly, basename: !strings.Contains(line, "/")})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

func dockerIgnorePatternRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func dockerIgnoreExcludes(rules []dockerIgnoreRule, rel string, isDir bool) bool {
	rel = path.Clean(filepath.ToSlash(rel))
	excluded := false
	for _, rule := range rules {
		if rule.dirOnly && !isDir && !strings.Contains(rel, "/") {
			continue
		}
		matched := rule.re.MatchString(rel)
		if !matched && rule.basename {
			for _, part := range strings.Split(rel, "/") {
				if rule.re.MatchString(part) {
					matched = true
					break
				}
			}
		}
		if !matched && rule.dirOnly {
			matched = strings.HasPrefix(rel, strings.TrimSuffix(rule.re.String(), "$"))
		}
		if matched {
			excluded = !rule.negate
		}
	}
	return excluded
}

func loadRepositoryWorkloadBuildState(files application.RuntimeFiles) (repositoryWorkloadBuildState, error) {
	data, err := os.ReadFile(repositoryWorkloadBuildStatePath(files))
	if errors.Is(err, os.ErrNotExist) {
		return repositoryWorkloadBuildState{Version: 1, Services: map[string]string{}}, nil
	}
	if err != nil {
		return repositoryWorkloadBuildState{}, err
	}
	var state repositoryWorkloadBuildState
	if err := json.Unmarshal(data, &state); err != nil {
		return repositoryWorkloadBuildState{}, fmt.Errorf("decode workload build fingerprint state: %w", err)
	}
	if state.Version != 1 {
		return repositoryWorkloadBuildState{}, fmt.Errorf("unsupported workload build fingerprint state version %d", state.Version)
	}
	if state.Services == nil {
		state.Services = map[string]string{}
	}
	return state, nil
}

func changedRepositoryWorkloadBuildServices(current map[string]string, previous repositoryWorkloadBuildState) []string {
	var changed []string
	for service, digest := range current {
		if previous.Services[service] != digest {
			changed = append(changed, service)
		}
	}
	sort.Strings(changed)
	return changed
}

func persistRepositoryWorkloadBuildState(files application.RuntimeFiles, fingerprints map[string]string) error {
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		return err
	}
	state := repositoryWorkloadBuildState{Version: 1, Services: fingerprints}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := repositoryWorkloadBuildStatePath(files)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
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
