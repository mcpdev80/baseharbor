package extension

import (
	"fmt"
	"regexp"
	"strings"
)

const DescriptorVersion = "baseharbor.extension/v1"
const JSONSchema202012 = "https://json-schema.org/draft/2020-12/schema"

type Family string

const (
	FamilyProvider    Family = "provider"
	FamilyDevelopment Family = "development"
)

type Artifact struct {
	OCIReference string `json:"oci_reference,omitempty"`
	Digest       string `json:"digest,omitempty"`
	MediaType    string `json:"media_type,omitempty"`
}

type SchemaReference struct {
	Dialect string `json:"dialect,omitempty"`
	URI     string `json:"uri,omitempty"`
}

type Provenance struct {
	SignatureRef   string `json:"signature_ref,omitempty"`
	SBOMRef        string `json:"sbom_ref,omitempty"`
	AttestationRef string `json:"attestation_ref,omitempty"`
}

type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type Compatibility struct {
	BaseHarbor string     `json:"baseharbor,omitempty"`
	Contracts  []string   `json:"contracts,omitempty"`
	Platforms  []Platform `json:"platforms,omitempty"`
}

type Metadata struct {
	SchemaVersion       string          `json:"schema_version"`
	ID                  string          `json:"id"`
	Family              Family          `json:"family"`
	Version             string          `json:"version"`
	Compatibility       Compatibility   `json:"compatibility,omitempty"`
	Artifact            Artifact        `json:"artifact,omitempty"`
	ConfigurationSchema SchemaReference `json:"configuration_schema,omitempty"`
	Provenance          Provenance      `json:"provenance,omitempty"`
}

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`)
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

func (m Metadata) Validate() error {
	if m.SchemaVersion != DescriptorVersion {
		return fmt.Errorf("extension schema version %q is unsupported; expected %q", m.SchemaVersion, DescriptorVersion)
	}
	if !idPattern.MatchString(strings.TrimSpace(m.ID)) {
		return fmt.Errorf("extension id %q must use namespace/name form", m.ID)
	}
	switch m.Family {
	case FamilyProvider, FamilyDevelopment:
	default:
		return fmt.Errorf("extension %q has unsupported family %q", m.ID, m.Family)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("extension %q version is required", m.ID)
	}
	if err := m.Compatibility.Validate(); err != nil {
		return fmt.Errorf("extension %q compatibility: %w", m.ID, err)
	}
	if err := m.Artifact.Validate(); err != nil {
		return fmt.Errorf("extension %q artifact: %w", m.ID, err)
	}
	if err := m.ConfigurationSchema.Validate(); err != nil {
		return fmt.Errorf("extension %q configuration schema: %w", m.ID, err)
	}
	return nil
}

func (a Artifact) Validate() error {
	ref := strings.TrimSpace(a.OCIReference)
	digest := strings.TrimSpace(a.Digest)
	mediaType := strings.TrimSpace(a.MediaType)
	if ref == "" && digest == "" && mediaType == "" {
		return nil
	}
	if ref == "" {
		return fmt.Errorf("OCI reference is required when artifact metadata is declared")
	}
	if digest != "" && !digestPattern.MatchString(digest) {
		return fmt.Errorf("digest %q must be an immutable sha256 digest", digest)
	}
	return nil
}

func (s SchemaReference) Validate() error {
	dialect := strings.TrimSpace(s.Dialect)
	uri := strings.TrimSpace(s.URI)
	if dialect == "" && uri == "" {
		return nil
	}
	if dialect != JSONSchema202012 {
		return fmt.Errorf("dialect %q is unsupported; expected JSON Schema 2020-12", dialect)
	}
	return nil
}

func (c Compatibility) Validate() error {
	seen := map[string]struct{}{}
	for _, contract := range c.Contracts {
		contract = strings.TrimSpace(contract)
		if contract == "" {
			return fmt.Errorf("contract entries must not be empty")
		}
		if _, exists := seen[contract]; exists {
			return fmt.Errorf("contract %q is declared more than once", contract)
		}
		seen[contract] = struct{}{}
	}
	seenPlatforms := map[string]struct{}{}
	for _, platform := range c.Platforms {
		osName := strings.TrimSpace(platform.OS)
		arch := strings.TrimSpace(platform.Arch)
		if osName == "" || arch == "" {
			return fmt.Errorf("platform requires os and arch")
		}
		key := osName + "/" + arch
		if _, exists := seenPlatforms[key]; exists {
			return fmt.Errorf("platform %q is declared more than once", key)
		}
		seenPlatforms[key] = struct{}{}
	}
	return nil
}
