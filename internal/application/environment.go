package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepositoryEnvironmentSelection is the deterministic repository/application
// source selected for one deployment environment. Environment-specific
// manifests are complete application contracts; they are never overlays.
type RepositoryEnvironmentSelection struct {
	RepositoryRoot      string
	ManifestPath        string
	Manifest            Manifest
	Environment         string
	EnvironmentSpecific bool
	LegacyState         bool
}

// ResolveRepositoryEnvironment resolves the repository root, complete manifest
// and deployment environment as one operation.
//
// A root baseharbor.yaml remains the compatibility path for simple repositories.
// An explicit environment first prefers envs/<environment>/baseharbor.yaml. If
// that file does not exist, the root manifest is reused as complete intent and
// only its deployment context is overridden. This preserves the established
// -e/--environment behavior without mutating the repository contract.
func ResolveRepositoryEnvironment(start, requested string) (RepositoryEnvironmentSelection, error) {
	if start == "" {
		start = "."
	}
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if err := validateSlug("environment", requested); err != nil {
			return RepositoryEnvironmentSelection{}, err
		}
	}

	current, err := filepath.Abs(start)
	if err != nil {
		return RepositoryEnvironmentSelection{}, fmt.Errorf("resolve current directory: %w", err)
	}

	for {
		rootManifest := filepath.Join(current, RepositoryManifestName)

		// When invoked from inside envs/<name>, preserve the real repository root
		// instead of treating the environment directory as a standalone repo.
		if filepath.Base(filepath.Dir(current)) == "envs" {
			if exists, err := regularFileExists(rootManifest); err != nil {
				return RepositoryEnvironmentSelection{}, err
			} else if exists {
				environment := filepath.Base(current)
				repoRoot := filepath.Dir(filepath.Dir(current))
				if requested == "" || requested == environment {
					return loadEnvironmentSelection(repoRoot, rootManifest, environment, true, false)
				}
				// A different explicit environment must be resolved from the real
				// repository root, never by retargeting this environment manifest.
				current = repoRoot
				continue
			}
		}

		if requested != "" {
			environmentManifest := filepath.Join(current, "envs", requested, RepositoryManifestName)
			if exists, err := regularFileExists(environmentManifest); err != nil {
				return RepositoryEnvironmentSelection{}, err
			} else if exists {
				return loadEnvironmentSelection(current, environmentManifest, requested, true, false)
			}

			if exists, err := regularFileExists(rootManifest); err != nil {
				return RepositoryEnvironmentSelection{}, err
			} else if exists {
				selection, err := loadEnvironmentSelection(current, rootManifest, requested, false, false)
				if err != nil {
					return RepositoryEnvironmentSelection{}, err
				}
				declared, err := LoadManifestFile(rootManifest)
				if err != nil {
					return RepositoryEnvironmentSelection{}, err
				}
				selection.LegacyState = declared.Environment == requested
				return selection, nil
			}
		} else {
			if exists, err := regularFileExists(rootManifest); err != nil {
				return RepositoryEnvironmentSelection{}, err
			} else if exists {
				m, err := LoadManifestFile(rootManifest)
				if err != nil {
					return RepositoryEnvironmentSelection{}, err
				}
				return RepositoryEnvironmentSelection{
					RepositoryRoot: current,
					ManifestPath:   rootManifest,
					Manifest:       m,
					Environment:    m.Environment,
					LegacyState:    true,
				}, nil
			}

			candidates, err := environmentManifestCandidates(current)
			if err != nil {
				return RepositoryEnvironmentSelection{}, err
			}
			switch len(candidates) {
			case 1:
				environment := filepath.Base(filepath.Dir(candidates[0]))
				return loadEnvironmentSelection(current, candidates[0], environment, true, false)
			case 0:
			default:
				return RepositoryEnvironmentSelection{}, fmt.Errorf(
					"multiple environment manifests found (%s); select one with -e/--environment",
					strings.Join(environmentNames(candidates), ", "),
				)
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return RepositoryEnvironmentSelection{}, fmt.Errorf(
		"%w: no %s or envs/<environment>/%s found in the current directory or any parent",
		ErrRepositoryManifestNotFound,
		RepositoryManifestName,
		RepositoryManifestName,
	)
}

// HasRepositoryApplication reports whether start is inside a repository that
// carries either the compatibility root manifest or one or more complete
// environment manifests. It intentionally does not select an environment.
func HasRepositoryApplication(start string) (bool, error) {
	if start == "" {
		start = "."
	}
	current, err := filepath.Abs(start)
	if err != nil {
		return false, fmt.Errorf("resolve current directory: %w", err)
	}
	for {
		rootManifest := filepath.Join(current, RepositoryManifestName)
		if exists, err := regularFileExists(rootManifest); err != nil {
			return false, err
		} else if exists {
			return true, nil
		}
		candidates, err := environmentManifestCandidates(current)
		if err != nil {
			return false, err
		}
		if len(candidates) > 0 {
			return true, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return false, nil
}

func loadEnvironmentSelection(repoRoot, path, environment string, environmentSpecific, legacyState bool) (RepositoryEnvironmentSelection, error) {
	m, err := LoadManifestFile(path)
	if err != nil {
		return RepositoryEnvironmentSelection{}, err
	}
	if environmentSpecific {
		if m.Environment != environment {
			return RepositoryEnvironmentSelection{}, fmt.Errorf(
				"environment manifest %s declares environment %q; expected %q",
				path,
				m.Environment,
				environment,
			)
		}
	} else {
		m.Environment = environment
		if err := m.Validate(); err != nil {
			return RepositoryEnvironmentSelection{}, fmt.Errorf("invalid environment selection %q: %w", environment, err)
		}
	}
	return RepositoryEnvironmentSelection{
		RepositoryRoot:      repoRoot,
		ManifestPath:        path,
		Manifest:            m,
		Environment:         environment,
		EnvironmentSpecific: environmentSpecific,
		LegacyState:         legacyState,
	}, nil
}

// RepositoryEnvironmentStateRoot keeps the v0.4.12 state path for the unchanged
// single-environment compatibility case and isolates every explicit/multi-env
// deployment below a stable environment scope.
func RepositoryEnvironmentStateRoot(selection RepositoryEnvironmentSelection) string {
	base := filepath.Join(selection.RepositoryRoot, ".baseharbor")
	if selection.LegacyState {
		return base
	}
	return filepath.Join(base, "environments", selection.Environment)
}

func environmentManifestCandidates(repoRoot string) ([]string, error) {
	envsRoot := filepath.Join(repoRoot, "envs")
	entries, err := os.ReadDir(envsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect environment manifests: %w", err)
	}
	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(envsRoot, entry.Name(), RepositoryManifestName)
		exists, err := regularFileExists(path)
		if err != nil {
			return nil, err
		}
		if exists {
			candidates = append(candidates, path)
		}
	}
	sort.Strings(candidates)
	return candidates, nil
}

func environmentNames(paths []string) []string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, filepath.Base(filepath.Dir(path)))
	}
	return names
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a regular file", path)
	}
	return true, nil
}
