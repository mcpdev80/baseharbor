package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// ManagedSecretsSecureBinding describes the existing BaseHarbor runtime
// identity/secret-delivery semantics without exposing OpenBao paths, AppRoles,
// policies, tokens, private keys, or provider-specific object names.
func ManagedSecretsSecureBinding(m Manifest) capability.SecureBinding {
	subject := fmt.Sprintf("spiffe://baseharbor/apps/%s/%s", m.Name, m.Environment)
	base := fmt.Sprintf("baseharbor://applications/%s/%s", m.Name, m.Environment)

	binding := capability.SecureBinding{
		Identity: &capability.WorkloadIdentityBinding{
			Subject:   subject,
			Reference: base + "/identities/runtime",
		},
		Credentials: []capability.CredentialReference{
			{
				Name:      "runtime-authentication",
				Reference: base + "/credentials/runtime-authentication",
			},
		},
		Trust: []capability.TrustMaterialReference{
			{
				Name:      "runtime-ca",
				Reference: base + "/trust/runtime-ca",
				Format:    "x509-pem",
			},
		},
		Authorization: []capability.AuthorizationMetadata{
			{
				Audience: "managed-secrets",
				Scopes:   []string{"secrets.read"},
			},
		},
		Lifecycle: capability.SecurityLifecycleSupport{
			Renewable: true,
			Rotatable: true,
			Revocable: true,
		},
		Diagnostics: []capability.Diagnostic{
			{
				Code:     "secure-binding-managed",
				Severity: capability.SeverityInfo,
				Message:  "managed secret binding uses application-scoped identity and least-privilege secret access",
			},
		},
	}

	for _, requirement := range m.Secrets.Required {
		binding.Secrets = append(binding.Secrets, capability.SecretReference{
			Name:      requirement.Name,
			Reference: base + "/secrets/" + requirement.Name,
		})
	}

	return binding
}
