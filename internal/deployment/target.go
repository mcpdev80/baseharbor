package deployment

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/apierror"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
	"go.yaml.in/yaml/v3"
)

const ConfigVersion = 1

// Config sentinels. Re-exported from internal/apierror so callers that already
// match them via errors.Is against the apierror package keep working; the
// classify layer (internal/machine) recognizes the family from internal/apierror
// without importing this higher-level package. The values are identical to the
// apierror sentinels.
var (
	// ErrConfigMissing is returned when the configured BaseHarbor config root is
	// populated (other BaseHarbor state files are present) but the authoritative
	// config.yaml is absent. This is a misconfiguration the user must fix, never
	// a silent empty default — a missing config under an active config root would
	// otherwise make the daemon fall through to its implicit defaults. It belongs
	// to the same "configured but unusable" family as ErrConfigEmpty (an existing
	// but empty file).
	ErrConfigMissing = apierror.ErrConfigMissing

	// ErrConfigEmpty is returned when an existing config.yaml resolves but carries
	// no loadable content (zero bytes or whitespace only). An existing file is a
	// malformed configuration, never a clean absent-config default: the user
	// pointed BaseHarbor at this path on purpose, so an emptied or untouched file
	// must surface as an actionable error rather than silently falling through to
	// the implicit defaults. It belongs to the ErrConfigMissing family so callers
	// can treat "configured but unusable" the same way.
	ErrConfigEmpty = apierror.ErrConfigEmpty
)

var targetTenantID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var targetSlug = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

type AccessDefinition struct {
	Provider      string `yaml:"provider" json:"provider"`
	Reference     string `yaml:"reference" json:"reference"`
	NativeContext string `yaml:"native-context,omitempty" json:"native_context,omitempty"`
}

type RuntimeDefinition struct {
	Provider       string `yaml:"provider" json:"provider"`
	DockerEndpoint string `yaml:"docker-endpoint,omitempty" json:"docker_endpoint,omitempty"`
	DockerContext  string `yaml:"docker-context,omitempty" json:"docker_context,omitempty"`
	DockerMode     string `yaml:"docker-mode,omitempty" json:"docker_mode,omitempty"`
}

type TargetAccess struct {
	Reference string `yaml:"reference" json:"reference"`
}

type OpenBaoTargetConfig struct {
	RecoveryFile string `yaml:"recovery-file,omitempty" json:"recovery_file,omitempty"`
}

type OperatorAuthEnvironmentConfig struct {
	Provider     string   `yaml:"provider" json:"provider"`
	Issuer       string   `yaml:"issuer" json:"issuer"`
	ClientID     string   `yaml:"client-id" json:"client_id"`
	Scopes       []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	CallbackPort int      `yaml:"callback-port,omitempty" json:"callback_port,omitempty"`
}

type TargetDefinition struct {
	TenantID         string                                   `yaml:"tenant-id,omitempty" json:"tenant_id,omitempty"`
	Runtime          RuntimeDefinition                        `yaml:"runtime" json:"runtime"`
	Access           TargetAccess                             `yaml:"access" json:"access"`
	Scope            string                                   `yaml:"scope,omitempty" json:"scope,omitempty"`
	OpenBao          OpenBaoTargetConfig                      `yaml:"openbao,omitempty" json:"openbao,omitempty"`
	OperatorAuth     map[string]OperatorAuthEnvironmentConfig `yaml:"operator-auth,omitempty" json:"operator_auth,omitempty"`
	DeploymentPolicy *TargetDeploymentPolicy                  `yaml:"deployment-policy,omitempty" json:"deployment_policy,omitempty"`
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
	DockerEndpoint   string                  `json:"docker_endpoint,omitempty"`
	DockerContext    string                  `json:"docker_context,omitempty"`
	DockerMode       string                  `json:"docker_mode,omitempty"`
	TenantID         string                  `json:"tenant_id,omitempty"`
	Name             string                  `json:"name"`
	RuntimeProvider  string                  `json:"runtime_provider"`
	AccessProvider   string                  `json:"access_provider"`
	AccessReference  string                  `json:"access_reference"`
	Scope            string                  `json:"scope,omitempty"`
	DeploymentPolicy *TargetDeploymentPolicy `json:"deployment_policy,omitempty"`
}

func ConfigPath() (string, error) {
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return filepath.Join(root, "config", "config.yaml"), nil
	}
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
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return filepath.Clean(root), nil
	}
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
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// A config the user believes exists but that this process cannot
			// resolve is not a clean empty default: it is a configuration that
			// is being silently ignored (BF-05). When the resolved config
			// directory is populated by other BaseHarbor state files, the user
			// clearly intends this to be their BaseHarbor config root, so a
			// missing config.yaml here is a misconfiguration worth a loud,
			// actionable error rather than a silent fall-through to the
			// implicit default. An absent directory is still a clean no-config.
			if dirExistsAndHasBaseharborState(filepath.Dir(path)) {
				return Config{}, fmt.Errorf("%w: %s", ErrConfigMissing, path)
			}
			return Config{Version: ConfigVersion, Access: map[string]AccessDefinition{}, Targets: map[string]TargetDefinition{}}, nil
		}
		return Config{}, fmt.Errorf("read BaseHarbor config: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		// An EXISTING but empty/whitespace-only config.yaml is a malformed
		// configuration, not a clean absent-config default. The user pointed
		// BaseHarbor at this path on purpose, so an emptied or untouched file
		// must fail loud with an actionable error rather than silently falling
		// through to the implicit defaults (BF-05). Only a genuinely absent
		// file may keep the clean default below.
		return Config{}, fmt.Errorf("%w: %s", ErrConfigEmpty, path)
	}
	return decodeConfig(data)
}

// dirExistsAndHasBaseharborState reports whether the user's BaseHarbor config
// root is populated but the authoritative config.yaml is missing, so
// LoadConfig can fail loud (BF-05) instead of silently falling through to the
// implicit default. Two precise signals, no speculative climbing:
//
//  1. A config.yaml sits exactly one level up from where ConfigPath() looks.
//     This is the XDG double-append that made the H-D28 repro fall silently
//     through: XDG_CONFIG_HOME already targets the baseharbor config root, so
//     ConfigPath() appends a second "/baseharbor" and never reads the user's
//     file.
//  2. The resolved config directory exists and already carries the deployment
//     target marker (active-target) or the installation lock: the user's root
//     is active for deployment, so a missing config.yaml is a misconfiguration.
//
// Only deployment markers count. active.json / an organization/ subtree is the
// separate org-config system and must not, by itself, mark the deployment
// config root as populated. Anything else (unrelated subdirs, sibling temp
// directories) is ignored so a clean empty root stays a clean no-config.
func dirExistsAndHasBaseharborState(dir string) bool {
	// Signal 1: a config.yaml exactly one level up (XDG double-append).
	parent := filepath.Dir(dir)
	if parent != dir {
		if info, err := os.Stat(parent); err == nil && info.IsDir() {
			if entries, rerr := os.ReadDir(parent); rerr == nil {
				for _, entry := range entries {
					if entry.Name() == "config.yaml" {
						return true
					}
				}
			}
		}
	}
	// Signal 2: the resolved config directory carries deployment-target state.
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		if entries, rerr := os.ReadDir(dir); rerr == nil {
			for _, entry := range entries {
				switch entry.Name() {
				case "active-target", ".installation.lock":
					return true
				}
			}
		}
	}
	return false
}

func decodeConfig(data []byte) (Config, error) {
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
	return c.saveWithExpected(nil, false)
}

func (c Config) saveAt(path string, revalidate func() error) error {
	if c.Version == 0 {
		c.Version = ConfigVersion
	}
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode BaseHarbor config: %w", err)
	}
	if len(data) > 8<<20 {
		return errors.New("Target configuration exceeds the supported owner size")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write BaseHarbor config: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := revalidate(); err != nil {
		return err
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
		if _, err := targetaccess.ParseProviderKind(access.Provider); err != nil {
			return fmt.Errorf("access %q: %w", name, err)
		}
	}
	for name, target := range c.Targets {
		if target.DeploymentPolicy != nil {
			if err := target.DeploymentPolicy.Validate(); err != nil {
				return fmt.Errorf("target %q deployment policy: %w", name, err)
			}
		}
		if target.Runtime.DockerEndpoint != "" && target.Runtime.DockerContext != "" {
			return fmt.Errorf("target %q selects both Docker endpoint and context", name)
		}
		if target.Runtime.DockerMode != "" && target.Runtime.DockerMode != "rootless" && target.Runtime.DockerMode != "rootful" {
			return fmt.Errorf("target %q Docker mode must be rootless or rootful", name)
		}
		if target.Runtime.Provider != "docker" && (target.Runtime.DockerEndpoint != "" || target.Runtime.DockerContext != "" || target.Runtime.DockerMode != "") {
			return fmt.Errorf("target %q Docker engine configuration requires Docker runtime", name)
		}
		if target.TenantID != "" && !targetTenantID.MatchString(target.TenantID) {
			return fmt.Errorf("target %q tenant-id must be a canonical UUID", name)
		}
		if err := ValidateTargetName(name); err != nil {
			return err
		}
		for environment, auth := range target.OperatorAuth {
			if !targetSlug.MatchString(strings.TrimSpace(environment)) {
				return fmt.Errorf("target %q operator-auth environment %q is invalid", name, environment)
			}
			switch strings.TrimSpace(auth.Provider) {
			case "external-oidc", "managed-keycloak":
			default:
				return fmt.Errorf("target %q environment %q operator-auth provider must be external-oidc or managed-keycloak", name, environment)
			}
			issuer := strings.TrimRight(strings.TrimSpace(auth.Issuer), "/")
			if !strings.HasPrefix(issuer, "https://") || strings.ContainsAny(issuer, "\r\n") {
				return fmt.Errorf("target %q environment %q operator-auth issuer must be HTTPS", name, environment)
			}
			if strings.TrimSpace(auth.ClientID) == "" || strings.ContainsAny(auth.ClientID, "\r\n") {
				return fmt.Errorf("target %q environment %q operator-auth client-id is required", name, environment)
			}
			if auth.CallbackPort < 0 || auth.CallbackPort > 65535 {
				return fmt.Errorf("target %q environment %q operator-auth callback-port is invalid", name, environment)
			}
			for _, scope := range auth.Scopes {
				if strings.TrimSpace(scope) == "" || strings.ContainsAny(scope, " \t\r\n") {
					return fmt.Errorf("target %q environment %q operator-auth scope %q is invalid", name, environment, scope)
				}
			}
		}
		if strings.TrimSpace(target.Runtime.Provider) == "" {
			return fmt.Errorf("target %q requires runtime.provider", name)
		}
		ref := strings.TrimSpace(target.Access.Reference)
		if ref == "" {
			return fmt.Errorf("target %q requires access.reference", name)
		}
		if _, ok := c.Access[ref]; !ok {
			return fmt.Errorf("target %q references unknown access %q", name, ref)
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
				RuntimeProvider: "docker",
				AccessProvider:  "local",
				AccessReference: "local",
				Scope:           "default",
			}, nil
		}
		return ResolvedTarget{}, fmt.Errorf("target %q is not configured", name)
	}
	access := c.Access[target.Access.Reference]
	return ResolvedTarget{
		DockerEndpoint:   target.Runtime.DockerEndpoint,
		DockerContext:    target.Runtime.DockerContext,
		DockerMode:       target.Runtime.DockerMode,
		Name:             name,
		TenantID:         target.TenantID,
		RuntimeProvider:  target.Runtime.Provider,
		AccessProvider:   access.Provider,
		AccessReference:  target.Access.Reference,
		Scope:            target.Scope,
		DeploymentPolicy: target.DeploymentPolicy,
	}, nil
}

// ResolveDataRootTarget discovers a Core-only Target using its immutable,
// protected installation owner. It is never authority to adopt foreign state.
func ResolveDataRootTarget(name string) (ResolvedTarget, error) {
	root, err := TargetStateRoot(name)
	if err != nil {
		return ResolvedTarget{}, err
	}
	for _, dir := range []string{root, filepath.Join(root, "runtime")} {
		info, err := os.Lstat(dir)
		if err != nil {
			return ResolvedTarget{}, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return ResolvedTarget{}, fmt.Errorf("untrusted Core runtime directory %s; refusing to discover ownership", dir)
		}
	}
	state, err := coreinstallation.Load(filepath.Join(root, "runtime"))
	if err != nil {
		return ResolvedTarget{}, err
	}
	if state.Spec.Target != name || (state.Spec.Runtime != "docker" && state.Spec.Runtime != "podman") {
		return ResolvedTarget{}, fmt.Errorf("Core installation owner does not match Target %q or supported native runtime", name)
	}
	return ResolvedTarget{
		Name:            name,
		RuntimeProvider: state.Spec.Runtime,
		AccessProvider:  "local",
		AccessReference: "local",
		Scope:           "discovered",
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
