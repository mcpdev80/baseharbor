package development

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type ProfileScope string

const (
	ProfileScopeBuiltin ProfileScope = "built-in"
	ProfileScopeUser ProfileScope = "user"
	ProfileScopeRepository ProfileScope = "repository"
)

type ProfileEntry struct {
	Profile StackProfile `json:"profile"`
	Scope   ProfileScope `json:"scope"`
	Path    string       `json:"path,omitempty"`
}

type ProfileCatalogEntries map[string]ProfileEntry

func BuiltinProfile(adapterID, name string) StackProfile {
	return StackProfile{
		APIVersion: StackProfileAPIVersion,
		Kind:       StackProfileKind,
		Metadata:   ProfileMetadata{Name: name},
		Components: []Component{{ID: "app", Role: "application", Adapter: adapterID}},
	}
}

func UserProfileRoot() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(root, "baseharbor", "stacks"), nil
}

func RepositoryProfileRoot(repositoryRoot string) string {
	if strings.TrimSpace(repositoryRoot) == "" {
		repositoryRoot = "."
	}
	return filepath.Join(repositoryRoot, ".baseharbor", "stacks")
}

func LoadProfileCatalog(repositoryRoot string, builtins map[string]StackProfile) (ProfileCatalogEntries, error) {
	result := ProfileCatalogEntries{}
	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := builtins[name]
		if err := profile.Validate(); err != nil {
			return nil, fmt.Errorf("built-in stack profile %q: %w", name, err)
		}
		result[name] = ProfileEntry{Profile: profile, Scope: ProfileScopeBuiltin}
	}
	userRoot, err := UserProfileRoot()
	if err != nil {
		return nil, err
	}
	if err := loadProfileDirectory(result, userRoot, ProfileScopeUser); err != nil {
		return nil, err
	}
	if err := loadProfileDirectory(result, RepositoryProfileRoot(repositoryRoot), ProfileScopeRepository); err != nil {
		return nil, err
	}
	return result, nil
}

func loadProfileDirectory(result ProfileCatalogEntries, root string, scope ProfileScope) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s stack profiles: %w", scope, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".yaml") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read stack profile %s: %w", path, err)
		}
		var profile StackProfile
		if err := yaml.Unmarshal(data, &profile); err != nil {
			return fmt.Errorf("parse stack profile %s: %w", path, err)
		}
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("validate stack profile %s: %w", path, err)
		}
		result[profile.Metadata.Name] = ProfileEntry{Profile: profile, Scope: scope, Path: path}
	}
	return nil
}

func SaveProfile(profile StackProfile, scope ProfileScope, repositoryRoot string) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	var root string
	switch scope {
	case ProfileScopeUser:
		var err error
		root, err = UserProfileRoot()
		if err != nil {
			return "", err
		}
	case ProfileScopeRepository:
		root = RepositoryProfileRoot(repositoryRoot)
	default:
		return "", fmt.Errorf("stack profile scope %q is not writable", scope)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create stack profile directory: %w", err)
	}
	name := safeProfileFilename(profile.Metadata.Name)
	path := filepath.Join(root, name+".yaml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("stack profile %q already exists at %s", profile.Metadata.Name, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	data, err := yaml.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("encode stack profile: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write stack profile: %w", err)
	}
	return path, nil
}

func safeProfileFilename(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-.")
	if result == "" {
		return "stack"
	}
	return result
}

func ProfileMap(entries ProfileCatalogEntries) ProfileCatalog {
	out := ProfileCatalog{}
	for name, entry := range entries {
		out[name] = entry.Profile
	}
	return out
}
