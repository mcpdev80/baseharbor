package capability

import (
	"fmt"
	"net/url"
	"strings"
)

// SecureBinding carries provider-neutral security metadata required to bind one
// logical capability resource to one workload. Secret material itself is never
// represented here; only stable references may cross this boundary.
type SecureBinding struct {
	Identity      *WorkloadIdentityBinding `json:"identity,omitempty"`
	Credentials   []CredentialReference    `json:"credentials,omitempty"`
	Trust         []TrustMaterialReference `json:"trust,omitempty"`
	Authorization []AuthorizationMetadata  `json:"authorization,omitempty"`
	Secrets       []SecretReference        `json:"secrets,omitempty"`
	Lifecycle     SecurityLifecycleSupport `json:"lifecycle"`
	Diagnostics   []Diagnostic             `json:"diagnostics,omitempty"`
}

type WorkloadIdentityBinding struct {
	Subject   string `json:"subject"`
	Reference string `json:"reference"`
}

type CredentialReference struct {
	Name      string `json:"name"`
	Reference string `json:"reference"`
}

type TrustMaterialReference struct {
	Name      string `json:"name"`
	Reference string `json:"reference"`
	Format    string `json:"format,omitempty"`
}

type AuthorizationMetadata struct {
	Audience string   `json:"audience,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
}

type SecretReference struct {
	Name      string `json:"name"`
	Reference string `json:"reference"`
}

type SecurityLifecycleSupport struct {
	Renewable bool `json:"renewable"`
	Rotatable bool `json:"rotatable"`
	Revocable bool `json:"revocable"`
}

// Validate rejects ambiguous or unsafe secure-binding metadata before provider
// mutation. It validates references and metadata shape, not referenced secret
// values, which are resolved only at the trusted provider/runtime boundary.
func (b SecureBinding) Validate() error {
	if b.Identity != nil {
		if err := validateNamedReference("workload identity", b.Identity.Subject, b.Identity.Reference); err != nil {
			return err
		}
	}

	seenCredentials := make(map[string]struct{}, len(b.Credentials))
	for _, credential := range b.Credentials {
		if err := validateNamedReference("credential", credential.Name, credential.Reference); err != nil {
			return err
		}
		if _, exists := seenCredentials[credential.Name]; exists {
			return fmt.Errorf("secure binding contains duplicate credential %q", credential.Name)
		}
		seenCredentials[credential.Name] = struct{}{}
	}

	seenTrust := make(map[string]struct{}, len(b.Trust))
	for _, trust := range b.Trust {
		if err := validateNamedReference("trust material", trust.Name, trust.Reference); err != nil {
			return err
		}
		if _, exists := seenTrust[trust.Name]; exists {
			return fmt.Errorf("secure binding contains duplicate trust material %q", trust.Name)
		}
		seenTrust[trust.Name] = struct{}{}
		if strings.ContainsAny(trust.Format, "\r\n") {
			return fmt.Errorf("secure binding trust material %q has invalid format metadata", trust.Name)
		}
	}

	seenSecrets := make(map[string]struct{}, len(b.Secrets))
	for _, secret := range b.Secrets {
		if err := validateNamedReference("secret", secret.Name, secret.Reference); err != nil {
			return err
		}
		if _, exists := seenSecrets[secret.Name]; exists {
			return fmt.Errorf("secure binding contains duplicate secret %q", secret.Name)
		}
		seenSecrets[secret.Name] = struct{}{}
	}

	for _, authorization := range b.Authorization {
		if strings.ContainsAny(authorization.Audience, "\r\n") {
			return fmt.Errorf("secure binding authorization audience contains invalid control characters")
		}
		for _, scope := range authorization.Scopes {
			if strings.TrimSpace(scope) == "" || strings.ContainsAny(scope, "\r\n") {
				return fmt.Errorf("secure binding authorization contains invalid scope metadata")
			}
		}
	}

	for _, diagnostic := range b.Diagnostics {
		if strings.TrimSpace(diagnostic.Code) == "" {
			return fmt.Errorf("secure binding diagnostic code is required")
		}
		if diagnostic.Severity != SeverityInfo && diagnostic.Severity != SeverityError {
			return fmt.Errorf("secure binding diagnostic %q has invalid severity %q", diagnostic.Code, diagnostic.Severity)
		}
	}

	return nil
}

func validateNamedReference(kind, name, reference string) error {
	name = strings.TrimSpace(name)
	reference = strings.TrimSpace(reference)
	if name == "" {
		return fmt.Errorf("secure binding %s name is required", kind)
	}
	if reference == "" {
		return fmt.Errorf("secure binding %s %q reference is required", kind, name)
	}
	if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(reference, "\r\n") {
		return fmt.Errorf("secure binding %s %q contains invalid control characters", kind, name)
	}
	parsed, err := url.Parse(reference)
	if err != nil || parsed.Scheme != "baseharbor" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("secure binding %s %q reference must be an opaque baseharbor:// reference", kind, name)
	}
	return nil
}
