package deployment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ConfigVersion = 1

var targetSlug = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

type AccessDefinition struct {
	Provider      string `yaml:"provider" json:"provider"`
	Reference     string `yaml:"reference" json:"reference"`
	NativeContext string `yaml:"native-context,omitempty" json:"native_context,omitempty"`
}

type RuntimeDefinition struct {
	Provider string `yaml:"provider" json:"provider"`
}

type TargetAccess struct {
	Reference string `yaml:"reference" json:"reference"`
}

type OpenBaoTargetConfig struct {
	RecoveryFile string `yaml:"recovery-file,omitempty" json:"recovery_file,omitempty"`
}

type TargetDefinition struct {
	Runtime RuntimeDefinition   `yaml:"runtime" json:"runtime"`
	Access  TargetAccess        `yaml:"access" json:"access"`
	Scope   string              `yaml:"scope,omitempty" json:"scope,omitempty"`
	OpenBao OpenBaoTargetConfig `yaml:"openbao,omitempty" json:"openbao,omitempty"`
}

type PromptConfig struct {
	Enabled         bool              `yaml:"enabled" json:"enabled"`
	Preset          string            `yaml:"preset,omitempty" json:"preset,omitempty"`
	Position        string            `yaml:"position,omitempty" json:"position,omitempty"`
	Environment     string            `yaml:"environment,omitempty" json:"environment,omitempty"`
	ShowApplication bool              `yaml:"show-application,omitempty" json:"show_application,omitempty"`
	TextOnly        bool              `yaml:"text-only,omitempty" json:"text_only,omitempty"`
	ProdIndicator   string            `yaml:"prod-indicator,omitempty" json:"prod_indicator,omitempty"`
	Labels          map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Colors          map[string]string `yaml:"colors,omitempty" json:"colors,omitempty"`
}

type Config struct {
	Version       int                         `yaml:"version" json:"version"`
	DefaultTarget string                      `yaml:"default-target,omitempty" json:"default_target,omitempty"`
	Access        map[string]AccessDefinition `yaml:"access,omitempty" json:"access,omitempty"`
	Targets       map[string]TargetDefinition `yaml:"targets,omitempty" json:"targets,omitempty"`
	Prompt        PromptConfig                `yaml:"prompt,omitempty" json:"prompt,omitempty"`
}

type ResolvedTarget struct {
	Name            string `json:"name"`
	RuntimeProvider string `json:"runtime_provider"`
	AccessReference string `json:"access_reference"`
	Scope           string `json:"scope,omitempty"`
}

func ConfigPath() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); root != "" {
		return filepath.Join(root, "baseharbor", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot determine BaseHarbor config directory")
	}
	return filepath.Join(home, ".config", "baseharbor", "config.yaml"), nil
}

func DataRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); root != "" {
		return filepath.Join(root, "baseharbor"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot determine BaseHarbor data directory")
	}
	return filepath.Join(home, ".local", "share", "baseharbor"), nil
}

func TargetStateRoot(name string) (string, error) {
	if err := ValidateTargetName(name); err != nil {
		return "", err
	}
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "targets", name), nil
}

func ValidateTargetName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !targetSlug.MatchString(name) {
		return fmt.Errorf("invalid target name %q: use lowercase letters, digits, dots or hyphens; names must start and end with a letter or digit", name)
	}
	return nil
}

func LoadConfig() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Version: ConfigVersion, Access: map[string]AccessDefinition{}, Targets: map[string]TargetDefinition{}}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read BaseHarbor config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse BaseHarbor config: %w", err)
	}
	if cfg.Version != ConfigVersion {
		return Config{}, fmt.Errorf("unsupported BaseHarbor config version %d", cfg.Version)
	}
	if cfg.Access == nil {
		cfg.Access = map[string]AccessDefinition{}
	}
	if cfg.Targets == nil {
		cfg.Targets = map[string]TargetDefinition{}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Save() error {
	if c.Version == 0 {
		c.Version = ConfigVersion
	}
	if err := c.Validate(); err != nil {
		return err
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create BaseHarbor config directory: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode BaseHarbor config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write BaseHarbor config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace BaseHarbor config: %w", err)
	}
	return nil
}

func (c Config) Validate() error {
	if c.Version != ConfigVersion {
		return fmt.Errorf("unsupported BaseHarbor config version %d", c.Version)
	}
	for name, access := range c.Access {
		if err := ValidateTargetName(name); err != nil {
			return fmt.Errorf("access %q: %w", name, err)
		}
		if strings.TrimSpace(access.Provider) == "" || strings.TrimSpace(access.Reference) == "" {
			return fmt.Errorf("access %q requires provider and reference", name)
		}
	}
	for name, target := range c.Targets {
		if err := ValidateTargetName(name); err != nil {
			return err
		}
		if strings.TrimSpace(target.Runtime.Provider) == "" {
			return fmt.Errorf("target %q requires runtime.provider", name)
		}
		ref := strings.TrimSpace(target.Access.Reference)
		if ref == "" {
			return fmt.Errorf("target %q requires access.reference", name)
		}
		access, ok := c.Access[ref]
		if !ok {
			return fmt.Errorf("target %q references unknown access %q", name, ref)
		}
		if strings.TrimSpace(access.Provider) != strings.TrimSpace(target.Runtime.Provider) {
			return fmt.Errorf("target %q runtime provider %q does not match access provider %q", name, target.Runtime.Provider, access.Provider)
		}
	}
	if c.DefaultTarget != "" {
		if _, ok := c.Targets[c.DefaultTarget]; !ok {
			return fmt.Errorf("default target %q is not configured", c.DefaultTarget)
		}
	}
	if value := strings.TrimSpace(c.Prompt.Preset); value != "" {
		switch value {
		case "minimal", "compact", "accessible", "detailed", "none":
		default:
			return fmt.Errorf("unsupported prompt preset %q", value)
		}
	}
	if value := strings.TrimSpace(c.Prompt.Position); value != "" {
		switch value {
		case "before-path", "after-path", "right":
		default:
			return fmt.Errorf("unsupported prompt position %q", value)
		}
	}
	if value := strings.TrimSpace(c.Prompt.Environment); value != "" {
		switch value {
		case "never", "critical-only", "always":
		default:
			return fmt.Errorf("unsupported prompt environment mode %q", value)
		}
	}
	return nil
}

func (c Config) ResolveTarget(explicit, activated string) (ResolvedTarget, error) {
	name := strings.TrimSpace(explicit)
	if name == "" {
		name = strings.TrimSpace(activated)
	}
	if name == "" {
		name = strings.TrimSpace(c.DefaultTarget)
	}
	if name == "" {
		name = "local"
	}
	target, ok := c.Targets[name]
	if !ok {
		if name == "local" {
			return ResolvedTarget{
				Name:            "local",
				RuntimeProvider: "compose",
				AccessReference: "local",
				Scope:           "default",
			}, nil
		}
		return ResolvedTarget{}, fmt.Errorf("target %q is not configured", name)
	}
	return ResolvedTarget{
		Name:            name,
		RuntimeProvider: target.Runtime.Provider,
		AccessReference: target.Access.Reference,
		Scope:           target.Scope,
	}, nil
}

func (c Config) TargetNames() []string {
	names := make([]string, 0, len(c.Targets))
	for name := range c.Targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
