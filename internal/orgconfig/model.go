package orgconfig

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const ContractVersion = "baseharbor.organization/v1"

type SourceKind string

const (
	SourceOCI    SourceKind = "oci"
	SourceGit    SourceKind = "git"
	SourceLocal  SourceKind = "local"
	SourceSystem SourceKind = "system"
)

type Source struct {
	Kind      SourceKind `yaml:"kind" json:"kind"`
	Location  string     `yaml:"location" json:"location"`
	Requested string     `yaml:"requested,omitempty" json:"requested,omitempty"`
}

type Resolution struct {
	Source           Source `yaml:"source" json:"source"`
	ResolvedVersion  string `yaml:"resolved-version,omitempty" json:"resolved_version,omitempty"`
	ResolvedDigest   string `yaml:"resolved-digest,omitempty" json:"resolved_digest,omitempty"`
	ResolvedRevision string `yaml:"resolved-revision,omitempty" json:"resolved_revision,omitempty"`
	Provenance       string `yaml:"provenance,omitempty" json:"provenance,omitempty"`
	CachePath        string `yaml:"cache-path,omitempty" json:"cache_path,omitempty"`
}

type Reference struct {
	Reference string `yaml:"reference" json:"reference"`
}

type ProviderDefault struct {
	Provider string `yaml:"provider" json:"provider"`
	Scope    string `yaml:"scope,omitempty" json:"scope,omitempty"`
}

type PolicyDefault struct {
	Policy    string `yaml:"policy" json:"policy"`
	Mandatory bool   `yaml:"mandatory,omitempty" json:"mandatory,omitempty"`
}

type EnvironmentDefaults struct {
	Target    string                     `yaml:"target,omitempty" json:"target,omitempty"`
	Stack     string                     `yaml:"stack,omitempty" json:"stack,omitempty"`
	Providers map[string]ProviderDefault `yaml:"providers,omitempty" json:"providers,omitempty"`
	Trust     map[string]Reference       `yaml:"trust,omitempty" json:"trust,omitempty"`
	Policies  []PolicyDefault            `yaml:"policies,omitempty" json:"policies,omitempty"`
}

type Config struct {
	APIVersion   string                         `yaml:"apiVersion" json:"api_version"`
	Organization string                         `yaml:"organization" json:"organization"`
	Defaults     EnvironmentDefaults            `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Environments map[string]EnvironmentDefaults `yaml:"environments,omitempty" json:"environments,omitempty"`
	Providers    map[string]Reference           `yaml:"providers,omitempty" json:"providers,omitempty"`
	Targets      map[string]Reference           `yaml:"targets,omitempty" json:"targets,omitempty"`
	Stacks       map[string]Reference           `yaml:"stacks,omitempty" json:"stacks,omitempty"`
	Trust        map[string]Reference           `yaml:"trust,omitempty" json:"trust,omitempty"`
	Policies     map[string]Reference           `yaml:"policies,omitempty" json:"policies,omitempty"`
}

var (
	namePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

func (s Source) Validate() error {
	switch s.Kind {
	case SourceOCI, SourceGit, SourceLocal, SourceSystem:
	default:
		return fmt.Errorf("organization source kind %q is unsupported", s.Kind)
	}
	if strings.TrimSpace(s.Location) == "" {
		return fmt.Errorf("organization source location is required")
	}
	switch s.Kind {
	case SourceOCI:
		if strings.ContainsAny(s.Location, "\r\n") {
			return fmt.Errorf("organization OCI source location is invalid")
		}
	case SourceGit:
		if strings.ContainsAny(s.Location, "\r\n") {
			return fmt.Errorf("organization Git source location is invalid")
		}
	}
	return nil
}

func (r Resolution) Validate() error {
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if d := strings.TrimSpace(r.ResolvedDigest); d != "" && !digestPattern.MatchString(d) {
		return fmt.Errorf("resolved digest %q must be an immutable sha256 digest", d)
	}
	if r.Source.Kind == SourceOCI && strings.TrimSpace(r.ResolvedDigest) == "" {
		return fmt.Errorf("resolved OCI organization configuration requires immutable digest")
	}
	if r.Source.Kind == SourceGit && strings.TrimSpace(r.ResolvedRevision) == "" {
		return fmt.Errorf("resolved Git organization configuration requires immutable revision")
	}
	return nil
}

func (c Config) Validate() error {
	if c.APIVersion != ContractVersion {
		return fmt.Errorf("organization apiVersion %q is unsupported; expected %q", c.APIVersion, ContractVersion)
	}
	if !namePattern.MatchString(strings.TrimSpace(c.Organization)) {
		return fmt.Errorf("organization name %q is invalid", c.Organization)
	}
	if err := validateDefaults("defaults", c.Defaults); err != nil {
		return err
	}
	for name, env := range c.Environments {
		if !namePattern.MatchString(strings.TrimSpace(name)) {
			return fmt.Errorf("environment name %q is invalid", name)
		}
		if err := validateDefaults("environment "+name, env); err != nil {
			return err
		}
	}
	for label, refs := range map[string]map[string]Reference{
		"providers": c.Providers,
		"targets":   c.Targets,
		"stacks":    c.Stacks,
		"trust":     c.Trust,
		"policies":  c.Policies,
	} {
		if err := validateReferenceMap(label, refs); err != nil {
			return err
		}
	}
	return nil
}

func validateDefaults(scope string, d EnvironmentDefaults) error {
	if strings.TrimSpace(d.Target) != "" && !namePattern.MatchString(strings.TrimSpace(d.Target)) {
		return fmt.Errorf("%s target %q is invalid", scope, d.Target)
	}
	if strings.TrimSpace(d.Stack) != "" && !namePattern.MatchString(strings.TrimSpace(d.Stack)) {
		return fmt.Errorf("%s stack %q is invalid", scope, d.Stack)
	}
	for capability, provider := range d.Providers {
		if strings.TrimSpace(capability) == "" || strings.TrimSpace(provider.Provider) == "" {
			return fmt.Errorf("%s provider defaults require capability and provider reference", scope)
		}
		switch strings.TrimSpace(provider.Scope) {
		case "", "application", "shared", "external":
		default:
			return fmt.Errorf("%s provider %q has unsupported scope %q", scope, capability, provider.Scope)
		}
		if containsSecretMaterial(provider.Provider) {
			return fmt.Errorf("%s provider %q contains secret-like material; use a reference", scope, capability)
		}
	}
	for name, ref := range d.Trust {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s trust name is required", scope)
		}
		if err := validateReference(ref); err != nil {
			return fmt.Errorf("%s trust %q: %w", scope, name, err)
		}
	}
	for i, policy := range d.Policies {
		if !namePattern.MatchString(strings.TrimSpace(policy.Policy)) {
			return fmt.Errorf("%s policy %d has invalid policy reference %q", scope, i, policy.Policy)
		}
	}
	return nil
}

func validateReferenceMap(label string, refs map[string]Reference) error {
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !namePattern.MatchString(strings.TrimSpace(name)) {
			return fmt.Errorf("%s name %q is invalid", label, name)
		}
		if err := validateReference(refs[name]); err != nil {
			return fmt.Errorf("%s %q: %w", label, name, err)
		}
	}
	return nil
}

func validateReference(ref Reference) error {
	value := strings.TrimSpace(ref.Reference)
	if value == "" {
		return fmt.Errorf("reference is required")
	}
	if containsSecretMaterial(value) {
		return fmt.Errorf("reference contains secret-like material; organization configuration stores references only")
	}
	return nil
}

func containsSecretMaterial(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(lower, "-----begin") {
		return true
	}
	for _, marker := range []string{"password=", "token=", "secret=", "private_key=", "private-key="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
